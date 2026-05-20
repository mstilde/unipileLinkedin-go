package auth

import (
	"errors"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestHashAndVerify_Roundtrip(t *testing.T) {
	hash, err := HashPassword("hunter2")
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyPassword(hash, "hunter2"); err != nil {
		t.Errorf("expected match, got %v", err)
	}
}

func TestVerify_WrongPassword(t *testing.T) {
	hash, _ := HashPassword("hunter2")
	err := VerifyPassword(hash, "wrongpass")
	if !errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		t.Errorf("expected mismatch error, got %v", err)
	}
}

func TestVerify_MalformedHash(t *testing.T) {
	if err := VerifyPassword("not-a-hash", "anything"); err == nil {
		t.Error("expected error for malformed hash")
	}
}

func TestHash_DifferentEveryTime(t *testing.T) {
	// bcrypt embeds a random salt; two hashes of the same password must differ.
	h1, _ := HashPassword("same")
	h2, _ := HashPassword("same")
	if h1 == h2 {
		t.Error("expected different hashes (salt should vary)")
	}
}
