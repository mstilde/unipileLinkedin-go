package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

// fakeStore satisfies AccountOwnershipStore for tests.
type fakeStore struct {
	owned map[string]map[int64]bool // accountID -> userID -> owned
	err   error                     // if set, returned from UserOwnsAccount
}

func (f *fakeStore) UserOwnsAccount(_ context.Context, userID int64, accountID string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	users, ok := f.owned[accountID]
	if !ok {
		return false, nil
	}
	return users[userID], nil
}

// okHandler is a downstream handler that 200s and writes "ok".
var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
})

func TestAuthenticate_MissingHeader_401(t *testing.T) {
	s := mustSigner(t)
	h := Authenticate(s)(okHandler)
	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("got status %d", rr.Code)
	}
}

func TestAuthenticate_WrongScheme_401(t *testing.T) {
	s := mustSigner(t)
	h := Authenticate(s)(okHandler)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Basic abcdef")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("got status %d", rr.Code)
	}
}

func TestAuthenticate_HappyPath_200_AndAttachesClaims(t *testing.T) {
	s := mustSigner(t)
	tok, _ := s.Sign(99, RoleWorker)

	var seenUser int64
	var seenRole Role
	probe := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := ClaimsFromContext(r.Context())
		if !ok {
			t.Error("claims missing in handler context")
			return
		}
		seenUser = c.UserID
		seenRole = c.Role
		w.WriteHeader(200)
	})

	h := Authenticate(s)(probe)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Errorf("got status %d", rr.Code)
	}
	if seenUser != 99 || seenRole != RoleWorker {
		t.Errorf("got user=%d role=%v", seenUser, seenRole)
	}
}

func TestAuthenticate_ExpiredToken_401(t *testing.T) {
	s := mustSigner(t)
	s.now = func() time.Time { return time.Now().Add(-2 * time.Hour) }
	tok, _ := s.Sign(1, RoleWorker)
	s.now = time.Now

	h := Authenticate(s)(okHandler)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("got status %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "expired") {
		t.Errorf("expected expired error, got %q", rr.Body.String())
	}
}

func TestRequireAdmin_Forbids_NonAdmin(t *testing.T) {
	s := mustSigner(t)
	tok, _ := s.Sign(1, RoleWorker)

	h := Authenticate(s)(RequireAdmin()(okHandler))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("got status %d", rr.Code)
	}
}

func TestRequireAdmin_AllowsAdmin(t *testing.T) {
	s := mustSigner(t)
	tok, _ := s.Sign(1, RoleAdmin)

	h := Authenticate(s)(RequireAdmin()(okHandler))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Errorf("got status %d", rr.Code)
	}
}

func TestRequireOwnedAccount_Allows_Owner(t *testing.T) {
	s := mustSigner(t)
	tok, _ := s.Sign(42, RoleWorker)
	store := &fakeStore{
		owned: map[string]map[int64]bool{"acct-1": {42: true}},
	}

	r := chi.NewRouter()
	r.With(Authenticate(s), RequireOwnedAccount(store)).
		Get("/accounts/{accountID}", okHandler.ServeHTTP)

	req := httptest.NewRequest("GET", "/accounts/acct-1", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Errorf("got status %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestRequireOwnedAccount_Forbids_NonOwner(t *testing.T) {
	s := mustSigner(t)
	tok, _ := s.Sign(42, RoleWorker)
	store := &fakeStore{
		owned: map[string]map[int64]bool{"acct-1": {99: true}}, // user 99 owns it, not 42
	}

	r := chi.NewRouter()
	r.With(Authenticate(s), RequireOwnedAccount(store)).
		Get("/accounts/{accountID}", okHandler.ServeHTTP)

	req := httptest.NewRequest("GET", "/accounts/acct-1", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("got status %d", rr.Code)
	}
}

func TestRequireOwnedAccount_AdminBypassesCheck(t *testing.T) {
	s := mustSigner(t)
	tok, _ := s.Sign(42, RoleAdmin)
	store := &fakeStore{owned: map[string]map[int64]bool{}} // empty: admin should still pass

	r := chi.NewRouter()
	r.With(Authenticate(s), RequireOwnedAccount(store)).
		Get("/accounts/{accountID}", okHandler.ServeHTTP)

	req := httptest.NewRequest("GET", "/accounts/acct-1", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Errorf("admin should bypass, got %d", rr.Code)
	}
}

func TestRequireOwnedAccount_StoreError_500(t *testing.T) {
	s := mustSigner(t)
	tok, _ := s.Sign(42, RoleWorker)
	store := &fakeStore{err: errors.New("db down")}

	r := chi.NewRouter()
	r.With(Authenticate(s), RequireOwnedAccount(store)).
		Get("/accounts/{accountID}", okHandler.ServeHTTP)

	req := httptest.NewRequest("GET", "/accounts/acct-1", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("got status %d", rr.Code)
	}
}
