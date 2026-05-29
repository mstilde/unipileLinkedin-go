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

// feedClassifyBatch bounds how many posts we classify per tick (LLM spend cap).
const feedClassifyBatch = 10

// tickFeed is one iteration of the feed loop (front 4: post-feed scanner). Two
// phases, mirroring the jobs loop:
//
//  1. discovery — run each feed search whose last_run is older than
//     FeedRediscover, store new posts as 'new'
//  2. classification — for every account with enabled searches, classify a
//     batch of unclassified posts via the AI post classifier
func (m *Manager) tickFeed(ctx context.Context) {
	l := m.log.With("scheduler", "feed")

	cli, err := m.unipile.Get()
	if err != nil {
		l.Warn("no unipile client; skipping feed tick", "err", err)
		return
	}

	// ---- Phase 1: discovery ----
	rediscover := pgtype.Interval{
		Microseconds: int64(m.cfg.FeedRediscover / time.Microsecond),
		Valid:        true,
	}
	due, err := m.q.ListDueFeedSearches(ctx, gen.ListDueFeedSearchesParams{
		Column1: rediscover,
		Limit:   m.cfg.BatchSize,
	})
	if err != nil {
		l.Error("list due feed searches failed", "err", err)
	}
	for _, s := range due {
		m.runFeedSearch(ctx, l, cli, s)
	}

	// ---- Phase 2: classification ----
	if m.classifier == nil {
		if len(due) > 0 {
			l.Debug("no classifier configured; posts left unclassified")
		}
		return
	}
	accounts, err := m.q.ListEnabledFeedSearchAccounts(ctx)
	if err != nil {
		l.Error("list feed-search accounts failed", "err", err)
		return
	}
	for _, acct := range accounts {
		m.classifyUnscoredPosts(ctx, l, acct)
	}
}

// runFeedSearch executes one feed search and stores its hits as new posts.
func (m *Manager) runFeedSearch(ctx context.Context, l *slog.Logger, cli *unipile.Client, s gen.FeedSearch) {
	l = l.With("search_id", s.ID, "search", s.Name)

	unipileAcctID, ok := m.resolveUnipileAccount(ctx, l, s.AccountID)
	if !ok {
		return
	}
	searchURL := ""
	if s.SearchUrl != nil {
		searchURL = *s.SearchUrl
	}
	res, err := cli.SearchPosts(ctx, unipile.SearchPostsParams{
		AccountID: unipileAcctID,
		Keywords:  s.Keywords,
		SearchURL: searchURL,
		Limit:     int(s.MaxResults),
	})
	if err != nil {
		l.Warn("feed search failed", "err", err)
		return
	}

	stored := 0
	for _, hit := range res.Items {
		var postedAt pgtype.Timestamptz
		if hit.PostedAt != nil {
			postedAt = pgtype.Timestamptz{Time: *hit.PostedAt, Valid: true}
		}
		_, err := m.q.InsertFeedPost(ctx, gen.InsertFeedPostParams{
			AccountID:        s.AccountID,
			SearchID:         s.ID,
			LinkedinPostID:   hit.ID,
			PostUrl:          strOrNil(hit.URL),
			Text:             hit.Text,
			AuthorName:       strOrNil(hit.AuthorName),
			AuthorHeadline:   strOrNil(hit.AuthorHeadline),
			AuthorProviderID: strOrNil(hit.AuthorProviderID),
			AuthorProfileUrl: strOrNil(hit.AuthorProfileURL),
			ReactionsCount:   int32OrNil(hit.ReactionsCount),
			CommentsCount:    int32OrNil(hit.CommentsCount),
			PostedAt:         postedAt,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				continue // already seen
			}
			l.Warn("insert feed post failed", "post_id", hit.ID, "err", err)
			continue
		}
		stored++
	}

	if err := m.q.TouchFeedSearch(ctx, gen.TouchFeedSearchParams{
		ID:            s.ID,
		LastSeenCount: int32(len(res.Items)),
	}); err != nil {
		l.Warn("touch feed search failed", "err", err)
	}
	l.Info("feed search done", "hits", len(res.Items), "new", stored, "dry_run", res.DryRun)
}

// classifyUnscoredPosts classifies a batch of an account's unclassified posts.
func (m *Manager) classifyUnscoredPosts(ctx context.Context, l *slog.Logger, accountID string) {
	rows, err := m.q.ListUnscoredFeedPosts(ctx, gen.ListUnscoredFeedPostsParams{
		AccountID: accountID,
		Limit:     feedClassifyBatch,
	})
	if err != nil {
		l.Error("list unclassified posts failed", "account", accountID, "err", err)
		return
	}
	if len(rows) == 0 {
		return
	}
	cvSummary := m.accountCVSummary(ctx, accountID)
	for _, p := range rows {
		m.classifyOnePost(ctx, l, cvSummary, p)
	}
}

// classifyOnePost runs the classifier and persists the result.
func (m *Manager) classifyOnePost(ctx context.Context, l *slog.Logger, cvSummary string, p gen.FeedPost) {
	l = l.With("post_id", p.ID, "linkedin_post_id", p.LinkedinPostID)

	res, err := m.classifier.ClassifyPost(ctx, ai.PostClassifyInput{
		CVSummary:      cvSummary,
		AuthorName:     derefStr(p.AuthorName),
		AuthorHeadline: derefStr(p.AuthorHeadline),
		Text:           p.Text,
	})
	if err != nil {
		// Transient provider errors leave the post 'new' for a later retry.
		if ai.IsTransientError(err) {
			l.Warn("classify transient error; leaving post for retry", "err", err)
			return
		}
		reason := fmt.Sprintf("classify: %v", err)
		if e := m.q.MarkFeedPostClassifyFailed(ctx, gen.MarkFeedPostClassifyFailedParams{
			ID: p.ID, AiReasoning: &reason,
		}); e != nil {
			l.Warn("mark classify failed failed", "err", e)
		}
		return
	}

	status := "irrelevant"
	if res.Relevant {
		status = "relevant"
	}
	tagsJSON, _ := json.Marshal(res.Tags)
	score := int32(res.Score)
	relevant := res.Relevant
	reasoning := res.Reasoning
	model := res.Model

	if err := m.q.SetFeedPostClassification(ctx, gen.SetFeedPostClassificationParams{
		ID:          p.ID,
		AiRelevant:  &relevant,
		AiScore:     &score,
		AiReasoning: &reasoning,
		AiRole:      strOrNil(res.Role),
		AiCompany:   strOrNil(res.Company),
		AiTags:      tagsJSON,
		AiModel:     &model,
		Status:      status,
	}); err != nil {
		l.Warn("store post classification failed", "err", err)
		return
	}
	l.Info("post classified", "relevant", res.Relevant, "score", res.Score, "model", res.Model, "cost_usd", res.CostUSD)
}

func int32OrNil(n int) *int32 {
	if n <= 0 {
		return nil
	}
	v := int32(n)
	return &v
}
