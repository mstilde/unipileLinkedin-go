package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPIILeaks_NoLeakWhenAuthorized(t *testing.T) {
	auth := "Mi calendario: https://cal.com/mati y mi tel +52 55 1234 5678 hello@x.com"
	reply := "Acá tienes mi cal: https://cal.com/mati - escríbeme a hello@x.com"
	leaks := piiLeaks(reply, auth)
	if len(leaks) != 0 {
		t.Errorf("expected no leaks, got %v", leaks)
	}
}

func TestPIILeaks_DetectsInventedEmail(t *testing.T) {
	auth := "Mi calendario: https://cal.com/mati"
	reply := "Escribime a contacto@inventado.com"
	leaks := piiLeaks(reply, auth)
	if len(leaks) != 1 || !strings.HasPrefix(leaks[0], "email:") {
		t.Errorf("expected one email leak, got %v", leaks)
	}
}

func TestPIILeaks_DetectsInventedPhone(t *testing.T) {
	auth := "Sin teléfono"
	reply := "Mi número es +52 55 9999 8888"
	leaks := piiLeaks(reply, auth)
	if len(leaks) != 1 || !strings.HasPrefix(leaks[0], "phone:") {
		t.Errorf("expected one phone leak, got %v", leaks)
	}
}

func TestPIILeaks_PhoneNormalized(t *testing.T) {
	// Authorized has the number with dashes, reply uses spaces. Should not leak.
	auth := "Mi tel es +52-55-1234-5678"
	reply := "Hablamos al +52 55 1234 5678 mañana"
	leaks := piiLeaks(reply, auth)
	if len(leaks) != 0 {
		t.Errorf("formatting differences shouldn't trigger leak, got %v", leaks)
	}
}

func TestBuildReplySystemPrompt_IncludesPersona(t *testing.T) {
	p := Persona{
		SystemPrompt: "Soy Mati, consultor.",
		GuionesJSON: map[string]string{
			"pide_precio": "respondé con rango $5k-$10k",
		},
	}
	sp := buildReplySystemPrompt(p)
	if !strings.Contains(sp, "Soy Mati, consultor.") {
		t.Error("missing persona system prompt")
	}
	if !strings.Contains(sp, "respondé con rango") {
		t.Error("missing guion")
	}
	if !strings.Contains(sp, "NUNCA termines una oración con punto final") {
		t.Error("missing style rules")
	}
}

func TestBuildReplySystemPrompt_NoPersona_HasDefault(t *testing.T) {
	sp := buildReplySystemPrompt(Persona{})
	if !strings.Contains(sp, "representante comercial profesional") {
		t.Error("missing default system prompt")
	}
	if !strings.Contains(sp, "sin guiones configurados") {
		t.Error("missing default guiones")
	}
}

func TestBuildReplyUserContent_IncludesAllFields(t *testing.T) {
	p := Prospect{FirstName: "Ana", Company: "Acme", Headline: "CEO"}
	hist := []HistoryMessage{
		{Text: "Hola", IsSender: true},
		{Text: "Qué onda", IsSender: false},
	}
	out := buildReplyUserContent(p, hist, IntentInterest, "Me interesa")
	for _, s := range []string{"Ana", "Acme", "CEO", "interest", "Me interesa", "Nosotros: Hola", "Prospect: Qué onda"} {
		if !strings.Contains(out, s) {
			t.Errorf("missing %q in user content", s)
		}
	}
}

func TestGenerateReply_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"content":[{"type":"text","text":"perfecto Ana, te paso el calendario mañana"}],
			"usage":{"input_tokens":200,"output_tokens":15}
		}`))
	}))
	defer srv.Close()

	c, _ := NewAnthropicClient("test")
	c = c.WithBaseURL(srv.URL)
	g := NewReplyGenerator(c, "", "")

	out, err := g.GenerateReply(
		context.Background(),
		Persona{SystemPrompt: "soy Mati"},
		Prospect{FirstName: "Ana"},
		nil,
		IntentInterest,
		"qué tienes en mente?",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Text, "Ana") {
		t.Errorf("got %q", out.Text)
	}
	if out.PIIBlocked {
		t.Error("should not be PII blocked")
	}
	if out.CostUSD == 0 {
		t.Error("cost should be >0")
	}
}

func TestGenerateReply_PIIBlocked_OnInventedPhone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"content":[{"type":"text","text":"perfecto, mi tel es +52 55 0000 1234"}],
			"usage":{"input_tokens":50,"output_tokens":10}
		}`))
	}))
	defer srv.Close()

	c, _ := NewAnthropicClient("test")
	c = c.WithBaseURL(srv.URL)
	g := NewReplyGenerator(c, "", "")

	out, err := g.GenerateReply(
		context.Background(),
		Persona{SystemPrompt: "soy Mati, mi calendario es cal.com/mati"},
		Prospect{FirstName: "Ana"},
		nil,
		IntentInterest,
		"pasame un tel",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !out.PIIBlocked {
		t.Error("expected PII blocked")
	}
	if len(out.PIILeaks) != 1 || !strings.HasPrefix(out.PIILeaks[0], "phone:") {
		t.Errorf("expected phone leak, got %v", out.PIILeaks)
	}
}
