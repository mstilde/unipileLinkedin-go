package ui

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/mstilde/unipile-linkedin-go/internal/auth"
	"github.com/mstilde/unipile-linkedin-go/internal/db/gen"
	"github.com/mstilde/unipile-linkedin-go/internal/http/views"
)

// MetricsPage renders the per-campaign metrics view.
func (h *Handlers) MetricsPage(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "accountID")
	var id pgUUID
	if err := id.scan(chi.URLParam(r, "campaignID")); err != nil {
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
	funnel, _ := h.Q.ProspectFunnelByCampaign(r.Context(), id.v)
	stages, _ := h.Q.ProspectStageDistribution(r.Context(), id.v)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = views.Metrics(accountID, c, funnel, stages).Render(r.Context(), w)
}

// InboxPage renders prospects for an account ordered by last reply.
func (h *Handlers) InboxPage(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "accountID")
	rows, _ := h.Q.ListProspectsByAccount(r.Context(), gen.ListProspectsByAccountParams{
		AccountID: accountID,
		Limit:     200,
	})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = views.Inbox(accountID, rows).Render(r.Context(), w)
}

// OnboardingPage renders the per-account onboarding form (system prompt + questionnaire).
func (h *Handlers) OnboardingPage(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "accountID")
	persona, _ := h.Q.GetAIPersonaByAccount(r.Context(), accountID)
	profile, _ := h.Q.GetClientProfileByAccount(r.Context(), accountID)
	var p *gen.AiPersona
	var cp *gen.ClientProfile
	if persona.AccountID == accountID {
		p = &persona
	}
	if profile.AccountID == accountID {
		cp = &profile
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = views.Onboarding(accountID, p, cp).Render(r.Context(), w)
}

// SaveOnboarding upserts the AI persona and client profile for the account.
func (h *Handlers) SaveOnboarding(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "accountID")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	systemPrompt := r.PostFormValue("system_prompt")
	questionnaire := r.PostFormValue("questionnaire")

	if _, err := h.Q.UpsertAIPersona(r.Context(), gen.UpsertAIPersonaParams{
		AccountID:    accountID,
		SystemPrompt: systemPrompt,
		Column3:      nil,
	}); err != nil {
		http.Error(w, "persona upsert failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var qbytes []byte
	if questionnaire != "" {
		// Validate it's valid JSON; reject otherwise (avoids garbage in the DB).
		if !json.Valid([]byte(questionnaire)) {
			http.Error(w, "questionnaire must be valid JSON", http.StatusBadRequest)
			return
		}
		qbytes = []byte(questionnaire)
	}
	if _, err := h.Q.UpsertClientProfile(r.Context(), gen.UpsertClientProfileParams{
		AccountID: accountID,
		Column2:   qbytes,
	}); err != nil {
		http.Error(w, "profile upsert failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AdminPage renders the admin landing (users + accounts).
func (h *Handlers) AdminPage(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	if claims.Role != auth.RoleAdmin {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	users, _ := h.Q.ListUsers(r.Context())
	accounts, _ := h.Q.ListAccounts(r.Context())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = views.Admin(users, accounts).Render(r.Context(), w)
}

// CreateUser (admin) handles the HTMX form to provision a new user.
func (h *Handlers) CreateUser(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	if claims.Role != auth.RoleAdmin {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	username := r.PostFormValue("username")
	password := r.PostFormValue("password")
	if username == "" || len(password) < 8 {
		http.Error(w, "username and password (8+) required", http.StatusBadRequest)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 10)
	if err != nil {
		http.Error(w, "hash failed", http.StatusInternalServerError)
		return
	}
	role := r.PostFormValue("role")
	if role != "admin" {
		role = "worker"
	}
	display := r.PostFormValue("display_name")
	var displayPtr *string
	if display != "" {
		displayPtr = &display
	}
	if _, err := h.Q.CreateUser(r.Context(), gen.CreateUserParams{
		Username:     username,
		PasswordHash: string(hash),
		DisplayName:  displayPtr,
		Role:         role,
	}); err != nil {
		http.Error(w, "create user failed: "+err.Error(), http.StatusBadRequest)
		return
	}
	// Re-render the admin page so the table refreshes.
	h.AdminPage(w, r)
}
