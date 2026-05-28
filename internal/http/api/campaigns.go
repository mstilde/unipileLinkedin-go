package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/mstilde/unipile-linkedin-go/internal/auth"
	"github.com/mstilde/unipile-linkedin-go/internal/db/gen"
)

type CampaignsHandler struct {
	Q *gen.Queries
}

type campaignDTO struct {
	ID                       string   `json:"id"`
	AccountID                string   `json:"account_id"`
	Name                     string   `json:"name"`
	Status                   string   `json:"status"`
	DailyInviteLimit         int32    `json:"daily_invite_limit"`
	TZ                       string   `json:"tz"`
	AutoReplyEnabled         bool     `json:"auto_reply_enabled"`
	WorkingHoursStart        int32    `json:"working_hours_start"`
	WorkingHoursEnd          int32    `json:"working_hours_end"`
	LunchBreakStart          *int32   `json:"lunch_break_start,omitempty"`
	LunchBreakEnd            *int32   `json:"lunch_break_end,omitempty"`
	SkipWeekends             bool     `json:"skip_weekends"`
	SkipHolidays             []string `json:"skip_holidays"`
	RampUpEnabled            bool     `json:"ramp_up_enabled"`
	AutoWithdrawAfterDays    *int32   `json:"auto_withdraw_after_days,omitempty"`
	AutoResumeOnHumanReply   bool     `json:"auto_resume_on_human_reply"`
	IsFollowup               bool     `json:"is_followup"`
	StrictTemplateValidation bool     `json:"strict_template_validation"`
	AutoPrewarmVisit         bool     `json:"auto_prewarm_visit"`
	AutoPrewarmDelayMinutes  int32    `json:"auto_prewarm_delay_minutes"`
	SimulateHumanTyping      bool     `json:"simulate_human_typing"`
	CreatedAt                string   `json:"created_at"`
	UpdatedAt                string   `json:"updated_at"`
}

func toCampaignDTO(c gen.Campaign) campaignDTO {
	return campaignDTO{
		ID:                       uuidString(c.ID),
		AccountID:                c.AccountID,
		Name:                     c.Name,
		Status:                   c.Status,
		DailyInviteLimit:         c.DailyInviteLimit,
		TZ:                       c.TZ,
		AutoReplyEnabled:         c.AutoReplyEnabled,
		WorkingHoursStart:        c.WorkingHoursStart,
		WorkingHoursEnd:          c.WorkingHoursEnd,
		LunchBreakStart:          c.LunchBreakStart,
		LunchBreakEnd:            c.LunchBreakEnd,
		SkipWeekends:             c.SkipWeekends,
		SkipHolidays:             c.SkipHolidays,
		RampUpEnabled:            c.RampUpEnabled,
		AutoWithdrawAfterDays:    c.AutoWithdrawAfterDays,
		AutoResumeOnHumanReply:   c.AutoResumeOnHumanReply,
		IsFollowup:               c.IsFollowup,
		StrictTemplateValidation: c.StrictTemplateValidation,
		AutoPrewarmVisit:         c.AutoPrewarmVisit,
		AutoPrewarmDelayMinutes:  c.AutoPrewarmDelayMinutes,
		SimulateHumanTyping:      c.SimulateHumanTyping,
		CreatedAt:                tsString(c.CreatedAt),
		UpdatedAt:                tsString(c.UpdatedAt),
	}
}

func tsString(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}
	return t.Time.UTC().Format("2006-01-02T15:04:05Z")
}

// List returns the campaigns for the account in the URL path.
func (h *CampaignsHandler) List(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "accountID")
	rows, err := h.Q.ListCampaignsByAccount(r.Context(), accountID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	out := make([]campaignDTO, 0, len(rows))
	for _, c := range rows {
		out = append(out, toCampaignDTO(c))
	}
	writeJSON(w, http.StatusOK, out)
}

type createCampaignReq struct {
	Name              string `json:"name"`
	DailyInviteLimit  int32  `json:"daily_invite_limit"`
	TZ                string `json:"tz"`
	AutoReplyEnabled  bool   `json:"auto_reply_enabled"`
	WorkingHoursStart int32  `json:"working_hours_start"`
	WorkingHoursEnd   int32  `json:"working_hours_end"`
	SkipWeekends      bool   `json:"skip_weekends"`
	IsFollowup        bool   `json:"is_followup"`
}

func (h *CampaignsHandler) Create(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "accountID")
	var req createCampaignReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	if req.DailyInviteLimit == 0 {
		req.DailyInviteLimit = 40
	}
	if req.TZ == "" {
		req.TZ = "America/Argentina/Buenos_Aires"
	}
	if req.WorkingHoursStart == 0 {
		req.WorkingHoursStart = 9
	}
	if req.WorkingHoursEnd == 0 {
		req.WorkingHoursEnd = 19
	}
	c, err := h.Q.CreateCampaign(r.Context(), gen.CreateCampaignParams{
		AccountID:         accountID,
		Name:              req.Name,
		DailyInviteLimit:  req.DailyInviteLimit,
		TZ:                req.TZ,
		AutoReplyEnabled:  req.AutoReplyEnabled,
		Column6:           nil,
		WorkingHoursStart: req.WorkingHoursStart,
		WorkingHoursEnd:   req.WorkingHoursEnd,
		SkipWeekends:      req.SkipWeekends,
		IsFollowup:        req.IsFollowup,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "create failed")
		return
	}
	writeJSON(w, http.StatusCreated, toCampaignDTO(c))
}

func (h *CampaignsHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := h.loadCampaignByPath(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, toCampaignDTO(id))
}

type updateCampaignReq struct {
	Name                     *string  `json:"name,omitempty"`
	DailyInviteLimit         *int32   `json:"daily_invite_limit,omitempty"`
	TZ                       *string  `json:"tz,omitempty"`
	AutoReplyEnabled         *bool    `json:"auto_reply_enabled,omitempty"`
	WorkingHoursStart        *int32   `json:"working_hours_start,omitempty"`
	WorkingHoursEnd          *int32   `json:"working_hours_end,omitempty"`
	LunchBreakStart          *int32   `json:"lunch_break_start,omitempty"`
	LunchBreakEnd            *int32   `json:"lunch_break_end,omitempty"`
	SkipWeekends             *bool    `json:"skip_weekends,omitempty"`
	SkipHolidays             []string `json:"skip_holidays,omitempty"`
	RampUpEnabled            *bool    `json:"ramp_up_enabled,omitempty"`
	AutoWithdrawAfterDays    *int32   `json:"auto_withdraw_after_days,omitempty"`
	AutoResumeOnHumanReply   *bool    `json:"auto_resume_on_human_reply,omitempty"`
	IsFollowup               *bool    `json:"is_followup,omitempty"`
	StrictTemplateValidation *bool    `json:"strict_template_validation,omitempty"`
	AutoPrewarmVisit         *bool    `json:"auto_prewarm_visit,omitempty"`
	AutoPrewarmDelayMinutes  *int32   `json:"auto_prewarm_delay_minutes,omitempty"`
	SimulateHumanTyping      *bool    `json:"simulate_human_typing,omitempty"`
}

// Update merges the patch with the existing record (read-modify-write).
func (h *CampaignsHandler) Update(w http.ResponseWriter, r *http.Request) {
	existing, ok := h.loadCampaignByPath(w, r)
	if !ok {
		return
	}
	var req updateCampaignReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	params := gen.UpdateCampaignParams{
		ID:                       existing.ID,
		Name:                     existing.Name,
		DailyInviteLimit:         existing.DailyInviteLimit,
		TZ:                       existing.TZ,
		AutoReplyEnabled:         existing.AutoReplyEnabled,
		AiMonthlyCapUsd:          existing.AiMonthlyCapUsd,
		WorkingHoursStart:        existing.WorkingHoursStart,
		WorkingHoursEnd:          existing.WorkingHoursEnd,
		LunchBreakStart:          existing.LunchBreakStart,
		LunchBreakEnd:            existing.LunchBreakEnd,
		SkipWeekends:             existing.SkipWeekends,
		SkipHolidays:             existing.SkipHolidays,
		RampUpEnabled:            existing.RampUpEnabled,
		AutoWithdrawAfterDays:    existing.AutoWithdrawAfterDays,
		AutoResumeOnHumanReply:   existing.AutoResumeOnHumanReply,
		IsFollowup:               existing.IsFollowup,
		StrictTemplateValidation: existing.StrictTemplateValidation,
		AutoPrewarmVisit:         existing.AutoPrewarmVisit,
		AutoPrewarmDelayMinutes:  existing.AutoPrewarmDelayMinutes,
		SimulateHumanTyping:      existing.SimulateHumanTyping,
	}
	if req.Name != nil {
		params.Name = *req.Name
	}
	if req.DailyInviteLimit != nil {
		params.DailyInviteLimit = *req.DailyInviteLimit
	}
	if req.TZ != nil {
		params.TZ = *req.TZ
	}
	if req.AutoReplyEnabled != nil {
		params.AutoReplyEnabled = *req.AutoReplyEnabled
	}
	if req.WorkingHoursStart != nil {
		params.WorkingHoursStart = *req.WorkingHoursStart
	}
	if req.WorkingHoursEnd != nil {
		params.WorkingHoursEnd = *req.WorkingHoursEnd
	}
	if req.LunchBreakStart != nil {
		params.LunchBreakStart = req.LunchBreakStart
	}
	if req.LunchBreakEnd != nil {
		params.LunchBreakEnd = req.LunchBreakEnd
	}
	if req.SkipWeekends != nil {
		params.SkipWeekends = *req.SkipWeekends
	}
	if req.SkipHolidays != nil {
		params.SkipHolidays = req.SkipHolidays
	}
	if req.RampUpEnabled != nil {
		params.RampUpEnabled = *req.RampUpEnabled
	}
	if req.AutoWithdrawAfterDays != nil {
		params.AutoWithdrawAfterDays = req.AutoWithdrawAfterDays
	}
	if req.AutoResumeOnHumanReply != nil {
		params.AutoResumeOnHumanReply = *req.AutoResumeOnHumanReply
	}
	if req.IsFollowup != nil {
		params.IsFollowup = *req.IsFollowup
	}
	if req.StrictTemplateValidation != nil {
		params.StrictTemplateValidation = *req.StrictTemplateValidation
	}
	if req.AutoPrewarmVisit != nil {
		params.AutoPrewarmVisit = *req.AutoPrewarmVisit
	}
	if req.AutoPrewarmDelayMinutes != nil {
		params.AutoPrewarmDelayMinutes = *req.AutoPrewarmDelayMinutes
	}
	if req.SimulateHumanTyping != nil {
		params.SimulateHumanTyping = *req.SimulateHumanTyping
	}
	c, err := h.Q.UpdateCampaign(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	writeJSON(w, http.StatusOK, toCampaignDTO(c))
}

func (h *CampaignsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	c, ok := h.loadCampaignByPath(w, r)
	if !ok {
		return
	}
	if err := h.Q.DeleteCampaign(r.Context(), c.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *CampaignsHandler) SetStatus(w http.ResponseWriter, r *http.Request, status string) {
	c, ok := h.loadCampaignByPath(w, r)
	if !ok {
		return
	}
	if !isValidCampaignStatus(status) {
		writeError(w, http.StatusBadRequest, "invalid status")
		return
	}
	updated, err := h.Q.SetCampaignStatus(r.Context(), gen.SetCampaignStatusParams{
		ID:     c.ID,
		Status: status,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "status update failed")
		return
	}
	writeJSON(w, http.StatusOK, toCampaignDTO(updated))
}

// Start transitions campaign to "active".
func (h *CampaignsHandler) Start(w http.ResponseWriter, r *http.Request) {
	h.SetStatus(w, r, "active")
}

// Pause transitions to "paused".
func (h *CampaignsHandler) Pause(w http.ResponseWriter, r *http.Request) {
	h.SetStatus(w, r, "paused")
}

// Resume sets back to "active".
func (h *CampaignsHandler) Resume(w http.ResponseWriter, r *http.Request) {
	h.SetStatus(w, r, "active")
}

type duplicateReq struct {
	Name string `json:"name"`
}

func (h *CampaignsHandler) Duplicate(w http.ResponseWriter, r *http.Request) {
	src, ok := h.loadCampaignByPath(w, r)
	if !ok {
		return
	}
	var req duplicateReq
	_ = decodeJSON(r, &req)
	name := req.Name
	if name == "" {
		name = src.Name + " (copy)"
	}
	dup, err := h.Q.DuplicateCampaign(r.Context(), gen.DuplicateCampaignParams{
		ID:   src.ID,
		Name: name,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "duplicate failed")
		return
	}
	writeJSON(w, http.StatusCreated, toCampaignDTO(dup))
}

// Funnel returns counts grouped by prospect status for the campaign.
func (h *CampaignsHandler) Funnel(w http.ResponseWriter, r *http.Request) {
	c, ok := h.loadCampaignByPath(w, r)
	if !ok {
		return
	}
	rows, err := h.Q.ProspectFunnelByCampaign(r.Context(), c.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	out := make(map[string]int64, len(rows))
	for _, r := range rows {
		out[r.Status] = r.Count
	}
	writeJSON(w, http.StatusOK, out)
}

// loadCampaignByPath reads /campaigns/{campaignID} and enforces account
// scope via the URL accountID param. Writes the error response on failure.
func (h *CampaignsHandler) loadCampaignByPath(w http.ResponseWriter, r *http.Request) (gen.Campaign, bool) {
	accountID := chi.URLParam(r, "accountID")
	id, err := parseUUID(chi.URLParam(r, "campaignID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad campaign id")
		return gen.Campaign{}, false
	}
	c, err := h.Q.GetCampaignForAccount(r.Context(), gen.GetCampaignForAccountParams{
		ID:        id,
		AccountID: accountID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "campaign not found")
			return gen.Campaign{}, false
		}
		writeError(w, http.StatusInternalServerError, "db error")
		return gen.Campaign{}, false
	}
	// Sanity: enforce admin-bypass already happens at middleware; nothing else.
	_ = auth.RoleAdmin
	return c, true
}

func isValidCampaignStatus(s string) bool {
	switch s {
	case "draft", "active", "paused", "completed", "archived":
		return true
	}
	return false
}
