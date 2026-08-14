package automation

// tokenPrice is the list cost of 1,000 tokens for one (provider, model) pair,
// expressed in integer minor currency units. The minor unit used for usage
// accounting is one millionth of the currency unit (a micro-dollar for USD):
// it is the smallest whole unit that captures published per-token prices
// exactly without rounding.
type tokenPrice struct {
	InputPer1KMinor  int64
	OutputPer1KMinor int64
	Currency         string
}

// modelPricing is the explicit pricing table keyed by "provider/model". Prices
// are the public list prices for each model, converted to integer minor units
// per 1K tokens. Only pairs with a published price are listed; anything else
// is priced "unknown" and is never given a fabricated figure.
var modelPricing = map[string]tokenPrice{
	// OpenAI (https://openai.com/api/pricing): input $2.50/1M, output $10.00/1M
	// gpt-4o => 2500 / 10000 minor units per 1K tokens.
	"openai/gpt-4o": {InputPer1KMinor: 2500, OutputPer1KMinor: 10000, Currency: "USD"},
	// OpenAI: input $0.15/1M, output $0.60/1M.
	"openai/gpt-4o-mini": {InputPer1KMinor: 150, OutputPer1KMinor: 600, Currency: "USD"},
	// Anthropic (https://www.anthropic.com/pricing): input $3.00/1M, output $15.00/1M.
	"anthropic/claude-3-5-sonnet-20241022": {InputPer1KMinor: 3000, OutputPer1KMinor: 15000, Currency: "USD"},
	// Anthropic: input $0.25/1M, output $1.25/1M.
	"anthropic/claude-3-haiku": {InputPer1KMinor: 250, OutputPer1KMinor: 1250, Currency: "USD"},
	// OpenAI GPT-5.6 Luna — the runtime model (D5): input $0.20/1M, output $1.20/1M.
	"openai/gpt-5.6-luna": {InputPer1KMinor: 200, OutputPer1KMinor: 1200, Currency: "USD"},
	// DeepSeek fallback: input $0.14/1M, output $0.28/1M.
	"deepseek/deepseek-v4-flash": {InputPer1KMinor: 140, OutputPer1KMinor: 280, Currency: "USD"},
	// DeepSeek: input $0.435/1M, output $0.87/1M.
	"deepseek/deepseek-v4-pro": {InputPer1KMinor: 435, OutputPer1KMinor: 870, Currency: "USD"},
}

// priceForModel returns the per-1K-token price for a (provider, model) pair.
// ok is false when the pair has no published price.
func priceForModel(provider, model string) (tokenPrice, bool) {
	p, ok := modelPricing[provider+"/"+model]
	return p, ok
}

// usageForTokens converts reported token counts into a monetary cost in
// integer minor units using the explicit pricing table. The per-1K rates are
// scaled by the token counts and rounded half-up to the nearest minor unit.
// When the (provider, model) pair has no published price, or no tokens were
// reported, known is false and the cost is 0: an unknown price is never
// replaced with a fabricated figure.
func usageForTokens(provider, model string, inputTokens, outputTokens int64) (minor int64, currency string, known bool) {
	p, ok := priceForModel(provider, model)
	if !ok || inputTokens+outputTokens <= 0 {
		return 0, "", false
	}
	inputMinor := (inputTokens*p.InputPer1KMinor + 500) / 1000
	outputMinor := (outputTokens*p.OutputPer1KMinor + 500) / 1000
	return inputMinor + outputMinor, p.Currency, true
}
