package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseEnrichJSON_Valid(t *testing.T) {
	text := `{"icebreaker":"vi que andas en IoT","pain_points":["a","b"],"communication_style":"casual","icp_fit_score":8,"icp_fit_reason":"rol fit","clean_position":"CEO","clean_company":"Acme","clean_school":null,"clean_industry":"Tecnología"}`
	got, err := parseEnrichJSON(text)
	if err != nil {
		t.Fatal(err)
	}
	if got.Icebreaker != "vi que andas en IoT" || got.ICPFitScore != 8 {
		t.Errorf("got %+v", got)
	}
	if got.CleanCompany == nil || *got.CleanCompany != "Acme" {
		t.Errorf("clean_company got %v", got.CleanCompany)
	}
	if got.CleanSchool != nil {
		t.Errorf("clean_school should be nil, got %v", *got.CleanSchool)
	}
}

func TestParseEnrichJSON_WithProse(t *testing.T) {
	text := "Aquí está el análisis:\n```json\n{\"icebreaker\":\"x\",\"pain_points\":[],\"communication_style\":\"formal\",\"icp_fit_score\":5,\"icp_fit_reason\":\"y\"}\n```"
	got, err := parseEnrichJSON(text)
	if err != nil {
		t.Fatal(err)
	}
	if got.Icebreaker != "x" {
		t.Errorf("got %+v", got)
	}
}

func TestParseEnrichJSON_NoJSON_Fallback(t *testing.T) {
	text := "Lo siento, no puedo analizar"
	got, err := parseEnrichJSON(text)
	if err == nil {
		t.Error("expected parse error")
	}
	if got.CommunicationStyle != "semiformal" || got.ICPFitScore != 5 {
		t.Errorf("expected fallback, got %+v", got)
	}
}

func TestParseEnrichJSON_DefaultsApplied(t *testing.T) {
	// Missing communication_style and pain_points
	text := `{"icebreaker":"hi","icp_fit_score":7,"icp_fit_reason":"r"}`
	got, err := parseEnrichJSON(text)
	if err != nil {
		t.Fatal(err)
	}
	if got.CommunicationStyle != "semiformal" {
		t.Errorf("default communication_style not applied, got %q", got.CommunicationStyle)
	}
	if got.PainPoints == nil {
		t.Error("PainPoints should default to empty slice")
	}
}

func TestEnrich_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing/wrong bearer")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"c1",
			"choices":[{"index":0,"message":{"role":"assistant","content":"{\"icebreaker\":\"vi tu post\",\"pain_points\":[\"falta tiempo\"],\"communication_style\":\"casual\",\"icp_fit_score\":8,\"icp_fit_reason\":\"buen fit\",\"clean_position\":\"CTO\",\"clean_company\":\"Acme\",\"clean_school\":null,\"clean_industry\":\"Tecnología\"}"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":300,"completion_tokens":80,"total_tokens":380}
		}`))
	}))
	defer srv.Close()

	c, _ := NewOpenAIClient("test-key")
	c = c.WithBaseURL(srv.URL)
	e := NewEnricher(c, "")

	res, err := e.Enrich(context.Background(), EnrichmentInput{
		FullName: "Ana Pérez",
		Headline: "CTO at Acme",
		Company:  "Acme",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Enrichment.Icebreaker != "vi tu post" {
		t.Errorf("got %+v", res.Enrichment)
	}
	if res.Usage.InputTokens != 300 || res.Usage.OutputTokens != 80 {
		t.Errorf("usage got %+v", res.Usage)
	}
	if res.CostUSD == 0 {
		t.Error("cost should be >0")
	}
}

func TestBuildEnrichPrompt_TruncatesLargeFields(t *testing.T) {
	in := EnrichmentInput{
		FullName:       "X",
		WorkExperience: strings.Repeat("a", 5000),
		Education:      strings.Repeat("b", 5000),
	}
	prompt := buildEnrichPrompt(in)
	if len(prompt) > 8000 {
		// Sanity: should fit in well under 8k chars even with 5k input.
		t.Errorf("prompt unexpectedly long: %d chars", len(prompt))
	}
	// Confirm truncation happened: full 5k strings should not appear.
	if strings.Contains(prompt, strings.Repeat("a", 1000)) {
		t.Error("expected work_experience to be truncated")
	}
}

func TestFallbackEnrichment(t *testing.T) {
	f := FallbackEnrichment("test reason")
	if f.ICPFitScore != 5 || f.CommunicationStyle != "semiformal" {
		t.Errorf("bad defaults: %+v", f)
	}
	if f.ICPFitReason != "test reason" {
		t.Error("reason not set")
	}
}
