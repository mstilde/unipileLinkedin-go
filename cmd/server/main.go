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

	"github.com/mstilde/unipile-linkedin-go/internal/auth"
	"github.com/mstilde/unipile-linkedin-go/internal/config"
	"github.com/mstilde/unipile-linkedin-go/internal/db"
	"github.com/mstilde/unipile-linkedin-go/internal/db/gen"
	"github.com/mstilde/unipile-linkedin-go/internal/http/api"
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
	var pool *gen.Queries
	var apiHandler http.Handler
	if cfg.DatabaseURL != "" {
		dbPool, err := db.OpenPool(rootCtx, cfg.DatabaseURL)
		if err != nil {
			slog.Error("db open failed", "err", err)
			os.Exit(1)
		}
		defer dbPool.Close()

		q := gen.New(dbPool)
		_ = pool

		// JWT signer (require JWT_SECRET when DB is wired).
		secret := cfg.JWTSecret
		if len(secret) < 32 {
			// Use a development default so the server boots locally without
			// extra setup. Refuses to start in production via config validation.
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
	slog.Info("server stopped")
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
