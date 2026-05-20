package unipile

import (
	"errors"
	"testing"
)

func TestParseAPIError_KnownType(t *testing.T) {
	body := []byte(`{"status":422,"type":"errors/cannot_resend_yet","title":"cannot resend","detail":"wait"}`)
	e := parseAPIError(422, body)
	if e.Status != 422 {
		t.Errorf("status got %d", e.Status)
	}
	if e.Type != "errors/cannot_resend_yet" {
		t.Errorf("type got %q", e.Type)
	}
	if e.Title != "cannot resend" {
		t.Errorf("title got %q", e.Title)
	}
}

func TestParseAPIError_InferPremiumRequired(t *testing.T) {
	body := []byte(`{"status":400,"title":"sales_navigator required for inmail"}`)
	e := parseAPIError(400, body)
	if e.Type != "premium_required" {
		t.Errorf("expected inferred premium_required, got %q", e.Type)
	}
}

func TestParseAPIError_InferAccountDisconnected(t *testing.T) {
	body := []byte(`{"status":401,"title":"credentials expired, please reauth"}`)
	e := parseAPIError(401, body)
	if e.Type != "account_disconnected" {
		t.Errorf("expected inferred account_disconnected, got %q", e.Type)
	}
}

func TestIsLinkedInRateLimit_422(t *testing.T) {
	e := &APIError{Status: 422, Type: "errors/cannot_resend_yet"}
	if !e.IsLinkedInRateLimit() {
		t.Error("expected true for 422")
	}
}

func TestIsLinkedInRateLimit_FromBlob(t *testing.T) {
	e := &APIError{Status: 200, Title: "weekly_invitation_limit reached"}
	if !e.IsLinkedInRateLimit() {
		t.Error("expected true from title blob")
	}
}

func TestIsWeeklyCap(t *testing.T) {
	e := &APIError{Status: 422, Type: "errors/cannot_resend_yet", Title: "weekly_invitation_limit reached"}
	if !e.IsWeeklyCap() {
		t.Error("expected true for weekly cap")
	}
}

func TestIsThrottled_429(t *testing.T) {
	e := &APIError{Status: 429}
	if !e.IsThrottled() {
		t.Error("expected true for 429")
	}
}

func TestIsTransient_5xx(t *testing.T) {
	for _, s := range []int{500, 502, 503, 504} {
		e := &APIError{Status: s}
		if !e.IsTransient() {
			t.Errorf("status %d: expected transient", s)
		}
	}
}

func TestIsTransient_Not5xx(t *testing.T) {
	for _, s := range []int{200, 400, 422, 429, 600} {
		e := &APIError{Status: s}
		if e.IsTransient() {
			t.Errorf("status %d: should not be transient", s)
		}
	}
}

func TestIsPermanent_StatusCodes(t *testing.T) {
	for _, s := range []int{403, 404, 451} {
		e := &APIError{Status: s}
		if !e.IsPermanent() {
			t.Errorf("status %d: expected permanent", s)
		}
	}
}

func TestIsPermanent_ByType(t *testing.T) {
	for _, ty := range []string{"premium_required", "inmail_credit_exhausted", "account_disconnected", "invalid_provider_id"} {
		e := &APIError{Status: 400, Type: ty}
		if !e.IsPermanent() {
			t.Errorf("type %q: expected permanent", ty)
		}
	}
}

func TestIsPermanent_ByBlob(t *testing.T) {
	e := &APIError{Status: 400, Title: "private_profile blocked"}
	if !e.IsPermanent() {
		t.Error("expected permanent from blob")
	}
}

func TestAsAPIError(t *testing.T) {
	e := &APIError{Status: 500, Title: "boom"}
	err := error(e)
	if got, ok := AsAPIError(err); !ok || got.Status != 500 {
		t.Errorf("got %v %v", got, ok)
	}
	if _, ok := AsAPIError(errors.New("other")); ok {
		t.Error("expected false for non-APIError")
	}
}
