package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/mstilde/unipile-linkedin-go/internal/db/gen"
)

// tickCampaign is the campaign scheduler iteration: re-arm stale leases, fetch
// the pending steps that are due, and dispatch each one. Dispatch is currently
// a no-op (marks "sent" in dry-run, "failed" otherwise) — wiring Unipile
// actions is the next task.
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

func (m *Manager) dispatchStep(ctx context.Context, l *slog.Logger, row gen.ListPendingProspectStepsRow) {
	step, err := m.q.LeaseProspectStep(ctx, row.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return
		}
		l.Warn("lease failed", "id", row.ID, "err", err)
		return
	}

	if m.cfg.DryRun {
		note := "dry-run: " + step.StepType
		if err := m.q.MarkProspectStepSent(ctx, gen.MarkProspectStepSentParams{
			ID:          step.ID,
			MessageSent: &note,
		}); err != nil {
			l.Warn("mark sent failed", "id", step.ID, "err", err)
		}
		l.Debug("dry-run step dispatched", "step_type", step.StepType, "id", step.ID)
		return
	}

	reason := "dispatch not implemented yet"
	if err := m.q.MarkProspectStepFailed(ctx, gen.MarkProspectStepFailedParams{
		ID:          step.ID,
		ErrorDetail: &reason,
	}); err != nil {
		l.Warn("mark failed", "id", step.ID, "err", err)
	}
}
