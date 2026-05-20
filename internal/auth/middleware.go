package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

type ctxKey int

const (
	ctxKeyClaims ctxKey = iota
)

// ClaimsFromContext returns the Claims placed into ctx by Authenticate.
// Returns (nil, false) if no claims are present (i.e. handler ran outside the
// auth middleware, which is a programming error).
func ClaimsFromContext(ctx context.Context) (*Claims, bool) {
	c, ok := ctx.Value(ctxKeyClaims).(*Claims)
	return c, ok
}

// AccountOwnershipStore looks up whether a user owns a given account.
// Implementations should be cheap (cache-friendly) — the middleware calls
// this on every request that hits a /accounts/{accountID} route.
type AccountOwnershipStore interface {
	UserOwnsAccount(ctx context.Context, userID int64, accountID string) (bool, error)
}

// Authenticate returns middleware that validates the bearer token in the
// Authorization header. On success it attaches Claims to the request context.
// Admins bypass nothing here — RequireAdmin enforces role separately.
func Authenticate(signer *Signer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenStr, ok := bearerToken(r)
			if !ok {
				writeJSONError(w, http.StatusUnauthorized, "missing bearer token")
				return
			}
			claims, err := signer.Parse(tokenStr)
			if err != nil {
				switch {
				case errors.Is(err, ErrTokenExpired):
					writeJSONError(w, http.StatusUnauthorized, "token expired")
				case errors.Is(err, ErrTokenWrongAudience),
					errors.Is(err, ErrTokenWrongIssuer):
					writeJSONError(w, http.StatusUnauthorized, "token rejected")
				default:
					writeJSONError(w, http.StatusUnauthorized, "token invalid")
				}
				return
			}
			ctx := context.WithValue(r.Context(), ctxKeyClaims, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAdmin returns middleware that 403s requests where the authenticated
// user is not an admin. Must run AFTER Authenticate.
func RequireAdmin() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := ClaimsFromContext(r.Context())
			if !ok {
				writeJSONError(w, http.StatusUnauthorized, "no auth context")
				return
			}
			if claims.Role != RoleAdmin {
				writeJSONError(w, http.StatusForbidden, "admin role required")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireOwnedAccount returns middleware that 403s requests where the chi URL
// parameter `accountID` is not owned by the authenticated user. Admins pass
// through (they can act on any account). Must run AFTER Authenticate.
//
// Example wiring:
//
//	r.Route("/api/accounts/{accountID}", func(r chi.Router) {
//	    r.Use(auth.RequireOwnedAccount(store))
//	    r.Get("/limits", h.getLimits)
//	})
func RequireOwnedAccount(store AccountOwnershipStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := ClaimsFromContext(r.Context())
			if !ok {
				writeJSONError(w, http.StatusUnauthorized, "no auth context")
				return
			}
			if claims.Role == RoleAdmin {
				next.ServeHTTP(w, r)
				return
			}
			accountID := chi.URLParam(r, "accountID")
			if accountID == "" {
				writeJSONError(w, http.StatusBadRequest, "missing accountID")
				return
			}
			owns, err := store.UserOwnsAccount(r.Context(), claims.UserID, accountID)
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, "ownership check failed")
				return
			}
			if !owns {
				writeJSONError(w, http.StatusForbidden, "account access denied")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// bearerToken extracts the token from "Authorization: Bearer <token>".
// Returns (token, true) on success, ("", false) when the header is missing,
// malformed, or uses a different scheme.
func bearerToken(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return "", false
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return "", false
	}
	tok := strings.TrimSpace(header[len(prefix):])
	if tok == "" {
		return "", false
	}
	return tok, true
}

// writeJSONError writes a JSON error response without dragging in encoding/json
// dependencies inside the hot path. Format: {"error":"msg"}.
func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"error":"` + jsonEscape(msg) + `"}`))
}

func jsonEscape(s string) string {
	// Cheap escape: quotes and backslashes. We control the inputs (literal
	// strings above), so this is sufficient.
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return r.Replace(s)
}
