package safety

import (
	"regexp"
	"strings"
)

// StopKeywords are opt-out triggers we auto-blacklist on. Matched with
// word-boundary semantics so "stopover" doesn't trigger "stop".
var StopKeywords = []string{
	"stop", "unsubscribe", "no me interesa", "no estoy interesado", "no estoy interesada",
	"quitame", "quítame", "quitar", "sacame de", "no contactar", "no me escribas",
	"remove me", "opt out", "opt-out",
}

// NormalizeURL canonicalizes a LinkedIn profile URL for blacklist matching:
//   - lowercase
//   - strip http(s)://
//   - strip www.
//   - drop trailing slash
//   - drop query string
func NormalizeURL(url string) string {
	if url == "" {
		return ""
	}
	s := strings.TrimSpace(strings.ToLower(url))
	for _, prefix := range []string{"https://www.", "http://www.", "https://", "http://", "www."} {
		if strings.HasPrefix(s, prefix) {
			s = s[len(prefix):]
			break
		}
	}
	s = strings.TrimRight(s, "/")
	if i := strings.IndexByte(s, '?'); i != -1 {
		s = s[:i]
	}
	return s
}

// NormalizeEmail returns the email lowercased and trimmed. Empty input → "".
func NormalizeEmail(email string) string {
	return strings.TrimSpace(strings.ToLower(email))
}

// DetectOptOutKeyword reports whether text contains any opt-out keyword with
// word-boundary semantics (so "stopover" doesn't match "stop").
func DetectOptOutKeyword(text string) bool {
	if text == "" {
		return false
	}
	lower := strings.TrimSpace(strings.ToLower(text))
	for _, kw := range StopKeywords {
		if lower == kw {
			return true
		}
		// Build a word-boundary regex around the keyword (escape any meta-chars).
		pattern := `(^|[^a-záéíóúñ])` + regexp.QuoteMeta(kw) + `([^a-záéíóúñ]|$)`
		if matched, _ := regexp.MatchString(pattern, lower); matched {
			return true
		}
	}
	return false
}

// BlacklistEntry is one row from the global_blacklist table (or the equivalent
// in memory). Either AccountID or it's the global scope (empty/nil).
type BlacklistEntry struct {
	AccountID        string
	LeadProviderID   string
	LeadLinkedInURL  string
	LeadEmail        string
	Reason           string
}

// LeadIdentity is the subset of prospect fields used for matching.
type LeadIdentity struct {
	ProviderID  string
	LinkedInURL string
	Email       string
}

// BlacklistMatch reports whether lead matches any entry, scoped to accountID
// or global (entry.AccountID == ""). All non-empty identifiers in lead are
// checked against the corresponding column.
//
// Returns the first matching entry (or nil).
func BlacklistMatch(entries []BlacklistEntry, accountID string, lead LeadIdentity) *BlacklistEntry {
	normURL := NormalizeURL(lead.LinkedInURL)
	normEmail := NormalizeEmail(lead.Email)

	for i := range entries {
		e := &entries[i]
		if e.AccountID != "" && e.AccountID != accountID {
			continue
		}
		if e.LeadProviderID != "" && lead.ProviderID != "" && e.LeadProviderID == lead.ProviderID {
			return e
		}
		if normURL != "" && NormalizeURL(e.LeadLinkedInURL) == normURL && e.LeadLinkedInURL != "" {
			return e
		}
		if normEmail != "" && NormalizeEmail(e.LeadEmail) == normEmail && e.LeadEmail != "" {
			return e
		}
	}
	return nil
}
