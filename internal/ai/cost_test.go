package ai

import (
	"math"
	"testing"
)

func TestCalcCost_Haiku(t *testing.T) {
	got := CalcCost("claude-haiku-4-5", Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000})
	want := 1.0 + 5.0 // 1M in @ $1 + 1M out @ $5
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestCalcCost_OpenAIMini(t *testing.T) {
	got := CalcCost("gpt-4o-mini", Usage{InputTokens: 1000, OutputTokens: 500})
	// 1000 * 0.15/1M + 500 * 0.60/1M = 0.00015 + 0.0003 = 0.00045
	want := 0.00045
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestCalcCost_UnknownModel_Fallback(t *testing.T) {
	got := CalcCost("unknown-model", Usage{InputTokens: 1_000_000, OutputTokens: 0})
	// Fallback pricing: 1.0 in, 5.0 out
	if math.Abs(got-1.0) > 1e-9 {
		t.Errorf("got %v want 1.0", got)
	}
}

func TestCalcCost_Zero(t *testing.T) {
	if got := CalcCost("gpt-4o-mini", Usage{}); got != 0 {
		t.Errorf("got %v want 0", got)
	}
}
