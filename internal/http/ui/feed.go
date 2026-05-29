package ui

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/mstilde/unipile-linkedin-go/internal/db/gen"
	"github.com/mstilde/unipile-linkedin-go/internal/http/views"
)

// FeedPage renders the post-feed scanner report for an account (front 4).
func (h *Handlers) FeedPage(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "accountID")
	posts, _ := h.Q.ListFeedPostsByAccount(r.Context(), gen.ListFeedPostsByAccountParams{
		AccountID: accountID,
		Limit:     200,
	})
	searches, _ := h.Q.ListFeedSearchesByAccount(r.Context(), accountID)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = views.Feed(accountID, posts, searches).Render(r.Context(), w)
}

// SetFeedStatus handles the dismiss/status HTMX action and re-renders the table.
func (h *Handlers) SetFeedStatus(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "accountID")
	var id pgUUID
	if err := id.scan(chi.URLParam(r, "postID")); err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	status := r.PostFormValue("status")
	switch status {
	case "relevant", "irrelevant", "dismissed", "new":
		// allowed
	default:
		http.Error(w, "invalid status", http.StatusBadRequest)
		return
	}
	if err := h.Q.SetFeedPostStatus(r.Context(), gen.SetFeedPostStatusParams{
		ID: id.v, Status: status, AccountID: accountID,
	}); err != nil {
		http.Error(w, "update failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	h.renderFeedTable(w, r, accountID)
}

// ImportFeedAuthor turns a post's author into a prospect in the account's first
// active campaign (closing the loop to front 1). It then marks the post
// 'imported' and re-renders the table.
func (h *Handlers) ImportFeedAuthor(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "accountID")
	var id pgUUID
	if err := id.scan(chi.URLParam(r, "postID")); err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}

	post, err := h.Q.GetFeedPost(r.Context(), gen.GetFeedPostParams{ID: id.v, AccountID: accountID})
	if err != nil {
		http.Error(w, "post not found", http.StatusNotFound)
		return
	}
	if post.AuthorProfileUrl == nil || *post.AuthorProfileUrl == "" {
		http.Error(w, "post author has no profile URL to import", http.StatusBadRequest)
		return
	}

	campaignID, ok := h.pickImportCampaign(r, accountID)
	if !ok {
		http.Error(w, "no campaign to import into — create one first", http.StatusBadRequest)
		return
	}

	prospect, err := h.Q.CreateProspect(r.Context(), gen.CreateProspectParams{
		CampaignID: campaignID.v,
		AccountID:  accountID,
		ProfileUrl: *post.AuthorProfileUrl,
		FullName:   post.AuthorName,
		FirstName:  firstNameOf(post.AuthorName),
		Headline:   post.AuthorHeadline,
		Company:    post.AiCompany,
		Column9:    nil, // status -> default queued
		Column10:   nil, // conversation_stage -> default
		Tags:       []string{"from_feed"},
		Column12:   nil, // variables -> default empty json
	})
	if err != nil {
		http.Error(w, "create prospect failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := h.Q.SetFeedPostImported(r.Context(), gen.SetFeedPostImportedParams{
		ID:                 id.v,
		ImportedProspectID: prospect.ID,
		AccountID:          accountID,
	}); err != nil {
		http.Error(w, "mark imported failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	h.renderFeedTable(w, r, accountID)
}

func (h *Handlers) renderFeedTable(w http.ResponseWriter, r *http.Request, accountID string) {
	posts, _ := h.Q.ListFeedPostsByAccount(r.Context(), gen.ListFeedPostsByAccountParams{
		AccountID: accountID,
		Limit:     200,
	})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = views.FeedTable(accountID, posts).Render(r.Context(), w)
}

// pickImportCampaign returns the account's first 'active' campaign, falling back
// to the first campaign of any status.
func (h *Handlers) pickImportCampaign(r *http.Request, accountID string) (pgUUID, bool) {
	campaigns, err := h.Q.ListCampaignsByAccount(r.Context(), accountID)
	if err != nil || len(campaigns) == 0 {
		return pgUUID{}, false
	}
	for _, c := range campaigns {
		if c.Status == "active" {
			return pgUUID{v: c.ID}, true
		}
	}
	return pgUUID{v: campaigns[0].ID}, true
}

// firstNameOf returns the first whitespace-delimited token of a full name.
func firstNameOf(full *string) *string {
	if full == nil {
		return nil
	}
	fields := strings.Fields(*full)
	if len(fields) == 0 {
		return nil
	}
	return &fields[0]
}
