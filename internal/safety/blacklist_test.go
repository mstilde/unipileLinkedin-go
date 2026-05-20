package safety

import "testing"

func TestNormalizeURL(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"https://www.linkedin.com/in/juan/", "linkedin.com/in/juan"},
		{"http://linkedin.com/in/juan", "linkedin.com/in/juan"},
		{"www.linkedin.com/in/juan/", "linkedin.com/in/juan"},
		{"https://linkedin.com/in/juan?foo=bar", "linkedin.com/in/juan"},
		{"linkedin.com/in/JUAN/", "linkedin.com/in/juan"},
		{"", ""},
	}
	for _, c := range cases {
		if got := NormalizeURL(c.in); got != c.want {
			t.Errorf("%q → %q, want %q", c.in, got, c.want)
		}
	}
}

func TestDetectOptOutKeyword(t *testing.T) {
	hits := []string{
		"stop",
		"STOP por favor",
		"por favor stop.",
		"no me interesa, gracias",
		"unsubscribe me please",
		"sacame de tu lista",
		"opt-out now",
	}
	for _, s := range hits {
		if !DetectOptOutKeyword(s) {
			t.Errorf("expected hit: %q", s)
		}
	}

	misses := []string{
		"stopover en madrid",   // "stop" inside a word — should NOT match
		"hola que tal",
		"",
		"gracias por escribir",
	}
	for _, s := range misses {
		if DetectOptOutKeyword(s) {
			t.Errorf("expected miss: %q", s)
		}
	}
}

func TestBlacklistMatch_ByProviderID(t *testing.T) {
	entries := []BlacklistEntry{
		{LeadProviderID: "ACoAA123", Reason: "opted_out"},
	}
	hit := BlacklistMatch(entries, "acct-1", LeadIdentity{ProviderID: "ACoAA123"})
	if hit == nil || hit.Reason != "opted_out" {
		t.Errorf("expected hit, got %v", hit)
	}
}

func TestBlacklistMatch_ByURL_Normalized(t *testing.T) {
	entries := []BlacklistEntry{
		{LeadLinkedInURL: "https://www.linkedin.com/in/juan/"},
	}
	hit := BlacklistMatch(entries, "acct-1", LeadIdentity{LinkedInURL: "linkedin.com/in/juan?utm=x"})
	if hit == nil {
		t.Error("expected hit after URL normalization")
	}
}

func TestBlacklistMatch_ByEmail_CaseInsensitive(t *testing.T) {
	entries := []BlacklistEntry{
		{LeadEmail: "JUAN@example.com"},
	}
	hit := BlacklistMatch(entries, "acct-1", LeadIdentity{Email: "juan@example.com"})
	if hit == nil {
		t.Error("expected hit (email case-insensitive)")
	}
}

func TestBlacklistMatch_AccountScopedMiss(t *testing.T) {
	entries := []BlacklistEntry{
		{AccountID: "acct-other", LeadProviderID: "X"},
	}
	hit := BlacklistMatch(entries, "acct-1", LeadIdentity{ProviderID: "X"})
	if hit != nil {
		t.Error("entry scoped to different account should not match")
	}
}

func TestBlacklistMatch_GlobalScopeAlwaysHits(t *testing.T) {
	entries := []BlacklistEntry{
		{AccountID: "", LeadProviderID: "X", Reason: "global"},
	}
	hit := BlacklistMatch(entries, "acct-1", LeadIdentity{ProviderID: "X"})
	if hit == nil || hit.Reason != "global" {
		t.Errorf("expected global hit, got %v", hit)
	}
}

func TestBlacklistMatch_NoMatch(t *testing.T) {
	entries := []BlacklistEntry{
		{LeadProviderID: "X"},
	}
	hit := BlacklistMatch(entries, "acct-1", LeadIdentity{ProviderID: "Y", Email: "a@b.com"})
	if hit != nil {
		t.Errorf("expected nil, got %v", hit)
	}
}
