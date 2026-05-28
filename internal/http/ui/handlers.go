package ui

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/mstilde/unipile-linkedin-go/internal/auth"
	"github.com/mstilde/unipile-linkedin-go/internal/db/gen"
	"github.com/mstilde/unipile-linkedin-go/internal/http/views"
)

type Handlers struct {
	Pool   *pgxpool.Pool
	Q      *gen.Queries
	Signer *auth.Signer
}

// Login GET: render the login form.
func (h *Handlers) LoginGet(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = views.Login("").Render(r.Context(), w)
}

// LoginPost validates credentials, sets the session cookie, redirects to /dashboard.
func (h *Handlers) LoginPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		_ = views.Login("invalid form").Render(r.Context(), w)
		return
	}
	username := r.PostFormValue("username")
	password := r.PostFormValue("password")
	if username == "" || password == "" {
		_ = views.Login("username and password required").Render(r.Context(), w)
		return
	}
	user, err := h.Q.GetUserByUsername(r.Context(), username)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		_ = views.Login("invalid credentials").Render(r.Context(), w)
		return
	}
	token, err := h.Signer.Sign(user.ID, auth.Role(user.Role))
	if err != nil {
		_ = views.Login("session creation failed").Render(r.Context(), w)
		return
	}
	setSession(w, token, 7*24*time.Hour)
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	clearSession(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// Dashboard lists the accounts owned by the user. An ?account=... query param
// selects which account's campaigns to show.
func (h *Handlers) Dashboard(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	user, err := h.Q.GetUserByID(r.Context(), claims.UserID)
	if err != nil {
		http.Error(w, "user not found", http.StatusInternalServerError)
		return
	}

	var accounts []gen.Account
	if claims.Role == auth.RoleAdmin {
		accounts, _ = h.Q.ListAccounts(r.Context())
	} else {
		accounts, _ = h.Q.ListAccountsByOwner(r.Context(), &claims.UserID)
	}

	display := user.Username
	if user.DisplayName != nil {
		display = *user.DisplayName
	}

	selected := r.URL.Query().Get("account")
	var campaigns []gen.Campaign
	if selected != "" {
		campaigns, _ = h.Q.ListCampaignsByAccount(r.Context(), selected)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = views.Dashboard(display, accounts, campaigns, selected).Render(r.Context(), w)
}

// CampaignsPage is the per-account campaigns page, reachable at
// /accounts/{accountID}/campaigns. Reuses the Dashboard view scoped to that account.
func (h *Handlers) CampaignsPage(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "accountID")
	claims := claimsFrom(r.Context())
	user, _ := h.Q.GetUserByID(r.Context(), claims.UserID)

	var accounts []gen.Account
	if claims.Role == auth.RoleAdmin {
		accounts, _ = h.Q.ListAccounts(r.Context())
	} else {
		accounts, _ = h.Q.ListAccountsByOwner(r.Context(), &claims.UserID)
	}

	campaigns, _ := h.Q.ListCampaignsByAccount(r.Context(), accountID)
	display := user.Username
	if user.DisplayName != nil {
		display = *user.DisplayName
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = views.Dashboard(display, accounts, campaigns, accountID).Render(r.Context(), w)
}

// CreateCampaign handles the HTMX form post; returns just the campaign table partial.
func (h *Handlers) CreateCampaign(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "accountID")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	name := r.PostFormValue("name")
	if name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}
	_, err := h.Q.CreateCampaign(r.Context(), gen.CreateCampaignParams{
		AccountID:         accountID,
		Name:              name,
		DailyInviteLimit:  40,
		TZ:                "America/Argentina/Buenos_Aires",
		AutoReplyEnabled:  false,
		Column6:           nil,
		WorkingHoursStart: 9,
		WorkingHoursEnd:   19,
		SkipWeekends:      false,
		IsFollowup:        false,
	})
	if err != nil {
		http.Error(w, "create failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	campaigns, _ := h.Q.ListCampaignsByAccount(r.Context(), accountID)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = views.CampaignList(accountID, campaigns).Render(r.Context(), w)
}

func (h *Handlers) setCampaignStatus(w http.ResponseWriter, r *http.Request, status string) {
	accountID := chi.URLParam(r, "accountID")
	idStr := chi.URLParam(r, "campaignID")
	var id pgUUID
	if err := id.scan(idStr); err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	c, err := h.Q.SetCampaignStatus(r.Context(), gen.SetCampaignStatusParams{ID: id.v, Status: status})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "update failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = views.CampaignRow(accountID, c).Render(r.Context(), w)
}

func (h *Handlers) StartCampaign(w http.ResponseWriter, r *http.Request) { h.setCampaignStatus(w, r, "active") }
func (h *Handlers) PauseCampaign(w http.ResponseWriter, r *http.Request) { h.setCampaignStatus(w, r, "paused") }

// SequencePage renders the sequence builder for one campaign.
func (h *Handlers) SequencePage(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "accountID")
	idStr := chi.URLParam(r, "campaignID")
	var id pgUUID
	if err := id.scan(idStr); err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	c, err := h.Q.GetCampaignForAccount(r.Context(), gen.GetCampaignForAccountParams{
		ID:        id.v,
		AccountID: accountID,
	})
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	steps, _ := h.Q.ListStepsByCampaign(r.Context(), id.v)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = views.SequenceBuilder(accountID, c, steps).Render(r.Context(), w)
}
