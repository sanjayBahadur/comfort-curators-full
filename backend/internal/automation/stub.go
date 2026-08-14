package automation

import (
	"context"
	"encoding/json"
	"fmt"
)

type StubProvider struct {
	Mode string
}

func NewStubProvider(mode string) *StubProvider {
	return &StubProvider{Mode: mode}
}

func (s *StubProvider) Call(ctx context.Context, req ProviderRequest) (*ProviderResponse, error) {
	switch s.Mode {
	case "timeout":
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	case "unavailable":
		return nil, fmt.Errorf("stub: provider unavailable")
	case "malformed":
		return nil, fmt.Errorf("stub: malformed response from provider")
	default:
	}

	out := map[string]any{
		"content": fmt.Sprintf("stub response for kind=%s model=%s", req.RunKind, req.Model),
	}
	if req.Input != nil {
		var parsed any
		if json.Unmarshal(req.Input, &parsed) == nil {
			out["echo"] = parsed
		}
	}

	outJSON, _ := json.Marshal(out)
	tokens := TokenUsage{InputTokens: 10, OutputTokens: 20, TotalTokens: 30}
	return &ProviderResponse{
		Output:     json.RawMessage(outJSON),
		Provider:   "stub",
		Model:      req.Model,
		TokenUsage: tokens,
		UsageMinor: 0,
		UsageCurr:  "",
		UsageKnown: false,
	}, nil
}

type CountingProvider struct {
	inner     Provider
	CallCount int
}

func NewCountingProvider(inner Provider) *CountingProvider {
	return &CountingProvider{inner: inner}
}

func (c *CountingProvider) Call(ctx context.Context, req ProviderRequest) (*ProviderResponse, error) {
	c.CallCount++
	return c.inner.Call(ctx, req)
}
