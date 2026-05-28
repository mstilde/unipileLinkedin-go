package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/mstilde/unipile-linkedin-go/internal/db/gen"
	"github.com/mstilde/unipile-linkedin-go/internal/domain/template"
	"github.com/mstilde/unipile-linkedin-go/internal/unipile"
)

// tickCampaign is one iteration of the campaign loop: release stale leases
// (rows that were 'processing' too long, e.g. crashed worker), then dispatch
// the pending steps that are due.
func (m *Manager) tickCampaign(ctx context.Context) {
	l := m.log.With("scheduler", "campaign")

	stale := pgtype.Interval{
		Microseconds: int64(m.cfg.StaleLeaseAge / time.Microsecond),
		Valid:        true,
	}
	if err := m.q.ReleaseStaleLeases(ctx, stale); err != nil {
		l.Warn("release stale leases failed", "err", err)
	}

	rows, err := m.q.ListPendingProspectSteps(ctx, m.cfg.BatchSize)
	if err != nil {
		l.Error("list pending steps failed", "err", err)
		return
	}
	if len(rows) == 0 {
		return
	}
	l.Debug("dispatching", "count", len(rows))

	for _, row := range rows {
		m.dispatchStep(ctx, l, row)
	}
}

// dispatchStep performs one prospect_step. The high-level flow is:
//
//  1. lease — guards against double-send across crashes
//  2. load context: prospect + sequence_step + campaign
//  3. render the step's template against the prospect's variables
//  4. branch on step_type and call the matching Unipile action
//  5. on success: mark sent, advance prospect (status/bookkeeping), schedule
//     the next sequence step
//  6. on failure: classify the error so transient/throttle cases become retries
//     and permanent ones short-circuit the prospect
func (m *Manager) dispatchStep(ctx context.Context, l *slog.Logger, row gen.ListPendingProspectStepsRow) {
	step, err := m.q.LeaseProspectStep(ctx, row.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return // someone else grabbed it, that's fine
		}
		l.Warn("lease failed", "id", row.ID, "err", err)
		return
	}
	l = l.With("step_id", step.ID, "step_type", step.StepType, "prospect_id", step.ProspectID)

	prospect, err := m.q.GetProspect(ctx, step.ProspectID)
	if err != nil {
		m.failStep(ctx, l, step.ID, fmt.Sprintf("load prospect: %v", err))
		return
	}

	// step.StepID may be NULL for ad-hoc steps; in that case we don't have a
	// sequence_step row and skip template rendering.
	var seqStep gen.SequenceStep
	if step.StepID.Valid {
		seqStep, err = m.q.GetStep(ctx, step.StepID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			m.failStep(ctx, l, step.ID, fmt.Sprintf("load sequence step: %v", err))
			return
		}
	}

	// Bypass: explicit kill switch on the manager (env DRY_RUN drives this for
	// the Unipile client too, but we want to short-circuit before any side
	// effect when the operator flips the global.
	if m.cfg.KillswitchGlobal {
		m.failStep(ctx, l, step.ID, "killswitch active")
		return
	}

	// Get a Unipile client. Provider re-reads env so weekly free-tier rotation
	// of the DSN/APIKey doesn't need a restart.
	cli, err := m.unipile.Get()
	if err != nil {
		// No creds yet — leave the row pending by releasing the lease so a
		// future tick can retry.
		l.Warn("no unipile client; releasing lease", "err", err)
		m.releaseStep(ctx, l, step.ID)
		return
	}

	// Resolve the Unipile account_id (the one the API expects) from the local
	// accounts row. prospect.AccountID is the local FK 'accounts.id', not the
	// vendor id.
	acct, err := m.q.GetAccountByID(ctx, prospect.AccountID)
	if err != nil {
		m.failStep(ctx, l, step.ID, fmt.Sprintf("load account: %v", err))
		return
	}
	unipileAcctID := ""
	if acct.AccountID != nil {
		unipileAcctID = *acct.AccountID
	}
	if unipileAcctID == "" {
		m.failStep(ctx, l, step.ID, "account row has no Unipile account_id set")
		return
	}

	switch normaliseStepType(step.StepType) {
	case "invite":
		m.dispatchInvite(ctx, l, cli, unipileAcctID, &step, &prospect, &seqStep)
	case "send_message":
		m.dispatchMessage(ctx, l, cli, unipileAcctID, &step, &prospect, &seqStep)
	case "wait":
		// 'wait' rows are pure scheduling markers — no side effect. Marking
		// 'sent' is just the cleanest way to advance the state machine.
		m.completeStep(ctx, l, &step, &prospect, &seqStep, "wait")
	case "end":
		m.markStepSent(ctx, l, step.ID, "end")
		if err := m.setProspectStatus(ctx, prospect.ID, "won"); err != nil {
			l.Warn("set prospect status to won failed", "err", err)
		}
	default:
		// visit_profile, follow, like_post, comment_post, withdraw_invite,
		// voice_note, inmail, condition, ab_test — not implemented yet. Mark
		// failed loudly so we notice in metrics.
		m.failStep(ctx, l, step.ID, fmt.Sprintf("step type %q not implemented yet", step.StepType))
	}
}

// dispatchInvite renders the invite note and sends a connection invitation.
func (m *Manager) dispatchInvite(ctx context.Context, l *slog.Logger, cli *unipile.Client, unipileAcctID string, step *gen.ProspectStep, prospect *gen.Prospect, seqStep *gen.SequenceStep) {
	tpl := stepTemplate(seqStep)

	rendered, err := renderTemplate(tpl, prospect)
	if err != nil {
		m.failStep(ctx, l, step.ID, fmt.Sprintf("render: %v", err))
		return
	}

	providerID := strPtr(prospect.LinkedinProviderID)
	if providerID == "" {
		m.failStep(ctx, l, step.ID, "prospect has no linkedin_provider_id; resolve first")
		return
	}

	noteLimit := 200
	if seqStep != nil && seqStep.NoteMaxChars > 0 {
		noteLimit = int(seqStep.NoteMaxChars)
	}

	resp, err := cli.SendInvitation(ctx, unipile.SendInvitationParams{
		AccountID:  unipileAcctID,
		ProviderID: providerID,
		Message:    rendered,
		NoteLimit:  noteLimit,
	})
	if err != nil {
		m.handleUnipileError(ctx, l, step.ID, "send_invitation", err)
		return
	}

	if err := m.q.SetProspectInvited(ctx, gen.SetProspectInvitedParams{
		ID:                 prospect.ID,
		LinkedinProviderID: nil,
		ChatID:             nil,
	}); err != nil {
		l.Warn("set prospect invited failed", "err", err)
	}

	note := rendered
	if resp != nil && resp.InvitationID != "" {
		note = fmt.Sprintf("invitation=%s: %s", resp.InvitationID, rendered)
	}
	m.markStepSent(ctx, l, step.ID, note)
	m.scheduleNextStep(ctx, l, step, prospect, seqStep)
}

// dispatchMessage renders a message and routes via existing chat or new chat.
func (m *Manager) dispatchMessage(ctx context.Context, l *slog.Logger, cli *unipile.Client, unipileAcctID string, step *gen.ProspectStep, prospect *gen.Prospect, seqStep *gen.SequenceStep) {
	tpl := stepTemplate(seqStep)

	rendered, err := renderTemplate(tpl, prospect)
	if err != nil {
		m.failStep(ctx, l, step.ID, fmt.Sprintf("render: %v", err))
		return
	}
	if strings.TrimSpace(rendered) == "" {
		m.failStep(ctx, l, step.ID, "rendered message is empty")
		return
	}

	chatID := strPtr(prospect.ChatID)
	if chatID != "" {
		_, err := cli.SendMessage(ctx, unipile.SendMessageParams{
			ChatID: chatID,
			Text:   rendered,
		})
		if err != nil {
			m.handleUnipileError(ctx, l, step.ID, "send_message", err)
			return
		}
	} else {
		providerID := strPtr(prospect.LinkedinProviderID)
		if providerID == "" {
			m.failStep(ctx, l, step.ID, "prospect has no chat_id and no linkedin_provider_id")
			return
		}
		resp, err := cli.StartNewChat(ctx, unipile.StartNewChatParams{
			AccountID:    unipileAcctID,
			AttendeesIDs: []string{providerID},
			Text:         rendered,
		})
		if err != nil {
			m.handleUnipileError(ctx, l, step.ID, "start_new_chat", err)
			return
		}
		if resp != nil && resp.ChatID != "" {
			newChatID := resp.ChatID
			if err := m.q.SetProspectChatID(ctx, gen.SetProspectChatIDParams{
				ID: prospect.ID, ChatID: &newChatID,
			}); err != nil {
				l.Warn("set prospect chat_id failed", "err", err)
			}
		}
	}

	m.completeStep(ctx, l, step, prospect, seqStep, rendered)
}

// completeStep marks the step sent and schedules the next one.
func (m *Manager) completeStep(ctx context.Context, l *slog.Logger, step *gen.ProspectStep, prospect *gen.Prospect, seqStep *gen.SequenceStep, note string) {
	m.markStepSent(ctx, l, step.ID, note)
	m.scheduleNextStep(ctx, l, step, prospect, seqStep)
}

// scheduleNextStep inserts the next prospect_step at NOW + delay_hours. If
// there is no next step, the prospect is left alone (campaign continues to
// process other prospects; reaching 'end' is the explicit terminator).
func (m *Manager) scheduleNextStep(ctx context.Context, l *slog.Logger, step *gen.ProspectStep, prospect *gen.Prospect, seqStep *gen.SequenceStep) {
	if seqStep == nil || !seqStep.ID.Valid {
		return // ad-hoc step, nothing to chain
	}
	next, err := m.q.GetNextSequenceStep(ctx, seqStep.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return
		}
		l.Warn("lookup next step failed", "err", err)
		return
	}

	scheduledAt := time.Now().Add(time.Duration(next.DelayHours) * time.Hour)
	_, err = m.q.CreateProspectStep(ctx, gen.CreateProspectStepParams{
		ProspectID:  prospect.ID,
		StepID:      next.ID,
		StepType:    next.StepType,
		ScheduledAt: pgtype.Timestamptz{Time: scheduledAt, Valid: true},
	})
	if err != nil {
		// uniq_prospect_steps_active means we already scheduled this step
		// (idempotency). Ignore.
		if strings.Contains(err.Error(), "uniq_prospect_steps_active") {
			return
		}
		l.Warn("schedule next step failed", "err", err)
	}
}

// handleUnipileError classifies the error and updates the step.
//   - Permanent  → mark failed (no retry)
//   - Weekly cap → mark failed with a 'weekly_cap' detail so the operator
//     reschedules via a manual sweep (next Monday)
//   - Throttled/Transient/RateLimit → release the lease; the scheduler will
//     pick it up again next tick (which honors retry_count via max_retries)
func (m *Manager) handleUnipileError(ctx context.Context, l *slog.Logger, stepID pgtype.UUID, action string, err error) {
	apiErr, ok := unipile.AsAPIError(err)
	if !ok {
		// Network or marshalling error — treat as transient.
		l.Warn("unipile call failed (non-api error); releasing lease", "action", action, "err", err)
		m.releaseStep(ctx, l, stepID)
		return
	}

	switch {
	case apiErr.IsPermanent():
		m.failStep(ctx, l, stepID, fmt.Sprintf("%s: permanent (%s): %s", action, apiErr.Type, apiErr.Title))
	case apiErr.IsWeeklyCap():
		m.failStep(ctx, l, stepID, fmt.Sprintf("%s: weekly_cap: %s", action, apiErr.Title))
	case apiErr.IsThrottled(), apiErr.IsLinkedInRateLimit(), apiErr.IsTransient():
		l.Info("unipile transient; releasing lease for retry", "action", action, "status", apiErr.Status, "type", apiErr.Type)
		m.releaseStep(ctx, l, stepID)
	default:
		m.failStep(ctx, l, stepID, fmt.Sprintf("%s: %s", action, apiErr.Error()))
	}
}

func (m *Manager) markStepSent(ctx context.Context, l *slog.Logger, id pgtype.UUID, note string) {
	if err := m.q.MarkProspectStepSent(ctx, gen.MarkProspectStepSentParams{
		ID:          id,
		MessageSent: &note,
	}); err != nil {
		l.Warn("mark sent failed", "err", err)
	}
}

func (m *Manager) failStep(ctx context.Context, l *slog.Logger, id pgtype.UUID, reason string) {
	if err := m.q.MarkProspectStepFailed(ctx, gen.MarkProspectStepFailedParams{
		ID:          id,
		ErrorDetail: &reason,
	}); err != nil {
		l.Warn("mark failed failed", "err", err)
	}
}

// releaseStep flips a 'processing' row back to 'pending' so the next tick can
// retry. We piggy-back on the stale-lease query by giving it a zero interval.
func (m *Manager) releaseStep(ctx context.Context, l *slog.Logger, id pgtype.UUID) {
	if _, err := m.pool.Exec(ctx,
		`UPDATE prospect_steps
		    SET status = 'pending', processing_started_at = NULL
		  WHERE id = $1 AND status = 'processing'`,
		id,
	); err != nil {
		l.Warn("release step failed", "err", err)
	}
}

// setProspectStatus updates only the status column (the existing query also
// touches error_detail; we don't want to clobber it).
func (m *Manager) setProspectStatus(ctx context.Context, id pgtype.UUID, status string) error {
	_, err := m.pool.Exec(ctx, `UPDATE prospects SET status = $2, updated_at = NOW() WHERE id = $1`, id, status)
	return err
}

// normaliseStepType folds 'message' into 'send_message' so handlers don't have
// to repeat the synonym (the schema allows both).
func normaliseStepType(t string) string {
	if t == "message" {
		return "send_message"
	}
	return t
}

// stepTemplate returns the inline template text of a sequence step, or empty
// if the step is unset / has no template.
func stepTemplate(s *gen.SequenceStep) string {
	if s == nil || s.Template == nil {
		return ""
	}
	return *s.Template
}

// renderTemplate translates a Prospect to template.Lead and renders.
func renderTemplate(tpl string, p *gen.Prospect) (string, error) {
	if tpl == "" {
		return "", nil
	}
	lead := &template.Lead{
		FullName:    strPtr(p.FullName),
		FirstName:   strPtr(p.FirstName),
		Headline:    strPtr(p.Headline),
		Company:     strPtr(p.Company),
		LinkedInURL: p.ProfileUrl,
		Variables:   decodeJSONB(p.Variables),
	}
	if lead.Variables == nil {
		lead.Variables = map[string]any{}
	}
	res, err := template.Render(tpl, lead, nil, nil, template.Options{Strict: false})
	if err != nil {
		return "", err
	}
	return res.Text, nil
}

func strPtr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func decodeJSONB(raw []byte) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	return m
}
