package scheduler

import (
	"context"

	"github.com/mstilde/unipile-linkedin-go/internal/db/gen"
)

// tickAIQueue drains the ai_reply_queue: classify+generate+send. Currently
// a stub that just marks rows "sent" with a placeholder draft in dry-run.
func (m *Manager) tickAIQueue(ctx context.Context) {
	l := m.log.With("scheduler", "aiqueue")

	rows, err := m.q.ListPendingAIReplies(ctx, m.cfg.BatchSize)
	if err != nil {
		l.Error("list ai queue failed", "err", err)
		return
	}
	if len(rows) == 0 {
		return
	}
	l.Debug("processing", "count", len(rows))

	for _, row := range rows {
		if m.cfg.DryRun {
			draft := "[dry-run draft] reply to: " + row.IncomingText
			if err := m.q.MarkAIReplyDone(ctx, gen.MarkAIReplyDoneParams{
				ID:      row.ID,
				AiDraft: &draft,
			}); err != nil {
				l.Warn("mark done failed", "id", row.ID, "err", err)
			}
			continue
		}
		reason := "ai dispatch not implemented yet"
		if err := m.q.MarkAIReplyFailed(ctx, gen.MarkAIReplyFailedParams{
			ID:          row.ID,
			ErrorDetail: &reason,
		}); err != nil {
			l.Warn("mark failed", "id", row.ID, "err", err)
		}
	}
}
