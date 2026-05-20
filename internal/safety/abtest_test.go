package safety

import (
	"strconv"
	"testing"
)

func TestPickVariant_Empty(t *testing.T) {
	if got := PickVariant("s", "l", nil); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestPickVariant_AllZeroWeights_PicksFirst(t *testing.T) {
	vs := []Variant{
		{ID: "A", Weight: 0}, {ID: "B", Weight: 0},
	}
	v := PickVariant("s", "l", vs)
	if v.ID != "A" {
		t.Errorf("got %s want A", v.ID)
	}
}

func TestPickVariant_Deterministic(t *testing.T) {
	vs := []Variant{
		{ID: "A", Weight: 50}, {ID: "B", Weight: 50},
	}
	a := PickVariant("step-1", "lead-1", vs)
	b := PickVariant("step-1", "lead-1", vs)
	if a.ID != b.ID {
		t.Errorf("not deterministic: %s vs %s", a.ID, b.ID)
	}
}

func TestPickVariant_DifferentLeadsCanDiffer(t *testing.T) {
	vs := []Variant{
		{ID: "A", Weight: 50}, {ID: "B", Weight: 50},
	}
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		v := PickVariant("step-1", "lead-"+strconv.Itoa(i), vs)
		seen[v.ID] = true
	}
	// With 50 leads and 50/50 weights, we should hit both arms.
	if !seen["A"] || !seen["B"] {
		t.Errorf("expected both arms picked across 50 leads, got %v", seen)
	}
}

func TestPickVariant_RoughlyRespectsWeights(t *testing.T) {
	vs := []Variant{
		{ID: "A", Weight: 80}, {ID: "B", Weight: 20},
	}
	counts := map[string]int{}
	const N = 2000
	for i := 0; i < N; i++ {
		v := PickVariant("step-1", strconv.Itoa(i), vs)
		counts[v.ID]++
	}
	// Allow 10% slack on both sides (FNV is uniform enough)
	if counts["A"] < int(float64(N)*0.7) || counts["A"] > int(float64(N)*0.9) {
		t.Errorf("A count out of expected range [70%%, 90%%]: got %d/%d", counts["A"], N)
	}
}

func TestPickVariant_FallbackToLast_OnEdge(t *testing.T) {
	// Force a single variant
	vs := []Variant{{ID: "only", Weight: 100}}
	v := PickVariant("s", "l", vs)
	if v.ID != "only" {
		t.Errorf("got %s want only", v.ID)
	}
}

func TestPickVariant_NegativeWeightTreatedAsZero(t *testing.T) {
	vs := []Variant{
		{ID: "A", Weight: -10}, {ID: "B", Weight: 100},
	}
	// All lookups should end up at B (A's effective weight is 0).
	allB := true
	for i := 0; i < 50; i++ {
		v := PickVariant("s", strconv.Itoa(i), vs)
		if v.ID != "B" {
			allB = false
		}
	}
	if !allB {
		t.Error("negative weight should never be picked when alternatives exist")
	}
}
