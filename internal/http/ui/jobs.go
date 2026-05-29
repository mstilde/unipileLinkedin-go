package ui

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/mstilde/unipile-linkedin-go/internal/db/gen"
	"github.com/mstilde/unipile-linkedin-go/internal/http/views"
)

// JobsPage renders the ranked job-postings report for an account (front 2B).
func (h *Handlers) JobsPage(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "accountID")
	postings, _ := h.Q.ListJobPostingsByAccount(r.Context(), gen.ListJobPostingsByAccountParams{
		AccountID: accountID,
		Limit:     200,
	})
	searches, _ := h.Q.ListJobSearchesByAccount(r.Context(), accountID)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = views.Jobs(accountID, postings, searches).Render(r.Context(), w)
}

// SetJobStatus handles the HTMX action buttons (applied/saved/dismissed) and
// re-renders the postings table.
func (h *Handlers) SetJobStatus(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "accountID")
	var id pgUUID
	if err := id.scan(chi.URLParam(r, "postingID")); err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	status := r.PostFormValue("status")
	switch status {
	case "applied", "saved", "dismissed", "scored", "new":
		// allowed
	default:
		http.Error(w, "invalid status", http.StatusBadRequest)
		return
	}
	if err := h.Q.SetJobPostingStatus(r.Context(), gen.SetJobPostingStatusParams{
		ID:        id.v,
		Status:    status,
		AccountID: accountID,
	}); err != nil {
		http.Error(w, "update failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	postings, _ := h.Q.ListJobPostingsByAccount(r.Context(), gen.ListJobPostingsByAccountParams{
		AccountID: accountID,
		Limit:     200,
	})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = views.JobsTable(accountID, postings).Render(r.Context(), w)
}
