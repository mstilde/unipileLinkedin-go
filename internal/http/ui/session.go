// Package ui serves the server-rendered HTML pages backed by templ + HTMX.
package ui

import (
	"context"
	"net/http"
	"time"

	"github.com/mstilde/unipile-linkedin-go/internal/auth"
)

const sessionCookieName = "unipile_session"

type ctxKey int

const ctxKeyClaims ctxKey = 1

// setSession writes the JWT into an HttpOnly cookie scoped to the whole app.
// The TTL matches the JWT's own TTL.
func setSession(w http.ResponseWriter, token string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(ttl.Seconds()),
	})
}

// clearSession deletes the cookie. Used by /logout.
func clearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// requireSession is HTTP middleware: parses the JWT cookie via signer and
// attaches Claims to ctx. Anonymous requests are redirected to /login (browser
// flow) so users don't see raw 401 JSON.
func requireSession(signer *auth.Signer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(sessionCookieName)
			if err != nil {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}
			claims, err := signer.Parse(cookie.Value)
			if err != nil {
				clearSession(w)
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}
			ctx := context.WithValue(r.Context(), ctxKeyClaims, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func claimsFrom(ctx context.Context) *auth.Claims {
	c, _ := ctx.Value(ctxKeyClaims).(*auth.Claims)
	return c
}
