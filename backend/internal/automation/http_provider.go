package automation

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

type HTTPProvider struct {
	BaseURL string
	APIKey  string
	Tools   []ChatToolDef
	client  *http.Client
}

func NewHTTPProvider(baseURL, apiKey string, tools []ChatToolDef) *HTTPProvider {
	return &HTTPProvider{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Tools:   tools,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

type chatRequest struct {
	Model         string             `json:"model"`
	Messages      []chatMessage      `json:"messages"`
	Tools         []ChatToolDef      `json:"tools,omitempty"`
	Stream        bool               `json:"stream,omitempty"`
	StreamOptions *chatStreamOptions `json:"stream_options,omitempty"`
}

type chatStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type ChatToolDef struct {
	Type     string           `json:"type"`
	Function ChatToolFunction `json:"function"`
}

type ChatToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type chatMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content,omitempty"`
	ToolCalls  json.RawMessage `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	Name       string          `json:"name,omitempty"`
}

type chatResponse struct {
	Choices []chatChoice `json:"choices"`
	Usage   usageInfo    `json:"usage"`
}

// usageInfo covers both the OpenAI-compatible ("prompt_tokens",
// "completion_tokens", "total_tokens") and Anthropic-compatible
// ("input_tokens", "output_tokens") usage object shapes. Fields absent from
// the response stay zero and are reported as such rather than fabricated.
type usageInfo struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
	InputTokens      int64 `json:"input_tokens"`
	OutputTokens     int64 `json:"output_tokens"`
}

func (u usageInfo) tokens() TokenUsage {
	input, output, total := u.PromptTokens, u.CompletionTokens, u.TotalTokens
	if u.InputTokens != 0 {
		input = u.InputTokens
	}
	if u.OutputTokens != 0 {
		output = u.OutputTokens
	}
	if total == 0 {
		total = input + output
	}
	return TokenUsage{InputTokens: input, OutputTokens: output, TotalTokens: total}
}

type chatChoice struct {
	Message chatChoiceMessage `json:"message"`
}

type chatChoiceMessage struct {
	Role      string          `json:"role"`
	Content   json.RawMessage `json:"content,omitempty"`
	ToolCalls []chatToolCall  `json:"tool_calls,omitempty"`
}

type chatToolCall struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Function chatToolCallFn  `json:"function"`
}

type chatToolCallFn struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type chatErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// redactKey strips any occurrence of a non-empty secret from text pulled
// from an upstream HTTP response before it can reach a returned error
// (and from there, logs or a persisted agent-run error record). A
// misconfigured or malicious endpoint that echoes back request headers
// (e.g. Authorization) must not be able to exfiltrate CC_MODEL_API_KEY
// this way.
func redactKey(text, key string) string {
	if key == "" {
		return text
	}
	return strings.ReplaceAll(text, key, "[REDACTED]")
}

// resolvedRequest carries what both Call and CallStream need before they
// diverge on how they read the HTTP response: the built chatRequest body
// plus the model/baseURL/apiKey the caller resolved it against (Call and
// CallStream must report the same Model back on ProviderResponse regardless
// of which path ran, and need apiKey again to redact it out of any error
// text).
type resolvedRequest struct {
	chatReq chatRequest
	model   string
	baseURL string
	apiKey  string
}

func (p *HTTPProvider) resolveRequest(req ProviderRequest, stream bool) (resolvedRequest, error) {
	model := req.Model
	if model == "" {
		model = "gpt-5.6-luna"
	}

	baseURL := p.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}

	apiKey := p.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("CC_MODEL_API_KEY")
	}

	var messages []chatMessage
	if len(req.Messages) > 0 {
		messages = make([]chatMessage, 0, len(req.Messages))
		for _, raw := range req.Messages {
			var cm chatMessage
			if err := json.Unmarshal(raw, &cm); err != nil {
				return resolvedRequest{}, fmt.Errorf("http provider: unmarshal message: %w", err)
			}
			messages = append(messages, cm)
		}
	} else {
		messages = make([]chatMessage, 0, 2)
		if req.System != "" {
			systemContent, err := json.Marshal(req.System)
			if err != nil {
				return resolvedRequest{}, fmt.Errorf("http provider: marshal system prompt: %w", err)
			}
			messages = append(messages, chatMessage{
				Role:    "system",
				Content: systemContent,
			})
		}
		messages = append(messages, chatMessage{
			Role:    "user",
			Content: req.Input,
		})
	}

	chatReq := chatRequest{
		Model:    model,
		Messages: messages,
		Tools:    p.Tools,
	}
	if stream {
		chatReq.Stream = true
		chatReq.StreamOptions = &chatStreamOptions{IncludeUsage: true}
	}

	return resolvedRequest{chatReq: chatReq, model: model, baseURL: baseURL, apiKey: apiKey}, nil
}

func (p *HTTPProvider) newRequest(ctx context.Context, rr resolvedRequest) (*http.Request, error) {
	body, err := json.Marshal(rr.chatReq)
	if err != nil {
		return nil, fmt.Errorf("http provider: marshal request: %w", err)
	}
	if os.Getenv("CC_DEBUG_HTTP_PROVIDER") != "" {
		_ = os.WriteFile("/tmp/cc-debug-request.json", body, 0o644)
	}

	url := rr.baseURL + "/v1/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("http provider: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if rr.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+rr.apiKey)
	}
	return httpReq, nil
}

func (p *HTTPProvider) Call(ctx context.Context, req ProviderRequest) (*ProviderResponse, error) {
	rr, err := p.resolveRequest(req, false)
	if err != nil {
		return nil, err
	}

	httpReq, err := p.newRequest(ctx, rr)
	if err != nil {
		return nil, err
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http provider: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("http provider: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var chatErr chatErrorResponse
		if json.Unmarshal(respBody, &chatErr) == nil && chatErr.Error.Message != "" {
			return nil, fmt.Errorf("http provider: status %d: %s", resp.StatusCode, redactKey(chatErr.Error.Message, rr.apiKey))
		}
		return nil, fmt.Errorf("http provider: status %d: %s", resp.StatusCode, redactKey(string(respBody), rr.apiKey))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("http provider: unmarshal response: %w", err)
	}

	var output json.RawMessage
	var toolCalls json.RawMessage
	if len(chatResp.Choices) > 0 {
		output = chatResp.Choices[0].Message.Content
		if len(chatResp.Choices[0].Message.ToolCalls) > 0 {
			tc, _ := json.Marshal(chatResp.Choices[0].Message.ToolCalls)
			toolCalls = json.RawMessage(tc)
		}
	} else {
		output = json.RawMessage(`{"empty":true}`)
	}

	tokens := chatResp.Usage.tokens()
	usageMinor, usageCurr, usageKnown := usageForTokens(req.Provider, rr.model, tokens.InputTokens, tokens.OutputTokens)
	return &ProviderResponse{
		Output:     output,
		Provider:   req.Provider,
		Model:      rr.model,
		TokenUsage: tokens,
		UsageMinor: usageMinor,
		UsageCurr:  usageCurr,
		UsageKnown: usageKnown,
		ToolCalls:  toolCalls,
	}, nil
}

// chatStreamChunk is one `data: {...}` frame of an OpenAI-compatible
// streaming chat completion. Every real provider tried so far (DeepSeek
// included) follows this shape: content arrives as small text fragments in
// delta.content; a tool call's arguments arrive the same way, fragmented
// across many chunks and identified by index (not by a stable id -- the id
// itself typically only appears once, on the first fragment for that call).
type chatStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
	} `json:"choices"`
	Usage *usageInfo `json:"usage"`
}

type accumulatingToolCall struct {
	id, callType, name string
	arguments          strings.Builder
}

// CallStream is the streaming counterpart to Call: same request, same final
// ProviderResponse shape, but onDelta is invoked with the cumulative
// narrative text every time more of it arrives, so a caller can show the
// model actually composing its answer instead of a blank wait followed by
// the full text appearing at once. Tool-call arguments also arrive
// fragmented in the underlying stream, but are only ever surfaced whole, on
// ProviderResponse.ToolCalls -- a tool call isn't the "type it live" thing
// this exists for, and reconstructing a syntactically valid tool call
// mid-stream only to overwrite it is not worth the complexity.
func (p *HTTPProvider) CallStream(ctx context.Context, req ProviderRequest, onDelta func(text string)) (*ProviderResponse, error) {
	rr, err := p.resolveRequest(req, true)
	if err != nil {
		return nil, err
	}

	httpReq, err := p.newRequest(ctx, rr)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http provider: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		var chatErr chatErrorResponse
		if json.Unmarshal(respBody, &chatErr) == nil && chatErr.Error.Message != "" {
			return nil, fmt.Errorf("http provider: status %d: %s", resp.StatusCode, redactKey(chatErr.Error.Message, rr.apiKey))
		}
		return nil, fmt.Errorf("http provider: status %d: %s", resp.StatusCode, redactKey(string(respBody), rr.apiKey))
	}

	var content strings.Builder
	toolCalls := map[int]*accumulatingToolCall{}
	var usage usageInfo
	var haveUsage bool

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		data, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue
		}
		data = strings.TrimSpace(data)
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			break
		}

		var chunk chatStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			// A malformed chunk from an otherwise-working stream isn't worth
			// aborting an in-progress, mostly-good response over -- skip it
			// and keep accumulating what does parse.
			continue
		}
		if chunk.Usage != nil {
			usage = *chunk.Usage
			haveUsage = true
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta
		if delta.Content != "" {
			content.WriteString(delta.Content)
			if os.Getenv("CC_DEBUG_HTTP_PROVIDER") != "" {
				log.Printf("http provider: stream delta at %s: %q (total %d chars)", time.Now().Format(time.RFC3339Nano), delta.Content, content.Len())
			}
			if onDelta != nil {
				onDelta(content.String())
			}
		}
		for _, tc := range delta.ToolCalls {
			acc, exists := toolCalls[tc.Index]
			if !exists {
				acc = &accumulatingToolCall{}
				toolCalls[tc.Index] = acc
			}
			if tc.ID != "" {
				acc.id = tc.ID
			}
			if tc.Type != "" {
				acc.callType = tc.Type
			}
			if tc.Function.Name != "" {
				acc.name = tc.Function.Name
			}
			acc.arguments.WriteString(tc.Function.Arguments)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("http provider: read stream: %w", err)
	}

	outputJSON, err := json.Marshal(content.String())
	if err != nil {
		return nil, fmt.Errorf("http provider: marshal streamed content: %w", err)
	}

	var toolCallsJSON json.RawMessage
	if len(toolCalls) > 0 {
		indices := make([]int, 0, len(toolCalls))
		for idx := range toolCalls {
			indices = append(indices, idx)
		}
		sort.Ints(indices)

		calls := make([]chatToolCall, 0, len(indices))
		for _, idx := range indices {
			acc := toolCalls[idx]
			argsJSON, marshalErr := json.Marshal(acc.arguments.String())
			if marshalErr != nil {
				return nil, fmt.Errorf("http provider: marshal streamed tool arguments: %w", marshalErr)
			}
			calls = append(calls, chatToolCall{
				ID:   acc.id,
				Type: acc.callType,
				Function: chatToolCallFn{
					Name:      acc.name,
					Arguments: json.RawMessage(argsJSON),
				},
			})
		}
		tc, marshalErr := json.Marshal(calls)
		if marshalErr != nil {
			return nil, fmt.Errorf("http provider: marshal streamed tool calls: %w", marshalErr)
		}
		toolCallsJSON = json.RawMessage(tc)
	}

	var tokens TokenUsage
	if haveUsage {
		tokens = usage.tokens()
	}
	usageMinor, usageCurr, usageKnown := usageForTokens(req.Provider, rr.model, tokens.InputTokens, tokens.OutputTokens)
	return &ProviderResponse{
		Output:     json.RawMessage(outputJSON),
		Provider:   req.Provider,
		Model:      rr.model,
		TokenUsage: tokens,
		UsageMinor: usageMinor,
		UsageCurr:  usageCurr,
		UsageKnown: usageKnown,
		ToolCalls:  toolCallsJSON,
	}, nil
}
