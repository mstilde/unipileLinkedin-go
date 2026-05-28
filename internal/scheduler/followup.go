package scheduler

import (
	"context"

	"github.com/mstilde/unipile-linkedin-go/internal/db/gen"
)

// tickFollowUp dispatches due follow-up tasks. Currently a stub: in dry-run
// marks them "sent" with a synthetic note; in real mode cancels with "not implemented".
func (m *Manager) tickFollowUp(ctx context.Context) {
	l := m.log.With("scheduler", "followup")

	rows, err := m.q.ListDueFollowUpTasks(ctx, m.cfg.BatchSize)
	if err != nil {
		l.Error("list due follow-ups failed", "err", err)
		return
	}
	if len(rows) == 0 {
		return
	}
	l.Debug("dispatching", "count", len(rows))

	for _, task := range rows {
		if m.cfg.DryRun {
			note := "dry-run follow-up: " + task.TriggerStatus
			if err := m.q.MarkFollowUpTaskSent(ctx, gen.MarkFollowUpTaskSentParams{
				ID:          task.ID,
				MessageSent: &note,
			}); err != nil {
				l.Warn("mark sent failed", "id", task.ID, "err", err)
			}
			continue
		}
		reason := "follow-up dispatch not implemented yet"
		if err := m.q.MarkFollowUpTaskCancelled(ctx, gen.MarkFollowUpTaskCancelledParams{
			ID:           task.ID,
			CancelReason: &reason,
		}); err != nil {
			l.Warn("mark cancelled failed", "id", task.ID, "err", err)
		}
	}
}
