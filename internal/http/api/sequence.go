package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mstilde/unipile-linkedin-go/internal/db/gen"
)

type SequenceHandler struct {
	Pool *pgxpool.Pool
	Q    *gen.Queries
}

type stepDTO struct {
	ID            string          `json:"id"`
	CampaignID    string          `json:"campaign_id"`
	StepIndex     int32           `json:"step_index"`
	StepType      string          `json:"step_type"`
	DelayHours    int32           `json:"delay_hours"`
	Template      *string         `json:"template,omitempty"`
	AIPersonalize bool            `json:"ai_personalize"`
	NoteMaxChars  int32           `json:"note_max_chars"`
	StageLabel    *string         `json:"stage_label,omitempty"`
	ConfigJSON    json.RawMessage `json:"config_json,omitempty"`
}

func toStepDTO(s gen.SequenceStep) stepDTO {
	var cfg json.RawMessage
	if len(s.ConfigJson) > 0 {
		cfg = json.RawMessage(s.ConfigJson)
	} else {
		cfg = json.RawMessage(`{}`)
	}
	return stepDTO{
		ID:            uuidString(s.ID),
		CampaignID:    uuidString(s.CampaignID),
		StepIndex:     s.StepIndex,
		StepType:      s.StepType,
		DelayHours:    s.DelayHours,
		Template:      s.Template,
		AIPersonalize: s.AiPersonalize,
		NoteMaxChars:  s.NoteMaxChars,
		StageLabel:    s.StageLabel,
		ConfigJSON:    cfg,
	}
}

// List returns the sequence steps of a campaign, ordered by step_index.
func (h *SequenceHandler) List(w http.ResponseWriter, r *http.Request) {
	campaignID, err := parseUUID(chi.URLParam(r, "campaignID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad campaign id")
		return
	}
	steps, err := h.Q.ListStepsByCampaign(r.Context(), campaignID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	out := make([]stepDTO, 0, len(steps))
	for _, s := range steps {
		out = append(out, toStepDTO(s))
	}
	writeJSON(w, http.StatusOK, out)
}

type replaceStepReq struct {
	StepType      string          `json:"step_type"`
	DelayHours    int32           `json:"delay_hours"`
	Template      *string         `json:"template,omitempty"`
	AIPersonalize bool            `json:"ai_personalize"`
	NoteMaxChars  int32           `json:"note_max_chars"`
	StageLabel    *string         `json:"stage_label,omitempty"`
	ConfigJSON    json.RawMessage `json:"config_json,omitempty"`
}

type replaceReq struct {
	Steps []replaceStepReq `json:"steps"`
}

// Replace atomically wipes the existing steps and inserts the new ordered list.
// Uses a transaction so partial sequences never leak through.
func (h *SequenceHandler) Replace(w http.ResponseWriter, r *http.Request) {
	campaignID, err := parseUUID(chi.URLParam(r, "campaignID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad campaign id")
		return
	}
	var req replaceReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if len(req.Steps) == 0 {
		writeError(w, http.StatusBadRequest, "steps required")
		return
	}

	tx, err := h.Pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "tx begin failed")
		return
	}
	defer tx.Rollback(r.Context())

	q := h.Q.WithTx(tx)
	if err := q.DeleteAllStepsForCampaign(r.Context(), campaignID); err != nil {
		writeError(w, http.StatusInternalServerError, "wipe failed")
		return
	}

	created := make([]stepDTO, 0, len(req.Steps))
	for i, s := range req.Steps {
		if s.NoteMaxChars == 0 {
			s.NoteMaxChars = 200
		}
		if s.DelayHours < 0 {
			s.DelayHours = 0
		}
		var cfg []byte
		if len(s.ConfigJSON) > 0 {
			cfg = []byte(s.ConfigJSON)
		} else {
			cfg = []byte("{}")
		}
		step, err := q.CreateStep(r.Context(), gen.CreateStepParams{
			CampaignID:     campaignID,
			StepIndex:      int32(i),
			StepType:       s.StepType,
			DelayHours:     s.DelayHours,
			Template:       s.Template,
			AiPersonalize:  s.AIPersonalize,
			NoteMaxChars:   s.NoteMaxChars,
			StageLabel:     s.StageLabel,
			ConfigJson:     cfg,
		})
		if err != nil {
			writeError(w, http.StatusBadRequest, "create step "+s.StepType+" failed: "+err.Error())
			return
		}
		created = append(created, toStepDTO(step))
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "commit failed")
		return
	}
	writeJSON(w, http.StatusOK, created)
}

// ----------------------------------------------------------------------------
// Templates
// ----------------------------------------------------------------------------

type templateDTO struct {
	ID           string `json:"id"`
	CampaignID   string `json:"campaign_id"`
	TemplateKind string `json:"template_kind"`
	TemplateText string `json:"template_text"`
	AIAdapt      bool   `json:"ai_adapt"`
}

func toTemplateDTO(t gen.CampaignTemplate) templateDTO {
	return templateDTO{
		ID:           uuidString(t.ID),
		CampaignID:   uuidString(t.CampaignID),
		TemplateKind: t.TemplateKind,
		TemplateText: t.TemplateText,
		AIAdapt:      t.AiAdapt,
	}
}

func (h *SequenceHandler) ListTemplates(w http.ResponseWriter, r *http.Request) {
	campaignID, err := parseUUID(chi.URLParam(r, "campaignID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad campaign id")
		return
	}
	rows, err := h.Q.ListTemplatesByCampaign(r.Context(), campaignID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	out := make([]templateDTO, 0, len(rows))
	for _, t := range rows {
		out = append(out, toTemplateDTO(t))
	}
	writeJSON(w, http.StatusOK, out)
}

type upsertTemplateReq struct {
	TemplateKind string `json:"template_kind"`
	TemplateText string `json:"template_text"`
	AIAdapt      bool   `json:"ai_adapt"`
}

func (h *SequenceHandler) UpsertTemplate(w http.ResponseWriter, r *http.Request) {
	campaignID, err := parseUUID(chi.URLParam(r, "campaignID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad campaign id")
		return
	}
	var req upsertTemplateReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if !isValidTemplateKind(req.TemplateKind) {
		writeError(w, http.StatusBadRequest, "invalid template_kind")
		return
	}
	t, err := h.Q.UpsertTemplate(r.Context(), gen.UpsertTemplateParams{
		CampaignID:   campaignID,
		TemplateKind: req.TemplateKind,
		TemplateText: req.TemplateText,
		AiAdapt:      req.AIAdapt,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "upsert failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toTemplateDTO(t))
}

func (h *SequenceHandler) DeleteTemplate(w http.ResponseWriter, r *http.Request) {
	campaignID, err := parseUUID(chi.URLParam(r, "campaignID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad campaign id")
		return
	}
	kind := chi.URLParam(r, "kind")
	if !isValidTemplateKind(kind) {
		writeError(w, http.StatusBadRequest, "invalid template_kind")
		return
	}
	if err := h.Q.DeleteTemplate(r.Context(), gen.DeleteTemplateParams{
		CampaignID:   campaignID,
		TemplateKind: kind,
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func isValidTemplateKind(k string) bool {
	switch k {
	case "invite_note", "nota_premium", "msg1", "propuesta",
		"transicion_with_phone", "transicion_ask_phone", "post_wa_confirmation":
		return true
	}
	return false
}

// touch to keep pgtype import used (campaigns/sequence both share it)
var _ = pgtype.UUID{}
