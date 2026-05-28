package api

import (
	"encoding/csv"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mstilde/unipile-linkedin-go/internal/db/gen"
)

type ProspectsHandler struct {
	Pool *pgxpool.Pool
	Q    *gen.Queries
}

type prospectDTO struct {
	ID                string   `json:"id"`
	CampaignID        string   `json:"campaign_id"`
	AccountID         string   `json:"account_id"`
	ProfileURL        string   `json:"profile_url"`
	FullName          *string  `json:"full_name,omitempty"`
	FirstName         *string  `json:"first_name,omitempty"`
	Headline          *string  `json:"headline,omitempty"`
	Company           *string  `json:"company,omitempty"`
	Status            string   `json:"status"`
	ConversationStage string   `json:"conversation_stage"`
	Tags              []string `json:"tags"`
	InvitedAt         string   `json:"invited_at,omitempty"`
	ConnectedAt       string   `json:"connected_at,omitempty"`
	RepliedAt         string   `json:"replied_at,omitempty"`
	CreatedAt         string   `json:"created_at"`
}

func toProspectDTO(p gen.Prospect) prospectDTO {
	return prospectDTO{
		ID:                uuidString(p.ID),
		CampaignID:        uuidString(p.CampaignID),
		AccountID:         p.AccountID,
		ProfileURL:        p.ProfileUrl,
		FullName:          p.FullName,
		FirstName:         p.FirstName,
		Headline:          p.Headline,
		Company:           p.Company,
		Status:            p.Status,
		ConversationStage: p.ConversationStage,
		Tags:              p.Tags,
		InvitedAt:         tsString(p.InvitedAt),
		ConnectedAt:       tsString(p.ConnectedAt),
		RepliedAt:         tsString(p.RepliedAt),
		CreatedAt:         tsString(p.CreatedAt),
	}
}

func (h *ProspectsHandler) List(w http.ResponseWriter, r *http.Request) {
	campaignID, err := parseUUID(chi.URLParam(r, "campaignID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad campaign id")
		return
	}
	limit := int32(100)
	offset := int32(0)
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			limit = int32(n)
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = int32(n)
		}
	}
	rows, err := h.Q.ListProspectsByCampaign(r.Context(), gen.ListProspectsByCampaignParams{
		CampaignID: campaignID,
		Limit:      limit,
		Offset:     offset,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	total, _ := h.Q.CountProspectsByCampaign(r.Context(), campaignID)

	out := make([]prospectDTO, 0, len(rows))
	for _, p := range rows {
		out = append(out, toProspectDTO(p))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"prospects": out,
		"total":     total,
		"limit":     limit,
		"offset":    offset,
	})
}

func (h *ProspectsHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "prospectID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad prospect id")
		return
	}
	p, err := h.Q.GetProspect(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "prospect not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, toProspectDTO(p))
}

func (h *ProspectsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "prospectID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad prospect id")
		return
	}
	if err := h.Q.DeleteProspect(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Upload accepts CSV with columns: profile_url, full_name (optional), headline (optional), company (optional), first_name (optional).
// Each row becomes a prospect via the CreateProspect upsert query.
func (h *ProspectsHandler) Upload(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "accountID")
	campaignID, err := parseUUID(chi.URLParam(r, "campaignID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad campaign id")
		return
	}
	if err := r.ParseMultipartForm(8 << 20); err != nil { // 8 MB
		writeError(w, http.StatusBadRequest, "multipart parse failed")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file field missing")
		return
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1

	header, err := reader.Read()
	if err != nil {
		writeError(w, http.StatusBadRequest, "csv header missing")
		return
	}
	cols := map[string]int{}
	for i, h := range header {
		cols[strings.ToLower(strings.TrimSpace(h))] = i
	}
	idxURL, ok := cols["profile_url"]
	if !ok {
		writeError(w, http.StatusBadRequest, "csv must include 'profile_url' column")
		return
	}

	type rowResult struct {
		Row   int    `json:"row"`
		Error string `json:"error,omitempty"`
		ID    string `json:"id,omitempty"`
	}
	results := []rowResult{}
	rowNum := 1
	tx, err := h.Pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "tx begin failed")
		return
	}
	defer tx.Rollback(r.Context())
	q := h.Q.WithTx(tx)

	for {
		rec, err := reader.Read()
		if err == io.EOF {
			break
		}
		rowNum++
		if err != nil {
			results = append(results, rowResult{Row: rowNum, Error: err.Error()})
			continue
		}
		url := strings.TrimSpace(rec[idxURL])
		if url == "" {
			results = append(results, rowResult{Row: rowNum, Error: "empty profile_url"})
			continue
		}
		getCol := func(name string) *string {
			if i, ok := cols[name]; ok && i < len(rec) {
				v := strings.TrimSpace(rec[i])
				if v == "" {
					return nil
				}
				return &v
			}
			return nil
		}
		p, err := q.CreateProspect(r.Context(), gen.CreateProspectParams{
			CampaignID:       campaignID,
			AccountID:        accountID,
			ProfileUrl:       url,
			PublicIdentifier: nil,
			FullName:         getCol("full_name"),
			FirstName:        getCol("first_name"),
			Headline:         getCol("headline"),
			Company:          getCol("company"),
			Column9:          nil,
			Column10:         nil,
			Tags:             []string{},
			Column12:         nil,
		})
		if err != nil {
			results = append(results, rowResult{Row: rowNum, Error: err.Error()})
			continue
		}
		results = append(results, rowResult{Row: rowNum, ID: uuidString(p.ID)})
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "commit failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"rows":    rowNum - 1,
		"results": results,
	})
}
