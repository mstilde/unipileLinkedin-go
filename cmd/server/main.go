package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/joho/godotenv"

	"github.com/mstilde/unipile-linkedin-go/internal/ai"
	"github.com/mstilde/unipile-linkedin-go/internal/auth"
	"github.com/mstilde/unipile-linkedin-go/internal/config"
	"github.com/mstilde/unipile-linkedin-go/internal/db"
	"github.com/mstilde/unipile-linkedin-go/internal/db/gen"
	"github.com/mstilde/unipile-linkedin-go/internal/http/api"
	"github.com/mstilde/unipile-linkedin-go/internal/http/ui"
	"github.com/mstilde/unipile-linkedin-go/internal/scheduler"
	"github.com/mstilde/unipile-linkedin-go/internal/unipile"
)

func main() {
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load failed", "err", err)
		os.Exit(1)
	}

	logger := setupLogger(cfg.LogLevel)
	slog.SetDefault(logger)

	rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// DB pool. Without DATABASE_URL the server still starts but DB-backed
	// routes will 500 — useful for /health smoke tests in CI.
	var apiHandler http.Handler
	var uiHandler http.Handler
	var schedMgr *scheduler.Manager
	if cfg.DatabaseURL != "" {
		dbPool, err := db.OpenPool(rootCtx, cfg.DatabaseURL)
		if err != nil {
			slog.Error("db open failed", "err", err)
			os.Exit(1)
		}
		defer dbPool.Close()

		q := gen.New(dbPool)

		// JWT signer (require JWT_SECRET when DB is wired).
		secret := cfg.JWTSecret
		if len(secret) < 32 {
			secret = "dev-secret-do-not-use-in-production-32chars-min!!"
		}
		signer, err := auth.NewSigner(secret, cfg.JWTIssuer, cfg.JWTAudience, cfg.JWTSessionTTL)
		if err != nil {
			slog.Error("signer init failed", "err", err)
			os.Exit(1)
		}

		apiHandler = api.Mount(api.Deps{
			Pool:   dbPool,
			Q:      q,
			Signer: signer,
			Store:  &api.SQLAccountStore{Q: q},
		})
		uiHandler = ui.Mount(ui.Deps{
			Pool:   dbPool,
			Q:      q,
			Signer: signer,
		})

		// Job-postings ranker (front 2B). Built over the Anthropic client, which
		// in this deploy points at OpenCode Go via AI_BASE_URL_ANTHROPIC. Nil when
		// no key is configured — the jobs loop then discovers but doesn't score.
		ranker := buildJobRanker(cfg)
		classifier := buildPostClassifier(cfg)

		schedMgr = scheduler.New(dbPool, q, unipile.NewEnvProvider(), ranker, classifier, scheduler.Config{
			CampaignInterval: cfg.CampaignSchedulerInterval,
			FollowUpInterval: cfg.FollowUpInterval,
			AIQueueInterval:  cfg.AIQueueInterval,
			DryRun:           cfg.DryRun,
			KillswitchGlobal: cfg.KillswitchGlobal,
		}, slog.Default())
		schedMgr.Start(rootCtx)
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","service":"unipile-linkedin-go"}`))
	})

	if apiHandler != nil {
		r.Mount("/api/v1", apiHandler)
	}
	if uiHandler != nil {
		r.Mount("/", uiHandler)
	}

	// Static assets (CSS, JS).
	staticDir := http.Dir("./static")
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(staticDir)))

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		slog.Info("server starting", "port", cfg.Port, "env", cfg.Env, "dry_run", cfg.DryRun, "db", cfg.DatabaseURL != "")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server failed", "err", err)
			os.Exit(1)
		}
	}()

	<-rootCtx.Done()
	slog.Info("shutdown signal received, draining")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("graceful shutdown failed", "err", err)
	}
	if schedMgr != nil {
		schedMgr.Wait()
	}
	slog.Info("server stopped")
}

// buildJobRanker constructs the AI job ranker from config. Returns nil when no
// Anthropic key is set (the jobs loop then discovers postings without scoring).
// AIBaseURLAnthropic routes traffic through OpenCode Go when set.
func buildJobRanker(cfg *config.Config) *ai.JobRanker {
	if cfg.AnthropicAPIKey == "" {
		slog.Warn("no ANTHROPIC_API_KEY; job ranker disabled (postings will not be scored)")
		return nil
	}
	client, err := ai.NewAnthropicClient(cfg.AnthropicAPIKey)
	if err != nil {
		slog.Warn("job ranker init failed; postings will not be scored", "err", err)
		return nil
	}
	if cfg.AIBaseURLAnthropic != "" {
		client = client.WithBaseURL(cfg.AIBaseURLAnthropic)
	}
	return ai.NewJobRanker(client, cfg.AnthropicModelSmart)
}

// buildPostClassifier constructs the AI feed-post classifier from config.
// Returns nil when no Anthropic key is set (the feed loop then discovers posts
// without classifying). Routes through OpenCode Go when AIBaseURLAnthropic set.
func buildPostClassifier(cfg *config.Config) *ai.PostClassifier {
	if cfg.AnthropicAPIKey == "" {
		slog.Warn("no ANTHROPIC_API_KEY; post classifier disabled (feed posts will not be classified)")
		return nil
	}
	client, err := ai.NewAnthropicClient(cfg.AnthropicAPIKey)
	if err != nil {
		slog.Warn("post classifier init failed; feed posts will not be classified", "err", err)
		return nil
	}
	if cfg.AIBaseURLAnthropic != "" {
		client = client.WithBaseURL(cfg.AIBaseURLAnthropic)
	}
	return ai.NewPostClassifier(client, cfg.AnthropicModelSmart)
}

func setupLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}
