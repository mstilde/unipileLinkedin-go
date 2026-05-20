package safety

import "hash/fnv"

// Variant is one arm of an A/B test step.
type Variant struct {
	ID       string
	Weight   float64 // ≥0; if all weights are 0 the first variant wins
	Template string
}

// PickVariant deterministically picks one variant for the (stepID, leadID)
// pair using FNV-1a hashing. Same inputs always return the same variant
// (idempotent), but different (stepID, leadID) pairs distribute uniformly
// across variants according to their weights.
//
// Returns nil if variants is empty.
func PickVariant(stepID, leadID string, variants []Variant) *Variant {
	if len(variants) == 0 {
		return nil
	}

	total := 0.0
	for _, v := range variants {
		if v.Weight > 0 {
			total += v.Weight
		}
	}
	if total <= 0 {
		v := variants[0]
		return &v
	}

	h := fnv.New32a()
	_, _ = h.Write([]byte(stepID))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(leadID))
	r := float64(h.Sum32()) / float64(1<<32) // [0, 1)

	acc := 0.0
	for i := range variants {
		w := variants[i].Weight
		if w < 0 {
			w = 0
		}
		acc += w / total
		if r < acc {
			v := variants[i]
			return &v
		}
	}
	v := variants[len(variants)-1]
	return &v
}
