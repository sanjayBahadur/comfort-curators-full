package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"comfort-curators-backend/internal/platform/logging"
)

type Runner struct {
	store        *AgentRunStore
	factory      ProviderFactory
	workerID     string
	systemPrompt string
	toolExecutor ToolExecutor
}

func NewRunner(store *AgentRunStore, factory ProviderFactory, workerID string) *Runner {
	return &Runner{
		store:    store,
		factory:  factory,
		workerID: workerID,
	}
}

func NewRunnerWithToolLoop(store *AgentRunStore, factory ProviderFactory, workerID, systemPrompt string, toolExecutor ToolExecutor) *Runner {
	return &Runner{
		store:        store,
		factory:      factory,
		workerID:     workerID,
		systemPrompt: systemPrompt,
		toolExecutor: toolExecutor,
	}
}

func (r *Runner) RunRecoveryLoop(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			recovered, err := r.store.RecoverExpiredLeases(ctx)
			if err != nil {
				logging.Error(ctx, "agent run: failed to recover expired leases", "error", err)
			} else if recovered > 0 {
				logging.Info(ctx, "agent run: recovered expired leases", "count", recovered)
			}
		}
	}
}

func (r *Runner) RunWorkLoop(ctx context.Context, wg *sync.WaitGroup, allowedKinds []string) {
	defer wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		run, err := r.store.Claim(ctx, r.workerID, DefaultLeaseDuration, allowedKinds)
		if err != nil {
			if err != ErrRunNotFound {
				logging.Error(ctx, "agent run: failed to claim", "error", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(500 * time.Millisecond):
			}
			continue
		}

		logging.Info(ctx, "agent run: claimed",
			"run_id", run.RunID,
			"run_kind", run.RunKind,
			"attempt", run.Attempt,
		)

		if err := r.store.TransitionState(ctx, run.RunID, r.workerID, StateLeased, StateRunning); err != nil {
			logging.Error(ctx, "agent run: failed to transition to running", "run_id", run.RunID, "error", err)
			continue
		}

		processCtx, processCancel := context.WithCancel(ctx)

		var heartbeatWg sync.WaitGroup
		if DefaultLeaseDuration > 10*time.Second {
			heartbeatWg.Add(1)
			go r.runHeartbeat(processCtx, &heartbeatWg, run.RunID, DefaultLeaseDuration/2)
		}

		provider := r.factory(run.RunKind)
		if provider == nil {
			processCancel()
			heartbeatWg.Wait()
			logging.Error(ctx, "agent run: no provider for kind", "run_id", run.RunID, "run_kind", run.RunKind)
			if failErr := r.store.Fail(ctx, run.RunID, r.workerID, fmt.Sprintf("no provider registered for kind %s", run.RunKind)); failErr != nil {
				logging.Error(ctx, "agent run: failed to mark run as failed", "run_id", run.RunID, "error", failErr)
			}
			continue
		}

		r.processRun(processCtx, run, provider)

		processCancel()
		heartbeatWg.Wait()
	}
}

type chatMsg struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content,omitempty"`
	ToolCalls  json.RawMessage `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	Name       string          `json:"name,omitempty"`
}

type toolResult struct {
	Result string `json:"result"`
}

type runCheckpoint struct {
	Messages       []json.RawMessage `json:"messages"`
	Iteration      int               `json:"iteration"`
	LastToolCallID string            `json:"last_tool_call_id"`
	LastToolName   string            `json:"last_tool_name"`
	LastToolArgs   json.RawMessage   `json:"last_tool_args,omitempty"`
}

func (r *Runner) processRun(ctx context.Context, run *AgentRun, provider Provider) {
	accumUsage := ProviderUsage{}

	if len(run.MessagesJSON) > 0 {
		r.resumeRun(ctx, run, provider, &accumUsage)
		return
	}

	systemContent, _ := json.Marshal(r.systemPrompt)

	messages := make([]json.RawMessage, 0, 8)
	sysMsg := chatMsg{Role: "system", Content: systemContent}
	sysRaw, _ := json.Marshal(sysMsg)
	messages = append(messages, sysRaw)

	// run.InputData is itself a JSON object (context + message, see
	// superhost/handler.go). OpenAI-compatible chat APIs require a message's
	// "content" to be a string or a list of content parts, never a bare
	// object -- the local model-stub never validated this, but a real
	// provider rejects it with a 400. Embed the object as its own string
	// representation so the model still receives the full payload as text.
	userContent, _ := json.Marshal(string(run.InputData))
	userMsg := chatMsg{Role: "user", Content: userContent}
	userRaw, _ := json.Marshal(userMsg)
	messages = append(messages, userRaw)

	iteration := 0

	for iteration < MaxToolLoopIterations {
		iteration++

		if iteration == 1 {
			// Deliberately send Messages (already-built, correctly-shaped
			// system+user chatMsg pair) rather than System+Input: the latter
			// sent run.InputData -- a raw JSON object -- straight through as
			// http_provider.go's fallback "no Messages" path, which puts it
			// verbatim into a message's "content" field. That's invalid per
			// the OpenAI-compatible contract (content must be a string or a
			// list) and a real provider 400s on it; the local stub never
			// validated it, which is why this went unnoticed until now.
			msgsJSON, _ := json.Marshal(messages)
			resp, err := r.callProvider(ctx, run, provider, ProviderRequest{
				RunKind:               run.RunKind,
				Provider:              run.Provider,
				Model:                 run.Model,
				System:                r.systemPrompt,
				PromptTemplateVersion: run.PromptTemplateVersion,
				InputSchemaVersion:    run.InputSchemaVersion,
				Messages:              messages,
				Input:                 msgsJSON,
			})
			if err != nil {
				errMsg := err.Error()
				logging.Error(ctx, "agent run: provider call failed",
					"run_id", run.RunID,
					"run_kind", run.RunKind,
					"attempt", run.Attempt,
					"error", errMsg,
				)
				r.completeFallback(ctx, run, err)
				return
			}

			accumUsage.InputTokens += resp.TokenUsage.InputTokens
			accumUsage.OutputTokens += resp.TokenUsage.OutputTokens
			accumUsage.TotalTokens += resp.TokenUsage.TotalTokens
			accumUsage.UsageMinor += resp.UsageMinor
			accumUsage.UsageCurrency = resp.UsageCurr
			accumUsage.UsageKnown = resp.UsageKnown

			if len(resp.ToolCalls) == 0 || string(resp.ToolCalls) == "null" {
				r.completeRun(ctx, run, resp.Output, accumUsage)
				return
			}

			paused, err := r.handleToolCalls(ctx, run, provider, resp, &messages, &accumUsage, &iteration)
			if err != nil {
				terminated := r.failRun(ctx, run, err.Error())
				_ = terminated
				return
			}
			if paused {
				return
			}
		} else {
			msgsJSON, _ := json.Marshal(messages)
			resp, err := r.callProvider(ctx, run, provider, ProviderRequest{
				RunKind:               run.RunKind,
				Provider:              run.Provider,
				Model:                 run.Model,
				Messages:              messages,
				PromptTemplateVersion: run.PromptTemplateVersion,
				InputSchemaVersion:    run.InputSchemaVersion,
				Input:                 msgsJSON,
			})
			if err != nil {
				errMsg := err.Error()
				logging.Error(ctx, "agent run: provider call failed",
					"run_id", run.RunID,
					"run_kind", run.RunKind,
					"iteration", iteration,
					"error", errMsg,
				)
				r.completeFallback(ctx, run, err)
				return
			}

			accumUsage.InputTokens += resp.TokenUsage.InputTokens
			accumUsage.OutputTokens += resp.TokenUsage.OutputTokens
			accumUsage.TotalTokens += resp.TokenUsage.TotalTokens
			accumUsage.UsageMinor += resp.UsageMinor
			accumUsage.UsageCurrency = resp.UsageCurr
			accumUsage.UsageKnown = resp.UsageKnown

			if len(resp.ToolCalls) == 0 || string(resp.ToolCalls) == "null" {
				r.completeRun(ctx, run, resp.Output, accumUsage)
				return
			}

			paused, err := r.handleToolCalls(ctx, run, provider, resp, &messages, &accumUsage, &iteration)
			if err != nil {
				terminated := r.failRun(ctx, run, err.Error())
				_ = terminated
				return
			}
			if paused {
				return
			}
		}
	}

	errMsg := fmt.Sprintf("tool-loop iteration cap (%d) exceeded", MaxToolLoopIterations)
	logging.Error(ctx, "agent run: tool-loop cap exceeded",
		"run_id", run.RunID,
		"run_kind", run.RunKind,
		"iterations", iteration,
	)
	if failErr := r.store.Fail(ctx, run.RunID, r.workerID, errMsg); failErr != nil {
		logging.Error(ctx, "agent run: failed to mark run as failed", "run_id", run.RunID, "error", failErr)
	}
}

func (r *Runner) resumeRun(ctx context.Context, run *AgentRun, provider Provider, accumUsage *ProviderUsage) {
	var ck runCheckpoint
	if err := json.Unmarshal(run.MessagesJSON, &ck); err != nil {
		logging.Error(ctx, "agent run: failed to unmarshal run checkpoint", "run_id", run.RunID, "error", err)
		r.completeFallback(ctx, run, fmt.Errorf("checkpoint corrupt: %w", err))
		return
	}

	messages := ck.Messages

	// A human approved this tool call (DecideRun only reaches here from a
	// real approval decision -- see RecoverExpiredLeases' comment on why
	// waiting_for_approval is deliberately excluded from lease recovery).
	// If the executor knows how to actually carry out the approved tool
	// (ApprovedToolExecutor), do that and report its real result. If it
	// doesn't -- or execution itself fails -- say so plainly rather than
	// claiming an effect that didn't happen.
	resultText := "Approved by human reviewer. No automated action is configured for this tool yet."
	if approvedExecutor, ok := r.toolExecutor.(ApprovedToolExecutor); ok {
		summary, err := approvedExecutor.ExecuteApproved(ctx, run, ck.LastToolName, ck.LastToolArgs)
		if err != nil {
			resultText = fmt.Sprintf("Approved by human reviewer, but executing %s failed: %v", ck.LastToolName, err)
		} else {
			resultText = summary
		}
	}
	approvalContent, _ := json.Marshal(resultText)
	toolResultMsg := chatMsg{
		Role:       "tool",
		Content:    approvalContent,
		ToolCallID: ck.LastToolCallID,
		Name:       ck.LastToolName,
	}
	toolRaw, _ := json.Marshal(toolResultMsg)
	messages = append(messages, toolRaw)

	iteration := ck.Iteration

	for iteration < MaxToolLoopIterations {
		iteration++

		msgsJSON, _ := json.Marshal(messages)
		resp, err := r.callProvider(ctx, run, provider, ProviderRequest{
			RunKind:               run.RunKind,
			Provider:              run.Provider,
			Model:                 run.Model,
			Messages:              messages,
			PromptTemplateVersion: run.PromptTemplateVersion,
			InputSchemaVersion:    run.InputSchemaVersion,
			Input:                 msgsJSON,
		})
		if err != nil {
			errMsg := err.Error()
			logging.Error(ctx, "agent run: provider call failed",
				"run_id", run.RunID,
				"iteration", iteration,
				"error", errMsg,
			)
			r.completeFallback(ctx, run, err)
			return
		}

		accumUsage.InputTokens += resp.TokenUsage.InputTokens
		accumUsage.OutputTokens += resp.TokenUsage.OutputTokens
		accumUsage.TotalTokens += resp.TokenUsage.TotalTokens
		accumUsage.UsageMinor += resp.UsageMinor
		accumUsage.UsageCurrency = resp.UsageCurr
		accumUsage.UsageKnown = resp.UsageKnown

		if len(resp.ToolCalls) == 0 || string(resp.ToolCalls) == "null" {
			r.completeRun(ctx, run, resp.Output, *accumUsage)
			return
		}

		paused, err := r.handleToolCalls(ctx, run, provider, resp, &messages, accumUsage, &iteration)
		if err != nil {
			terminated := r.failRun(ctx, run, err.Error())
			_ = terminated
			return
		}
		if paused {
			return
		}
	}

	errMsg := fmt.Sprintf("tool-loop iteration cap (%d) exceeded", MaxToolLoopIterations)
	logging.Error(ctx, "agent run: tool-loop cap exceeded",
		"run_id", run.RunID,
		"run_kind", run.RunKind,
		"iterations", iteration,
	)
	if failErr := r.store.Fail(ctx, run.RunID, r.workerID, errMsg); failErr != nil {
		logging.Error(ctx, "agent run: failed to mark run as failed", "run_id", run.RunID, "error", failErr)
	}
}

// streamingTextWriteInterval throttles how often an in-progress model
// response is persisted to agent_runs.streaming_text for the SSE poll loop
// to pick up (see stream.go). The poll loop itself only checks every
// streamPollInterval, so writing more often than that buys nothing but
// database load; this stays a bit under it so a write is essentially always
// ready by the time a poll tick looks for one.
const streamingTextWriteInterval = 300 * time.Millisecond

func (r *Runner) callProvider(ctx context.Context, run *AgentRun, provider Provider, request ProviderRequest) (*ProviderResponse, error) {
	callCtx, cancel := context.WithTimeout(ctx, providerCallTimeout)
	defer cancel()

	streamingProvider, canStream := provider.(StreamingProvider)
	if !canStream {
		resp, err := provider.Call(callCtx, request)
		if err != nil {
			return nil, err
		}
		if resp == nil {
			return nil, fmt.Errorf("provider returned nil response")
		}
		return resp, nil
	}

	var lastWrite time.Time
	onDelta := func(text string) {
		if time.Since(lastWrite) < streamingTextWriteInterval {
			return
		}
		lastWrite = time.Now()
		if err := r.store.UpdateStreamingText(ctx, run.RunID, text); err != nil {
			logging.Error(ctx, "agent run: failed to persist streaming text", "run_id", run.RunID, "error", err)
		}
	}

	resp, err := streamingProvider.CallStream(callCtx, request, onDelta)
	// Whatever the outcome, this call is done -- clear the in-progress text
	// so a stale mid-sentence fragment can't linger and be shown again
	// against a *later* run that reuses this same run_id's row state, and
	// so the final real text (from the completed/failed event) is what
	// renders next, not this now-obsolete partial one.
	if clearErr := r.store.UpdateStreamingText(ctx, run.RunID, ""); clearErr != nil {
		logging.Error(ctx, "agent run: failed to clear streaming text", "run_id", run.RunID, "error", clearErr)
	}
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, fmt.Errorf("provider returned nil response")
	}
	return resp, nil
}

func (r *Runner) completeFallback(ctx context.Context, run *AgentRun, providerErr error) {
	canned := chooseFallback(run)
	output, _ := json.Marshal(fallbackOutput{
		Message:        canned.Message,
		IsFallback:     true,
		FallbackMarker: fallbackMarker,
		Intent:         canned.Intent,
	})
	eventData, _ := json.Marshal(map[string]any{
		"is_fallback":     true,
		"fallback_marker": fallbackMarker,
		"intent":          canned.Intent,
		"reason":          providerErr.Error(),
		"usage_known":     false,
	})
	if err := r.recordEvent(ctx, run.RunID, EventRunFallback, eventData); err != nil {
		logging.Error(ctx, "agent run: failed to record fallback event", "run_id", run.RunID, "error", err)
	}
	r.completeRun(ctx, run, output, ProviderUsage{UsageKnown: false})
}

// NormalizeToolArguments handles the two shapes a tool call's
// function.arguments can arrive in. Per the OpenAI-compatible function-
// calling spec (which every real provider we've tried, DeepSeek included,
// follows), arguments is always a JSON-encoded *string* -- e.g.
// `"{\"property_id\": \"prop_123\"}"` -- never a bare object. If it's not a
// JSON string (e.g. some other provider already hands back a plain
// object), pass it through unchanged rather than fail.
func NormalizeToolArguments(raw json.RawMessage) json.RawMessage {
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return json.RawMessage(asString)
	}
	return raw
}

func (r *Runner) handleToolCalls(ctx context.Context, run *AgentRun, provider Provider, resp *ProviderResponse, messages *[]json.RawMessage, accumUsage *ProviderUsage, iteration *int) (paused bool, err error) {
	assistantMsg := chatMsg{
		Role:      "assistant",
		Content:   resp.Output,
		ToolCalls: resp.ToolCalls,
	}
	assistantRaw, _ := json.Marshal(assistantMsg)
	*messages = append(*messages, assistantRaw)

	var rawCalls []struct {
		ID       string `json:"id"`
		Type     string `json:"type"`
		Function struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		} `json:"function"`
	}
	if err := json.Unmarshal(resp.ToolCalls, &rawCalls); err != nil {
		return false, fmt.Errorf("unmarshal tool calls: %w", err)
	}

	hasToolExecutor := r.toolExecutor != nil

	for _, tc := range rawCalls {
		// Per the OpenAI-compatible function-calling spec every real
		// provider follows, function.arguments arrives as a JSON-encoded
		// *string*, not a bare object. Normalize once, here, at the source
		// -- everything downstream (the tool executor's policy evaluation
		// AND the ToolCallProposed.v1 event the frontend's UI-action driver
		// reads arguments.surface_id off of) depends on it already being a
		// real object. The local model-stub never produced this shape, so
		// it went uncaught until a real provider was wired in.
		tc.Function.Arguments = NormalizeToolArguments(tc.Function.Arguments)
		callJSON, _ := json.Marshal(tc)

		proposedData, _ := json.Marshal(map[string]any{
			"tool_name": tc.Function.Name,
			"version":   "v1",
			"arguments": tc.Function.Arguments,
		})

		if hasToolExecutor {
			if err := r.recordEvent(ctx, run.RunID, "ToolCallProposed.v1", proposedData); err != nil {
				logging.Error(ctx, "agent run: failed to record tool proposal event", "run_id", run.RunID, "error", err)
			}

			outcome, evalErr := r.toolExecutor.Evaluate(ctx, run, callJSON)
			if evalErr != nil {
				return false, fmt.Errorf("tool evaluation: %w", evalErr)
			}

			switch outcome.Type {
			case ToolLoopDenied:
				denialData, _ := json.Marshal(map[string]any{
					"tool_name": outcome.ToolName,
					"reason":    outcome.DenialReason,
				})
				if err := r.recordEvent(ctx, run.RunID, "PolicyDenied.v1", denialData); err != nil {
					logging.Error(ctx, "agent run: failed to record denial event", "run_id", run.RunID, "error", err)
				}

				denialText := fmt.Sprintf("Policy denied tool call to %s: %s", outcome.ToolName, outcome.DenialReason)
				denialContent, _ := json.Marshal(denialText)
				toolResultMsg := chatMsg{
					Role:       "tool",
					Content:    denialContent,
					ToolCallID: tc.ID,
					Name:       tc.Function.Name,
				}
				toolRaw, _ := json.Marshal(toolResultMsg)
				*messages = append(*messages, toolRaw)

			case ToolLoopApprovalRequired:
				approvalData, _ := json.Marshal(map[string]any{
					"tool_name":           outcome.ToolName,
					"approval_request_id": outcome.ApprovalRequestID,
					"summary":             outcome.ApprovalSummary,
				})
				if err := r.recordEvent(ctx, run.RunID, "ApprovalRequired.v1", approvalData); err != nil {
					logging.Error(ctx, "agent run: failed to record approval event", "run_id", run.RunID, "error", err)
				}

				ck := runCheckpoint{
					Messages:       *messages,
					Iteration:      *iteration,
					LastToolCallID: tc.ID,
					LastToolName:   tc.Function.Name,
					LastToolArgs:   tc.Function.Arguments,
				}
				ckJSON, err := json.Marshal(ck)
				if err != nil {
					return false, fmt.Errorf("marshal run checkpoint: %w", err)
				}
				if err := r.store.SaveMessages(ctx, run.RunID, r.workerID, ckJSON); err != nil {
					return false, fmt.Errorf("save run checkpoint: %w", err)
				}

				if err := r.store.TransitionState(ctx, run.RunID, r.workerID, StateRunning, StateWaitingForApproval); err != nil {
					return false, fmt.Errorf("transition to waiting_for_approval: %w", err)
				}
				return true, nil

			case ToolLoopAllowed:
				allowedData, _ := json.Marshal(map[string]any{
					"tool_name":      outcome.ToolName,
					"result_summary": outcome.ResultSummary,
				})
				if err := r.recordEvent(ctx, run.RunID, "PolicyAllowed.v1", allowedData); err != nil {
					logging.Error(ctx, "agent run: failed to record allowed event", "run_id", run.RunID, "error", err)
				}

				resultContent, _ := json.Marshal(outcome.ResultSummary)
				toolResultMsg := chatMsg{
					Role:       "tool",
					Content:    resultContent,
					ToolCallID: tc.ID,
					Name:       tc.Function.Name,
				}
				toolRaw, _ := json.Marshal(toolResultMsg)
				*messages = append(*messages, toolRaw)
			}
		}
	}

	return false, nil
}

func (r *Runner) recordEvent(ctx context.Context, runID, eventName string, eventData json.RawMessage) error {
	return RecordEvent(ctx, r.store.pool, runID, eventName, eventData)
}

func (r *Runner) completeRun(ctx context.Context, run *AgentRun, output json.RawMessage, accumUsage ProviderUsage) {
	logging.Info(ctx, "agent run: completed",
		"run_id", run.RunID,
		"run_kind", run.RunKind,
		"provider", run.Provider,
		"model", run.Model,
		"usage_minor", accumUsage.UsageMinor,
		"usage_known", accumUsage.UsageKnown,
		"input_tokens", accumUsage.InputTokens,
		"output_tokens", accumUsage.OutputTokens,
		"total_tokens", accumUsage.TotalTokens,
	)
	if completeErr := r.store.CompleteWithUsage(ctx, run.RunID, r.workerID, output, accumUsage); completeErr != nil {
		logging.Error(ctx, "agent run: failed to mark run as complete", "run_id", run.RunID, "error", completeErr)
	}
}

func (r *Runner) failRun(ctx context.Context, run *AgentRun, errMsg string) error {
	return r.store.Fail(ctx, run.RunID, r.workerID, errMsg)
}

func (r *Runner) runHeartbeat(ctx context.Context, wg *sync.WaitGroup, runID string, interval time.Duration) {
	defer wg.Done()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := r.store.Heartbeat(ctx, runID, r.workerID, DefaultLeaseDuration); err != nil {
				logging.Error(ctx, "agent run: heartbeat failed", "run_id", runID, "error", err)
				return
			}
		}
	}
}
