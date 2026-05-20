package unipile

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// testClient builds a Client whose HTTP transport routes to the httptest server
// regardless of the (fake) DSN we configured. This lets us use https:// URLs
// in tests without a real TLS endpoint.
func testClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	c, err := NewClient("api1.unipile.test", "test-key", false)
	if err != nil {
		t.Fatal(err)
	}
	transport := &redirectTransport{target: srv.URL}
	httpC := &http.Client{Transport: transport}
	return c.WithHTTPClient(httpC)
}

// redirectTransport rewrites every request URL to point at `target` (the
// httptest server). Keeps the path intact so handlers can route.
type redirectTransport struct{ target string }

func (rt *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	u, err := url.Parse(rt.target)
	if err != nil {
		return nil, err
	}
	req.URL.Scheme = u.Scheme
	req.URL.Host = u.Host
	// httptest server uses HTTP not HTTPS — skip TLS verification on our side
	// just by routing through this transport's default Dialer.
	_ = tls.Config{}
	return http.DefaultTransport.RoundTrip(req)
}

// readBody is a test helper.
func readBody(t *testing.T, r io.Reader) string {
	t.Helper()
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestNewClient_Validates(t *testing.T) {
	if _, err := NewClient("", "k", false); err == nil {
		t.Error("empty dsn should error")
	}
	if _, err := NewClient("d", "", false); err == nil {
		t.Error("empty key should error")
	}
	c, err := NewClient("https://api.unipile.test/", "k", false)
	if err != nil {
		t.Fatal(err)
	}
	if c.dsn != "api.unipile.test" {
		t.Errorf("dsn not stripped, got %q", c.dsn)
	}
}

func TestSendInvitation_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/users/invite" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if r.Header.Get("X-API-KEY") != "test-key" {
			t.Errorf("X-API-KEY missing or wrong: %q", r.Header.Get("X-API-KEY"))
		}
		if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("Content-Type: got %q", ct)
		}

		var got map[string]any
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		if got["account_id"] != "acct-1" || got["provider_id"] != "PID-1" {
			t.Errorf("bad body: %v", got)
		}
		if got["message"] != "Hi" {
			t.Errorf("expected message=Hi, got %v", got["message"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"invitation_id":"inv-123"}`))
	}))
	defer srv.Close()

	c := testClient(t, srv)
	res, err := c.SendInvitation(context.Background(), SendInvitationParams{
		AccountID: "acct-1", ProviderID: "PID-1", Message: "Hi",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.InvitationID != "inv-123" {
		t.Errorf("got %q", res.InvitationID)
	}
}

func TestSendInvitation_NoteTruncated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got map[string]any
		_ = json.NewDecoder(r.Body).Decode(&got)
		msg, _ := got["message"].(string)
		if len(msg) != 10 {
			t.Errorf("expected 10-char truncated message, got len=%d", len(msg))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"invitation_id":"ok"}`))
	}))
	defer srv.Close()

	c := testClient(t, srv)
	_, err := c.SendInvitation(context.Background(), SendInvitationParams{
		AccountID: "a", ProviderID: "p", Message: strings.Repeat("x", 50), NoteLimit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSendInvitation_NoMessage_OmitsField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got map[string]any
		_ = json.NewDecoder(r.Body).Decode(&got)
		if _, ok := got["message"]; ok {
			t.Errorf("message should be omitted when empty, got: %v", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"invitation_id":"ok"}`))
	}))
	defer srv.Close()

	c := testClient(t, srv)
	_, err := c.SendInvitation(context.Background(), SendInvitationParams{AccountID: "a", ProviderID: "p"})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSendInvitation_LinkedInRateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(422)
		_, _ = w.Write([]byte(`{"status":422,"type":"errors/cannot_resend_yet","title":"weekly_invitation_limit"}`))
	}))
	defer srv.Close()

	c := testClient(t, srv)
	_, err := c.SendInvitation(context.Background(), SendInvitationParams{AccountID: "a", ProviderID: "p"})
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := AsAPIError(err)
	if !ok {
		t.Fatalf("expected APIError, got %T", err)
	}
	if !apiErr.IsLinkedInRateLimit() {
		t.Error("expected IsLinkedInRateLimit true")
	}
	if !apiErr.IsWeeklyCap() {
		t.Error("expected IsWeeklyCap true")
	}
}

func TestSendInvitation_Throttled_429(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		_, _ = w.Write([]byte(`{"title":"too many"}`))
	}))
	defer srv.Close()

	c := testClient(t, srv)
	_, err := c.SendInvitation(context.Background(), SendInvitationParams{AccountID: "a", ProviderID: "p"})
	apiErr, ok := AsAPIError(err)
	if !ok || !apiErr.IsThrottled() {
		t.Errorf("expected throttled, got %v", err)
	}
}

func TestSendInvitation_Transient_5xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
	}))
	defer srv.Close()

	c := testClient(t, srv)
	_, err := c.SendInvitation(context.Background(), SendInvitationParams{AccountID: "a", ProviderID: "p"})
	apiErr, ok := AsAPIError(err)
	if !ok || !apiErr.IsTransient() {
		t.Errorf("expected transient, got %v", err)
	}
}

func TestSendInvitation_Permanent_403(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		_, _ = w.Write([]byte(`{"title":"profile_not_found"}`))
	}))
	defer srv.Close()

	c := testClient(t, srv)
	_, err := c.SendInvitation(context.Background(), SendInvitationParams{AccountID: "a", ProviderID: "p"})
	apiErr, ok := AsAPIError(err)
	if !ok || !apiErr.IsPermanent() {
		t.Errorf("expected permanent, got %v", err)
	}
}

func TestSendInvitation_DryRun(t *testing.T) {
	c, _ := NewClient("dsn", "k", true) // dryRun=true
	res, err := c.SendInvitation(context.Background(), SendInvitationParams{AccountID: "a", ProviderID: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.DryRun {
		t.Error("expected DryRun=true")
	}
	if !strings.HasPrefix(res.InvitationID, "dry-run-") {
		t.Errorf("got %q", res.InvitationID)
	}
}

func TestSendInvitation_ValidatesInputs(t *testing.T) {
	c, _ := NewClient("dsn", "k", false)
	if _, err := c.SendInvitation(context.Background(), SendInvitationParams{}); err == nil {
		t.Error("expected error for empty params")
	}
	if _, err := c.SendInvitation(context.Background(), SendInvitationParams{AccountID: "a"}); err == nil {
		t.Error("expected error without provider_id")
	}
}

func TestSendMessage_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/chats/chat-99/messages" {
			t.Errorf("got path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message_id":"msg-42"}`))
	}))
	defer srv.Close()

	c := testClient(t, srv)
	res, err := c.SendMessage(context.Background(), SendMessageParams{ChatID: "chat-99", Text: "Hola"})
	if err != nil {
		t.Fatal(err)
	}
	if res.MessageID != "msg-42" {
		t.Errorf("got %q", res.MessageID)
	}
}

func TestSendMessage_ValidatesInputs(t *testing.T) {
	c, _ := NewClient("dsn", "k", false)
	if _, err := c.SendMessage(context.Background(), SendMessageParams{}); err == nil {
		t.Error("expected error without chat_id")
	}
	if _, err := c.SendMessage(context.Background(), SendMessageParams{ChatID: "c"}); err == nil {
		t.Error("expected error without text")
	}
	if _, err := c.SendMessage(context.Background(), SendMessageParams{ChatID: "c", Text: "   "}); err == nil {
		t.Error("expected error for whitespace-only text")
	}
}

func TestStartNewChat_MultipartFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			t.Errorf("expected multipart, got %q", r.Header.Get("Content-Type"))
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		if r.FormValue("account_id") != "acct-1" {
			t.Errorf("account_id got %q", r.FormValue("account_id"))
		}
		if r.FormValue("text") != "Hola!" {
			t.Errorf("text got %q", r.FormValue("text"))
		}
		if r.FormValue("attendees_ids") != "PID-1,PID-2" {
			t.Errorf("attendees_ids got %q", r.FormValue("attendees_ids"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"chat_id":"chat-new","message_id":"msg-new"}`))
	}))
	defer srv.Close()

	c := testClient(t, srv)
	res, err := c.StartNewChat(context.Background(), StartNewChatParams{
		AccountID: "acct-1", AttendeesIDs: []string{"PID-1", "PID-2"}, Text: "Hola!",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.ChatID != "chat-new" || res.MessageID != "msg-new" {
		t.Errorf("got %+v", res)
	}
}

func TestActionStubs_ReturnNotImplemented(t *testing.T) {
	c, _ := NewClient("dsn", "k", false)
	ctx := context.Background()
	cases := []func() error{
		func() error { return c.VisitProfile(ctx, "a", "p") },
		func() error { return c.Follow(ctx, "a", "p") },
		func() error { return c.LikePost(ctx, "a", "urn") },
		func() error { return c.CommentPost(ctx, "a", "urn", "txt") },
		func() error { return c.WithdrawInvite(ctx, "a", "inv") },
		func() error { return c.SendVoiceNote(ctx, "ch", "url") },
		func() error { return c.SendInMail(ctx, "a", "p", "s", "t") },
	}
	for i, fn := range cases {
		if err := fn(); !errors.Is(err, ErrNotImplemented) {
			t.Errorf("case %d: expected ErrNotImplemented, got %v", i, err)
		}
	}
}

func TestActionStubs_DryRunReturnsNil(t *testing.T) {
	c, _ := NewClient("dsn", "k", true)
	ctx := context.Background()
	if err := c.VisitProfile(ctx, "a", "p"); err != nil {
		t.Errorf("dry-run should not error: %v", err)
	}
}

func TestDo_BuildsURLCorrectly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Body should not be set on GET
		body := readBody(t, r.Body)
		if r.Method == http.MethodGet && body != "" {
			t.Errorf("GET should have no body, got %q", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := testClient(t, srv)
	var out map[string]any
	if err := c.do(context.Background(), http.MethodGet, "/api/v1/x", nil, &out); err != nil {
		t.Fatal(err)
	}
	if out["ok"] != true {
		t.Errorf("got %v", out)
	}
}
