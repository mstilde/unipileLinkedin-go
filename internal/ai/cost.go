// Package ai integrates Anthropic Claude and OpenAI for the campaign pipeline.
//
// Three operations are exposed:
//
//   - ClassifyMessage — detects intent of an incoming prospect message. Two
//     layers: cheap regex (STOP_WORDS, AGGRESSIVE_WORDS, OBVIOUS_PATTERNS) that
//     never costs tokens, then Anthropic Haiku with Sonnet fallback.
//   - GenerateReply — composes an automatic reply for a prospect message in the
//     persona's voice, with PII validation post-generation.
//   - EnrichProspect — extracts icebreaker + pain_points + ICP fit + cleaned
//     fields from a LinkedIn profile using OpenAI GPT-4o-mini (22x cheaper than
//     Sonnet for structured tasks).
//
// Both providers are hand-rolled HTTP clients (no SDK dependency). Endpoints:
//
//   - Anthropic Messages API: https://api.anthropic.com/v1/messages
//   - OpenAI Chat Completions: https://api.openai.com/v1/chat/completions
package ai

// Usage captures token counts as returned by both providers (normalized).
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Pricing is the per-million-tokens cost of one model.
type Pricing struct {
	InputPerM  float64 // USD per 1M input tokens
	OutputPerM float64 // USD per 1M output tokens
}

// PricingTable mirrors enrich.js calcCost: 2 OpenAI + 2 Anthropic models.
// Add new models here as you change defaults.
var PricingTable = map[string]Pricing{
	"gpt-4o-mini":                {InputPerM: 0.15, OutputPerM: 0.60},
	"gpt-4o":                     {InputPerM: 2.50, OutputPerM: 10.00},
	"claude-sonnet-4-6":          {InputPerM: 3.00, OutputPerM: 15.00},
	"claude-haiku-4-5":           {InputPerM: 1.00, OutputPerM: 5.00},
	"claude-haiku-4-5-20251001":  {InputPerM: 1.00, OutputPerM: 5.00},
}

// CalcCost returns the USD cost of a single call with the given model and
// token usage. Unknown models fall back to Haiku-equivalent pricing.
func CalcCost(model string, u Usage) float64 {
	p, ok := PricingTable[model]
	if !ok {
		p = Pricing{InputPerM: 1.0, OutputPerM: 5.0}
	}
	return (float64(u.InputTokens)*p.InputPerM + float64(u.OutputTokens)*p.OutputPerM) / 1_000_000
}
