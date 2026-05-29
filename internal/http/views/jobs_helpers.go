package views

import "encoding/json"

// jobScore renders a nullable AI score as text ("—" when unscored).
func jobScore(score *int32) string {
	if score == nil {
		return "—"
	}
	return itoa(int(*score))
}

// jobScoreClass buckets a score into a CSS class for color-coding the badge.
func jobScoreClass(score *int32) string {
	if score == nil {
		return "score-none"
	}
	switch {
	case *score >= 75:
		return "score-high"
	case *score >= 50:
		return "score-mid"
	default:
		return "score-low"
	}
}

// jobTags decodes the ai_tags JSONB column (stored as []byte) into a slice.
func jobTags(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}
	var tags []string
	if err := json.Unmarshal(raw, &tags); err != nil {
		return nil
	}
	return tags
}
