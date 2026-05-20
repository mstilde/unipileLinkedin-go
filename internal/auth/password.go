package auth

import "golang.org/x/crypto/bcrypt"

// HashCost is the bcrypt cost factor. 12 is the recommended floor for 2026
// (≈250 ms/hash on commodity hardware).
const HashCost = 12

// HashPassword returns a bcrypt hash of the given password.
func HashPassword(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), HashCost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

// VerifyPassword reports whether password matches hash. Returns nil on match,
// bcrypt.ErrMismatchedHashAndPassword on mismatch, or another error if hash is
// malformed.
func VerifyPassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}
