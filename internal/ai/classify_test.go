package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsExplicitOptOut(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{"no me interesa, gracias", true},
		{"STOP por favor", true},
		{"unsubscribe me", true},
		{"hola que tal", false},
		{"", false},
	}
	for _, c := range cases {
		if got := IsExplicitOptOut(c.text); got != c.want {
			t.Errorf("%q: got %v want %v", c.text, got, c.want)
		}
	}
}

func TestIsAggressive(t *testing.T) {
	if !IsAggressive("Eres un BOT, deja de molestar") {
		t.Error("expected true")
	}
	if IsAggressive("Buenos días, gracias por escribir") {
		t.Error("expected false")
	}
}

func TestObviousPatterns_Thanks(t *testing.T) {
	if !ObviousPatterns[0].RE.MatchString("gracias!") {
		t.Error("should match 'gracias!'")
	}
}

func TestObviousPatterns_Greeting(t *testing.T) {
	for _, s := range []string{"hola", "Buenas", "Hi!", "Hey"} {
		if !ObviousPatterns[2].RE.MatchString(s) {
			t.Errorf("greeting did not match %q", s)
		}
	}
}

func TestObviousPatterns_Price(t *testing.T) {
	if !ObviousPatterns[4].RE.MatchString("¿cuánto cuesta?") {
		t.Error("price should match")
	}
	if !ObviousPatterns[4].RE.MatchString("nos pasarías una cotización") {
		t.Error("price should match cotización")
	}
}

func TestClassify_FastPath_OptOut(t *testing.T) {
	c, _ := NewAnthropicClient("test")
	cl := NewClassifier(c, ClassifyConfig{})
	out, err := cl.Classify(context.Background(), "no me interesa, gracias", nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Intent != IntentNotInterested || out.FastDetected != "opt_out" {
		t.Errorf("got %+v", out)
	}
	if out.Confidence != 1.0 {
		t.Errorf("confidence got %v", out.Confidence)
	}
}

func TestClassify_FastPath_Aggressive(t *testing.T) {
	c, _ := NewAnthropicClient("test")
	cl := NewClassifier(c, ClassifyConfig{})
	out, err := cl.Classify(context.Background(), "eres un bot, déjame en paz", nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Intent != IntentAggressive {
		t.Errorf("got intent %v", out.Intent)
	}
}

func TestClassify_FastPath_ObviousThanks(t *testing.T) {
	c, _ := NewAnthropicClient("test")
	cl := NewClassifier(c, ClassifyConfig{})
	out, err := cl.Classify(context.Background(), "gracias!", nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.FastDetected != "thanks" {
		t.Errorf("got fast=%q", out.FastDetected)
	}
}

func TestClassify_FastPath_Suppressed_WhenContextSubstantial(t *testing.T) {
	// "gracias!" alone would shortcut. But if prior prospect message had >15 chars,
	// we should fall through to LLM.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"m1","model":"claude-haiku-4-5","role":"assistant","type":"message",
			"content":[{"type":"text","text":"{\"intent\":\"interest\",\"confidence\":0.8,\"temperature\":\"warm\",\"message_type\":\"open_door\",\"raw_question\":null}"}],
			"stop_reason":"end_turn",
			"usage":{"input_tokens":50,"output_tokens":20}
		}`))
	}))
	defer srv.Close()

	c, _ := NewAnthropicClient("test")
	c = c.WithBaseURL(srv.URL)
	cl := NewClassifier(c, ClassifyConfig{})

	hist := []HistoryMessage{
		{Text: "claro que sí, me late mucho la idea, contame más", IsSender: false},
	}
	out, err := cl.Classify(context.Background(), "gracias!", hist)
	if err != nil {
		t.Fatal(err)
	}
	if out.FastDetected != "" {
		t.Errorf("expected no fast detection, got %q", out.FastDetected)
	}
	if out.Intent != IntentInterest {
		t.Errorf("intent got %v", out.Intent)
	}
}

func TestClassify_LLM_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("path %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "test-key" {
			t.Error("missing x-api-key")
		}
		if r.Header.Get("anthropic-version") == "" {
			t.Error("missing anthropic-version")
		}
		var req MessagesRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Model != "claude-haiku-4-5-20251001" {
			t.Errorf("model got %s", req.Model)
		}
		if len(req.System) == 0 || req.System[0].CacheControl == nil {
			t.Error("expected cache_control")
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"m1","model":"claude-haiku-4-5","role":"assistant","type":"message",
			"content":[{"type":"text","text":"{\"intent\":\"interest\",\"confidence\":0.85,\"temperature\":\"hot\",\"message_type\":\"open_door\",\"raw_question\":null}"}],
			"stop_reason":"end_turn",
			"usage":{"input_tokens":120,"output_tokens":30}
		}`))
	}))
	defer srv.Close()

	c, _ := NewAnthropicClient("test-key")
	c = c.WithBaseURL(srv.URL)
	cl := NewClassifier(c, ClassifyConfig{})

	out, err := cl.Classify(context.Background(), "qué tienes en mente?", nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Intent != IntentInterest || out.Temperature != "hot" {
		t.Errorf("got %+v", out)
	}
	if out.Usage.InputTokens != 120 {
		t.Errorf("usage got %+v", out.Usage)
	}
	if out.CostUSD == 0 {
		t.Errorf("cost should be >0")
	}
}

func TestClassify_LLM_FallbackToSonnet_OnError(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var req MessagesRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if strings.HasPrefix(req.Model, "claude-haiku") {
			// All Haiku calls fail with 500 (retryable, so client retries 3x)
			w.WriteHeader(500)
			_, _ = w.Write([]byte(`{"error":"fail"}`))
			return
		}
		// Sonnet succeeds
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"content":[{"type":"text","text":"{\"intent\":\"other\",\"confidence\":0.5,\"temperature\":\"warm\",\"message_type\":\"passive\",\"raw_question\":null}"}],
			"usage":{"input_tokens":10,"output_tokens":5}
		}`))
	}))
	defer srv.Close()

	c, _ := NewAnthropicClient("test")
	c = c.WithBaseURL(srv.URL)
	cl := NewClassifier(c, ClassifyConfig{})

	out, err := cl.Classify(context.Background(), "saludos cordiales prospect cuéntame más", nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Model != "claude-sonnet-4-6" {
		t.Errorf("expected fallback to sonnet, got model=%s", out.Model)
	}
	if calls < 4 { // 3 haiku retries + 1 sonnet success
		t.Errorf("expected >=4 calls, got %d", calls)
	}
}

func TestParseClassifyJSON_HandlesProse(t *testing.T) {
	text := "Here is my analysis:\n{\"intent\":\"interest\",\"confidence\":0.7,\"temperature\":\"warm\",\"message_type\":\"open_door\"}\nThat's all."
	got := parseClassifyJSON(text)
	if got.Intent != "interest" || got.Confidence != 0.7 {
		t.Errorf("got %+v", got)
	}
}
