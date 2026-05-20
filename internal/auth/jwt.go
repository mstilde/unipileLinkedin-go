package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Role is the access-control level of a user.
type Role string

const (
	RoleAdmin  Role = "admin"
	RoleWorker Role = "worker"
)

// Claims is the JWT payload. Extends RegisteredClaims so iss/aud/exp/iat are
// validated by the jwt library automatically.
type Claims struct {
	UserID int64 `json:"uid"`
	Role   Role  `json:"role"`
	jwt.RegisteredClaims
}

// Signer signs and parses JWTs with a shared HMAC secret + issuer + audience.
// All three values must be configured before use; instances are safe for
// concurrent use after construction.
type Signer struct {
	secret   []byte
	issuer   string
	audience string
	ttl      time.Duration
	now      func() time.Time // overridable in tests
}

// NewSigner constructs a Signer. ttl is the session lifetime baked into iat/exp.
// Returns error if secret is shorter than 32 bytes.
func NewSigner(secret, issuer, audience string, ttl time.Duration) (*Signer, error) {
	if len(secret) < 32 {
		return nil, errors.New("auth: JWT secret must be at least 32 bytes")
	}
	if issuer == "" || audience == "" {
		return nil, errors.New("auth: JWT issuer and audience are required")
	}
	if ttl <= 0 {
		return nil, errors.New("auth: JWT ttl must be positive")
	}
	return &Signer{
		secret:   []byte(secret),
		issuer:   issuer,
		audience: audience,
		ttl:      ttl,
		now:      time.Now,
	}, nil
}

// Sign produces a signed JWT for the given user.
func (s *Signer) Sign(userID int64, role Role) (string, error) {
	now := s.now()
	claims := Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			Audience:  jwt.ClaimStrings{s.audience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.ttl)),
			NotBefore: jwt.NewNumericDate(now),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(s.secret)
}

// Parse validates the token's signature, expiration, issuer, and audience.
// Returns the parsed claims on success. Errors are wrapped — use errors.Is with
// ErrTokenExpired, ErrTokenInvalid, ErrTokenWrongAudience, ErrTokenWrongIssuer
// for branching.
func (s *Signer) Parse(tokenStr string) (*Claims, error) {
	claims := &Claims{}
	tok, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("%w: unexpected signing method %v", ErrTokenInvalid, t.Method.Alg())
		}
		return s.secret, nil
	}, jwt.WithIssuer(s.issuer), jwt.WithAudience(s.audience), jwt.WithTimeFunc(s.now))
	if err != nil {
		switch {
		case errors.Is(err, jwt.ErrTokenExpired):
			return nil, ErrTokenExpired
		case errors.Is(err, jwt.ErrTokenInvalidAudience):
			return nil, ErrTokenWrongAudience
		case errors.Is(err, jwt.ErrTokenInvalidIssuer):
			return nil, ErrTokenWrongIssuer
		default:
			return nil, fmt.Errorf("%w: %v", ErrTokenInvalid, err)
		}
	}
	if !tok.Valid {
		return nil, ErrTokenInvalid
	}
	return claims, nil
}

// ErrTokenExpired is returned when the JWT exp is in the past.
var ErrTokenExpired = errors.New("auth: token expired")

// ErrTokenInvalid is returned for any signature / format / nbf failure.
var ErrTokenInvalid = errors.New("auth: token invalid")

// ErrTokenWrongAudience is returned when the JWT aud claim does not match.
var ErrTokenWrongAudience = errors.New("auth: token wrong audience")

// ErrTokenWrongIssuer is returned when the JWT iss claim does not match.
var ErrTokenWrongIssuer = errors.New("auth: token wrong issuer")
