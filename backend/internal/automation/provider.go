package automation

import (
	"context"
	"encoding/json"
)

type ProviderRequest struct {
	RunKind               string          `json:"run_kind"`
	Provider              string          `json:"provider"`
	Model                 string          `json:"model"`
	System                string          `json:"system,omitempty"`
	PromptTemplateVersion string          `json:"prompt_template_version,omitempty"`
	InputSchemaVersion    string          `json:"input_schema_version,omitempty"`
	Input                 json.RawMessage `json:"input"`

	Messages []json.RawMessage `json:"messages,omitempty"`
}

// TokenUsage records the model provider's reported token counts for one call.
type TokenUsage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
	TotalTokens  int64 `json:"total_tokens"`
}

type ProviderResponse struct {
	Output     json.RawMessage `json:"output"`
	Provider   string          `json:"provider"`
	Model      string          `json:"model"`
	TokenUsage TokenUsage      `json:"token_usage"`
	UsageMinor int64           `json:"usage_minor"`
	UsageCurr  string          `json:"usage_curr"`
	// UsageKnown reports whether UsageMinor is a real monetary figure derived
	// from the explicit pricing table. When false, the (provider, model) pair
	// has no published price or no reported token counts, and UsageMinor is 0
	// rather than a fabricated cost.
	UsageKnown bool            `json:"usage_known"`
	ToolCalls  json.RawMessage `json:"tool_calls,omitempty"`
}

type Provider interface {
	Call(ctx context.Context, req ProviderRequest) (*ProviderResponse, error)
}

// StreamingProvider is an optional capability a Provider can also implement.
// onDelta is called with the cumulative narrative text so far, each time
// more of it arrives from the upstream model -- not a diff, the whole
// string, so a caller can just render it directly. It is never called for
// pure tool-call turns with no narrative content. The final ProviderResponse
// returned is identical in shape and meaning to what Call would return for
// the same request.
type StreamingProvider interface {
	CallStream(ctx context.Context, req ProviderRequest, onDelta func(text string)) (*ProviderResponse, error)
}

type ProviderFactory func(kind string) Provider
