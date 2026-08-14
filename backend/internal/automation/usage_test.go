package automation

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHTTPProviderCostFromTokenCountsNotBodySize proves usage accounting is
// driven by the response's reported token counts, not by the response body
// size. Two responses with drastically different body sizes but identical
// token counts must yield identical recorded usage and cost.
func TestHTTPProviderCostFromTokenCountsNotBodySize(t *testing.T) {
	smallBody := `{"choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":100,"completion_tokens":50,"total_tokens":150}}`
	bigBody := `{"choices":[{"message":{"role":"assistant","content":"` + strings.Repeat("padding ", 2000) + `"}}],"usage":{"prompt_tokens":100,"completion_tokens":50,"total_tokens":150}}`

	call := func(body string) *ProviderResponse {
		t.Helper()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		}))
		defer srv.Close()

		p := NewHTTPProvider(srv.URL, "", nil)
		resp, err := p.Call(context.Background(), ProviderRequest{Provider: "openai", Model: "gpt-4o"})
		if err != nil {
			t.Fatalf("call: %v", err)
		}
		return resp
	}

	small := call(smallBody)
	big := call(bigBody)

	if small.TokenUsage.InputTokens != 100 || small.TokenUsage.OutputTokens != 50 || small.TokenUsage.TotalTokens != 150 {
		t.Fatalf("unexpected token counts: %+v", small.TokenUsage)
	}
	if small.UsageMinor != big.UsageMinor {
		t.Fatalf("cost must depend on token counts, not body size: small=%d big=%d", small.UsageMinor, big.UsageMinor)
	}
	if small.UsageMinor != 750 {
		t.Fatalf("expected computed cost 750 for gpt-4o (100 in, 50 out), got %d", small.UsageMinor)
	}
	if small.UsageMinor == int64(len(smallBody)) || big.UsageMinor == int64(len(bigBody)) {
		t.Fatalf("cost must not equal the response body length: smallLen=%d bigLen=%d cost=%d", len(smallBody), len(bigBody), small.UsageMinor)
	}
	if !small.UsageKnown || !big.UsageKnown {
		t.Fatal("priced pair with reported tokens must be marked known")
	}
}

// TestHTTPProviderParsesAnthropicUsageShape covers the
// input_tokens/output_tokens usage naming used by Anthropic-compatible
// responses.
func TestHTTPProviderParsesAnthropicUsageShape(t *testing.T) {
	body := `{"content":[{"type":"text","text":"hi"}],"usage":{"input_tokens":5,"output_tokens":7}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	p := NewHTTPProvider(srv.URL, "", nil)
	resp, err := p.Call(context.Background(), ProviderRequest{Provider: "anthropic", Model: "claude-3-haiku"})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if resp.TokenUsage.InputTokens != 5 || resp.TokenUsage.OutputTokens != 7 || resp.TokenUsage.TotalTokens != 12 {
		t.Fatalf("unexpected token counts: %+v", resp.TokenUsage)
	}
	// claude-3-haiku: input 250 minor/1K, output 1250 minor/1K.
	if resp.UsageMinor != 10 {
		t.Fatalf("unexpected cost: got %d want 10", resp.UsageMinor)
	}
	if !resp.UsageKnown {
		t.Fatal("priced pair with reported tokens must be marked known")
	}
}

// TestHTTPProviderMissingUsageIsZeroNotFabricated: when the response carries
// no usage object, the token counts stay zero and no cost is invented.
func TestHTTPProviderMissingUsageIsZeroNotFabricated(t *testing.T) {
	body := `{"choices":[{"message":{"role":"assistant","content":"no usage object"}}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	p := NewHTTPProvider(srv.URL, "", nil)
	resp, err := p.Call(context.Background(), ProviderRequest{Provider: "openai", Model: "gpt-4o"})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if resp.TokenUsage != (TokenUsage{}) {
		t.Fatalf("expected zero token usage, got %+v", resp.TokenUsage)
	}
	if resp.UsageMinor != 0 || resp.UsageKnown {
		t.Fatalf("missing usage must not fabricate a cost: minor=%d known=%v", resp.UsageMinor, resp.UsageKnown)
	}
}

// TestPricingKnownPairCalculatesCost verifies a known (provider, model) pair
// produces a real calculated cost from the pricing table.
func TestPricingKnownPairCalculatesCost(t *testing.T) {
	minor, currency, known := usageForTokens("openai", "gpt-4o", 1000, 1000)
	if !known {
		t.Fatal("known pair must be marked known")
	}
	if currency != "USD" {
		t.Fatalf("unexpected currency: %q", currency)
	}
	// input 1000*2500/1000 = 2500, output 1000*10000/1000 = 10000.
	if minor != 12500 {
		t.Fatalf("unexpected cost: got %d want 12500", minor)
	}
}

// TestPricingLunaAndDeepSeekRowsKnown verifies the DEF-05 additions: the
// gpt-5.6-luna runtime model and both DeepSeek models are priced, so a known
// token count produces a real calculated cost with known=true for each.
func TestPricingLunaAndDeepSeekRowsKnown(t *testing.T) {
	cases := []struct {
		provider, model string
		inMinor         int64
		outMinor        int64
	}{
		// openai/gpt-5.6-luna: input $0.20/1M => 200, output $1.20/1M => 1200.
		{"openai", "gpt-5.6-luna", 200, 1200},
		// deepseek/deepseek-v4-flash: input $0.14/1M => 140, output $0.28/1M => 280.
		{"deepseek", "deepseek-v4-flash", 140, 280},
		// deepseek/deepseek-v4-pro: input $0.435/1M => 435, output $0.87/1M => 870.
		{"deepseek", "deepseek-v4-pro", 435, 870},
	}
	for _, tc := range cases {
		minor, currency, known := usageForTokens(tc.provider, tc.model, 1000, 1000)
		if !known {
			t.Fatalf("%s/%s must be a known pair", tc.provider, tc.model)
		}
		if currency != "USD" {
			t.Fatalf("%s/%s: unexpected currency %q", tc.provider, tc.model, currency)
		}
		if want := tc.inMinor + tc.outMinor; minor != want {
			t.Fatalf("%s/%s: unexpected cost got %d want %d", tc.provider, tc.model, minor, want)
		}
	}
}

// TestPricingUnknownPairExplicitlyUnknown verifies an unknown (provider,
// model) pair records an explicit unknown/zero cost rather than a fabricated
// number.
func TestPricingUnknownPairExplicitlyUnknown(t *testing.T) {
	minor, currency, known := usageForTokens("openai", "gpt-9-imaginary", 1000, 1000)
	if known || minor != 0 || currency != "" {
		t.Fatalf("unknown pair must be explicitly unknown/zero, got minor=%d curr=%q known=%v", minor, currency, known)
	}
}

// TestHTTPProviderUnknownPairIsExplicitlyUnknown runs the same guarantee
// through the HTTP provider path.
func TestHTTPProviderUnknownPairIsExplicitlyUnknown(t *testing.T) {
	body := `{"choices":[{"message":{"role":"assistant","content":"hi"}}],"usage":{"prompt_tokens":500,"completion_tokens":300,"total_tokens":800}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	p := NewHTTPProvider(srv.URL, "", nil)
	resp, err := p.Call(context.Background(), ProviderRequest{Provider: "model-stub", Model: "stub-v1"})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if resp.UsageKnown || resp.UsageMinor != 0 || resp.UsageCurr != "" {
		t.Fatalf("unpriced pair must be explicitly unknown, got minor=%d curr=%q known=%v", resp.UsageMinor, resp.UsageCurr, resp.UsageKnown)
	}
	if resp.TokenUsage.InputTokens != 500 || resp.TokenUsage.OutputTokens != 300 {
		t.Fatalf("token counts must still be recorded: %+v", resp.TokenUsage)
	}
}

// TestStubProviderReportsDeterministicTokenCounts checks the deterministic
// stub returns fixed token counts and an explicitly unpriced (unknown) cost.
func TestStubProviderReportsDeterministicTokenCounts(t *testing.T) {
	p := NewStubProvider("")
	resp, err := p.Call(context.Background(), ProviderRequest{RunKind: "jarvis", Model: "test-model-v1"})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if resp.Provider != "stub" {
		t.Fatalf("unexpected provider: %q", resp.Provider)
	}
	if resp.Model != "test-model-v1" {
		t.Fatalf("unexpected model: %q", resp.Model)
	}
	if resp.TokenUsage.InputTokens != 10 || resp.TokenUsage.OutputTokens != 20 || resp.TokenUsage.TotalTokens != 30 {
		t.Fatalf("unexpected token counts: %+v", resp.TokenUsage)
	}
	if resp.UsageKnown || resp.UsageMinor != 0 || resp.UsageCurr != "" {
		t.Fatalf("unpriced stub must be explicitly unknown, got minor=%d curr=%q known=%v", resp.UsageMinor, resp.UsageCurr, resp.UsageKnown)
	}
}
