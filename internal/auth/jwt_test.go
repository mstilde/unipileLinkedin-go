package auth

import (
	"errors"
	"strings"
	"testing"
	"time"
)

const testSecret = "0123456789abcdef0123456789abcdef" // 32 bytes

func mustSigner(t *testing.T) *Signer {
	t.Helper()
	s, err := NewSigner(testSecret, "test-iss", "test-aud", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestNewSigner_RejectsShortSecret(t *testing.T) {
	_, err := NewSigner("short", "iss", "aud", time.Hour)
	if err == nil || !strings.Contains(err.Error(), "at least 32") {
		t.Errorf("expected length error, got %v", err)
	}
}

func TestNewSigner_RejectsZeroTTL(t *testing.T) {
	_, err := NewSigner(testSecret, "iss", "aud", 0)
	if err == nil {
		t.Error("expected error for zero ttl")
	}
}

func TestNewSigner_RequiresIssuerAndAudience(t *testing.T) {
	if _, err := NewSigner(testSecret, "", "aud", time.Hour); err == nil {
		t.Error("expected issuer error")
	}
	if _, err := NewSigner(testSecret, "iss", "", time.Hour); err == nil {
		t.Error("expected audience error")
	}
}

func TestSignParse_Roundtrip(t *testing.T) {
	s := mustSigner(t)
	tok, err := s.Sign(42, RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := s.Parse(tok)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if claims.UserID != 42 {
		t.Errorf("UserID: got %d want 42", claims.UserID)
	}
	if claims.Role != RoleAdmin {
		t.Errorf("Role: got %v want admin", claims.Role)
	}
	if claims.Issuer != "test-iss" {
		t.Errorf("Issuer: got %q", claims.Issuer)
	}
}

func TestParse_RejectsExpired(t *testing.T) {
	s := mustSigner(t)
	// Sign with "now" 2h ago — token already expired at real "now".
	twoHoursAgo := time.Now().Add(-2 * time.Hour)
	s.now = func() time.Time { return twoHoursAgo }
	tok, _ := s.Sign(1, RoleWorker)

	// Restore real clock for parse.
	s.now = time.Now
	_, err := s.Parse(tok)
	if !errors.Is(err, ErrTokenExpired) {
		t.Errorf("expected ErrTokenExpired, got %v", err)
	}
}

func TestParse_RejectsTamperedSignature(t *testing.T) {
	s := mustSigner(t)
	tok, _ := s.Sign(1, RoleWorker)

	// Mutate the last char of the signature segment.
	last := tok[len(tok)-1]
	flipped := byte('A')
	if last == 'A' {
		flipped = 'B'
	}
	tampered := tok[:len(tok)-1] + string(flipped)

	_, err := s.Parse(tampered)
	if !errors.Is(err, ErrTokenInvalid) {
		t.Errorf("expected ErrTokenInvalid, got %v", err)
	}
}

func TestParse_RejectsWrongAudience(t *testing.T) {
	signer1, _ := NewSigner(testSecret, "iss", "audA", time.Hour)
	signer2, _ := NewSigner(testSecret, "iss", "audB", time.Hour)
	tok, _ := signer1.Sign(1, RoleWorker)

	_, err := signer2.Parse(tok)
	if !errors.Is(err, ErrTokenWrongAudience) {
		t.Errorf("expected ErrTokenWrongAudience, got %v", err)
	}
}

func TestParse_RejectsWrongIssuer(t *testing.T) {
	signer1, _ := NewSigner(testSecret, "issA", "aud", time.Hour)
	signer2, _ := NewSigner(testSecret, "issB", "aud", time.Hour)
	tok, _ := signer1.Sign(1, RoleWorker)

	_, err := signer2.Parse(tok)
	if !errors.Is(err, ErrTokenWrongIssuer) {
		t.Errorf("expected ErrTokenWrongIssuer, got %v", err)
	}
}

func TestParse_RejectsGarbage(t *testing.T) {
	s := mustSigner(t)
	_, err := s.Parse("not.a.jwt")
	if !errors.Is(err, ErrTokenInvalid) {
		t.Errorf("expected ErrTokenInvalid, got %v", err)
	}
}
