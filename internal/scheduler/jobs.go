package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/mstilde/unipile-linkedin-go/internal/ai"
	"github.com/mstilde/unipile-linkedin-go/internal/db/gen"
	"github.com/mstilde/unipile-linkedin-go/internal/unipile"
)

// jobsScoreBatch bounds how many postings we score per tick, to cap LLM spend
// and keep a tick short. With 1-2 accounts and ~20 new postings/day this drains
// comfortably.
const jobsScoreBatch = 10

// tickJobs is one iteration of the jobs loop (front 2B). Two phases:
//
//  1. discovery — run each saved search whose last_run is older than
//     JobsRediscover, store new postings as 'new'
//  2. scoring — for every account with enabled searches, fetch the JD and rank
//     a batch of unscored postings via the AI ranker
//
// Discovery runs even without a ranker (postings just stay 'new'); scoring is
// skipped when no ranker is configured.
func (m *Manager) tickJobs(ctx context.Context) {
	l := m.log.With("scheduler", "jobs")

	cli, err := m.unipile.Get()
	if err != nil {
		l.Warn("no unipile client; skipping jobs tick", "err", err)
		return
	}

	// ---- Phase 1: discovery ----
	rediscover := pgtype.Interval{
		Microseconds: int64(m.cfg.JobsRediscover / time.Microsecond),
		Valid:        true,
	}
	due, err := m.q.ListDueJobSearches(ctx, gen.ListDueJobSearchesParams{
		Column1: rediscover,
		Limit:   m.cfg.BatchSize,
	})
	if err != nil {
		l.Error("list due job searches failed", "err", err)
	}
	for _, s := range due {
		m.runJobSearch(ctx, l, cli, s)
	}

	// ---- Phase 2: scoring ----
	if m.ranker == nil {
		if len(due) > 0 {
			l.Debug("no ranker configured; postings left unscored")
		}
		return
	}
	accounts, err := m.q.ListEnabledJobSearchAccounts(ctx)
	if err != nil {
		l.Error("list job-search accounts failed", "err", err)
		return
	}
	for _, acct := range accounts {
		m.scoreUnscoredJobs(ctx, l, cli, acct)
	}
}

// runJobSearch executes one saved search and stores its organic (non-promoted)
// hits as new postings, then stamps last_run_at.
func (m *Manager) runJobSearch(ctx context.Context, l *slog.Logger, cli *unipile.Client, s gen.JobSearch) {
	l = l.With("search_id", s.ID, "search", s.Name)

	unipileAcctID, ok := m.resolveUnipileAccount(ctx, l, s.AccountID)
	if !ok {
		return
	}

	searchURL := ""
	if s.SearchUrl != nil {
		searchURL = *s.SearchUrl
	}
	res, err := cli.SearchJobs(ctx, unipile.SearchJobsParams{
		AccountID: unipileAcctID,
		Keywords:  s.Keywords,
		SearchURL: searchURL,
		GeoID:     s.GeoID,
		Limit:     int(s.MaxResults),
	})
	if err != nil {
		// A failed search shouldn't poison the loop; log and move on. We do not
		// stamp last_run so the next tick retries.
		l.Warn("job search failed", "err", err)
		return
	}

	stored := 0
	for _, hit := range res.Items {
		if hit.Promoted {
			continue // skip sponsored ads (wall #2)
		}
		_, err := m.q.InsertJobPosting(ctx, gen.InsertJobPostingParams{
			AccountID:     s.AccountID,
			SearchID:      s.ID,
			LinkedinJobID: hit.ID,
			Title:         hit.Title,
			Company:       hit.Company,
			Location:      strOrNil(hit.Location),
			URL:           strOrNil(hit.URL),
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				continue // already seen — InsertJobPosting does ON CONFLICT DO NOTHING
			}
			l.Warn("insert job posting failed", "job_id", hit.ID, "err", err)
			continue
		}
		stored++
	}

	if err := m.q.TouchJobSearch(ctx, gen.TouchJobSearchParams{
		ID:            s.ID,
		LastSeenCount: int32(len(res.Items)),
	}); err != nil {
		l.Warn("touch job search failed", "err", err)
	}
	l.Info("job search done", "hits", len(res.Items), "new", stored, "dry_run", res.DryRun)
}

// scoreUnscoredJobs ranks a batch of an account's unscored postings. For each
// it fetches the full JD (needed for a good score), ranks it, and stores the
// result. The CV summary + preferences come from the account's client_profile.
func (m *Manager) scoreUnscoredJobs(ctx context.Context, l *slog.Logger, cli *unipile.Client, accountID string) {
	rows, err := m.q.ListUnscoredJobPostings(ctx, gen.ListUnscoredJobPostingsParams{
		AccountID: accountID,
		Limit:     jobsScoreBatch,
	})
	if err != nil {
		l.Error("list unscored postings failed", "account", accountID, "err", err)
		return
	}
	if len(rows) == 0 {
		return
	}

	cvSummary := m.accountCVSummary(ctx, accountID)
	unipileAcctID, ok := m.resolveUnipileAccount(ctx, l, accountID)
	if !ok {
		return
	}

	for _, p := range rows {
		m.scoreOneJob(ctx, l, cli, unipileAcctID, cvSummary, p)
	}
}

// scoreOneJob fetches the JD, ranks the posting, and persists the score.
func (m *Manager) scoreOneJob(ctx context.Context, l *slog.Logger, cli *unipile.Client, unipileAcctID, cvSummary string, p gen.JobPosting) {
	l = l.With("posting_id", p.ID, "job_id", p.LinkedinJobID)

	detail, err := cli.GetJob(ctx, unipileAcctID, p.LinkedinJobID)
	if err != nil {
		reason := fmt.Sprintf("fetch JD: %v", err)
		m.failJobScore(ctx, l, p.ID, reason)
		return
	}

	jd := detail.Description
	in := ai.JobRankInput{
		CVSummary:   cvSummary,
		Title:       firstNonEmptyJob(detail.Title, p.Title),
		Company:     firstNonEmptyJob(detail.Company, p.Company),
		Location:    firstNonEmptyJob(detail.Location, derefStr(p.Location)),
		Description: jd,
	}
	res, err := m.ranker.RankJob(ctx, in)
	if err != nil {
		// Transient provider errors (429/5xx/transport) shouldn't permanently
		// dismiss a posting — leave it 'new' so a later tick retries.
		if ai.IsTransientError(err) {
			l.Warn("rank transient error; leaving posting for retry", "err", err)
			return
		}
		m.failJobScore(ctx, l, p.ID, fmt.Sprintf("rank: %v", err))
		return
	}

	tagsJSON, _ := json.Marshal(res.Tags)
	score := int32(res.Score)
	model := res.Model
	reasoning := res.Reasoning

	var postedAt pgtype.Timestamptz
	if detail.PostedAt != nil {
		postedAt = pgtype.Timestamptz{Time: *detail.PostedAt, Valid: true}
	}
	var applicants *int32
	if detail.ApplicantsCount > 0 {
		a := int32(detail.ApplicantsCount)
		applicants = &a
	}

	if err := m.q.SetJobPostingScore(ctx, gen.SetJobPostingScoreParams{
		ID:              p.ID,
		AiScore:         &score,
		AiReasoning:     &reasoning,
		AiTags:          tagsJSON,
		AiModel:         &model,
		RawJd:           strOrNil(jd),
		ApplicantsCount: applicants,
		PostedAt:        postedAt,
	}); err != nil {
		l.Warn("store job score failed", "err", err)
		return
	}
	l.Info("job scored", "score", res.Score, "model", res.Model, "cost_usd", res.CostUSD)
}

func (m *Manager) failJobScore(ctx context.Context, l *slog.Logger, id pgtype.UUID, reason string) {
	l.Warn("job score failed", "reason", reason)
	if err := m.q.MarkJobPostingScoreFailed(ctx, gen.MarkJobPostingScoreFailedParams{
		ID:          id,
		AiReasoning: &reason,
	}); err != nil {
		l.Warn("mark job score failed failed", "err", err)
	}
}

// accountCVSummary returns the merged_summary from the account's client_profile,
// or empty if none is set.
func (m *Manager) accountCVSummary(ctx context.Context, accountID string) string {
	cp, err := m.q.GetClientProfileByAccount(ctx, accountID)
	if err != nil {
		return ""
	}
	if cp.MergedSummary != nil {
		return *cp.MergedSummary
	}
	return ""
}

// resolveUnipileAccount maps a local accounts.id to the Unipile vendor
// account_id the API expects. Mirrors the lookup in dispatchStep.
func (m *Manager) resolveUnipileAccount(ctx context.Context, l *slog.Logger, localAccountID string) (string, bool) {
	acct, err := m.q.GetAccountByID(ctx, localAccountID)
	if err != nil {
		l.Warn("resolve unipile account failed", "account", localAccountID, "err", err)
		return "", false
	}
	if acct.AccountID == nil || *acct.AccountID == "" {
		l.Warn("account has no Unipile account_id", "account", localAccountID)
		return "", false
	}
	return *acct.AccountID, true
}

func strOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func firstNonEmptyJob(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
