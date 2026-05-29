package views

// truncate shortens s to at most n runes, appending an ellipsis when cut.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// feedRelevant renders the nullable ai_relevant flag as a short label.
func feedRelevant(rel *bool) string {
	if rel == nil {
		return "—"
	}
	if *rel {
		return "relevant"
	}
	return "no"
}

// feedRelevantClass color-codes the relevance badge.
func feedRelevantClass(rel *bool) string {
	if rel == nil {
		return "score-none"
	}
	if *rel {
		return "score-high"
	}
	return "score-low"
}
