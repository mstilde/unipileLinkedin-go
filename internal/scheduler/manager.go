// Package scheduler runs three background loops that drive the campaign engine:
// the campaign scheduler (executes pending prospect_steps), the follow-up
// scheduler (sends scheduled follow-ups), and the AI reply scheduler (drains
// the ai_reply_queue). Each loop is a single goroutine; the Manager wires them
// up and propagates a shared context for graceful shutdown.
package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mstilde/unipile-linkedin-go/internal/db/gen"
)

// Config bundles the cadences each loop runs at. Zero values fall back to
// reasonable defaults; nothing is mandatory.
type Config struct {
	CampaignInterval time.Duration
	FollowUpInterval time.Duration
	AIQueueInterval  time.Duration
	BatchSize        int32         // rows per tick (default 50)
	StaleLeaseAge    time.Duration // re-arm processing rows older than this
	DryRun           bool          // when true, side-effecting actions are logged but not invoked
}

func (c Config) withDefaults() Config {
	if c.CampaignInterval <= 0 {
		c.CampaignInterval = 15 * time.Minute
	}
	if c.FollowUpInterval <= 0 {
		c.FollowUpInterval = 15 * time.Minute
	}
	if c.AIQueueInterval <= 0 {
		c.AIQueueInterval = 30 * time.Second
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 50
	}
	if c.StaleLeaseAge <= 0 {
		c.StaleLeaseAge = 15 * time.Minute
	}
	return c
}

// Manager owns the three loops. Construct with New, then call Start. Stop
// is implicit: cancel the context passed to Start.
type Manager struct {
	cfg  Config
	pool *pgxpool.Pool
	q    *gen.Queries
	log  *slog.Logger

	wg sync.WaitGroup
}

func New(pool *pgxpool.Pool, q *gen.Queries, cfg Config, log *slog.Logger) *Manager {
	if log == nil {
		log = slog.Default()
	}
	return &Manager{
		cfg:  cfg.withDefaults(),
		pool: pool,
		q:    q,
		log:  log,
	}
}

// Start launches the three loops. It returns immediately; the caller should
// pass a cancellable context and wait on Wait() to know when shutdown completes.
func (m *Manager) Start(ctx context.Context) {
	m.wg.Add(3)
	go m.runLoop(ctx, "campaign", m.cfg.CampaignInterval, m.tickCampaign)
	go m.runLoop(ctx, "followup", m.cfg.FollowUpInterval, m.tickFollowUp)
	go m.runLoop(ctx, "aiqueue", m.cfg.AIQueueInterval, m.tickAIQueue)
}

// Wait blocks until all loops have drained after their context was cancelled.
func (m *Manager) Wait() {
	m.wg.Wait()
}

// runLoop is the shared driver for all three schedulers: fire `tick` once on
// start (so cold deploys don't have to wait a full interval), then again on
// every Ticker pulse until ctx is done.
func (m *Manager) runLoop(ctx context.Context, name string, interval time.Duration, tick func(context.Context)) {
	defer m.wg.Done()
	l := m.log.With("scheduler", name)
	l.Info("starting", "interval", interval, "dry_run", m.cfg.DryRun)

	// Run once on start.
	m.safeTick(ctx, l, tick)

	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			l.Info("stopping")
			return
		case <-t.C:
			m.safeTick(ctx, l, tick)
		}
	}
}

// safeTick guards a tick function from panics so one bad row can't take down
// the loop. Errors are logged; the next tick re-tries.
func (m *Manager) safeTick(ctx context.Context, l *slog.Logger, tick func(context.Context)) {
	defer func() {
		if r := recover(); r != nil {
			l.Error("tick panicked", "recover", r)
		}
	}()
	tick(ctx)
}
