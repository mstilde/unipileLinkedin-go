package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Env      string
	Port     string
	LogLevel string

	DatabaseURL string

	JWTSecret     string
	JWTSessionTTL time.Duration
	JWTIssuer     string
	JWTAudience   string

	UnipileDSN           string
	UnipileAPIKey        string
	UnipileWebhookSecret string

	AnthropicAPIKey string
	OpenAIAPIKey    string

	// Optional base-URL overrides so AI traffic can be routed through proxies
	// like OpenCode Zen/Go. Empty = use vendor default.
	AIBaseURLAnthropic string
	AIBaseURLOpenAI    string

	AnthropicModelFast  string
	AnthropicModelSmart string
	OpenAIModelEnrich   string

	AIMonthlyCapUSD float64
	MaxConcurrentAI int

	StorageDriver string
	StorageFSRoot string

	CORSOrigins string

	CampaignSchedulerInterval time.Duration
	AIQueueInterval           time.Duration
	FollowUpInterval          time.Duration
	FollowUpTZ                string
	FollowUpWHStart           int
	FollowUpWHEnd             int
	FollowUpSkipWeekends      bool

	DryRun           bool
	KillswitchGlobal bool
}

func Load() (*Config, error) {
	c := &Config{
		Env:      getEnv("ENV", "development"),
		Port:     getEnv("PORT", "8080"),
		LogLevel: getEnv("LOG_LEVEL", "info"),

		DatabaseURL: os.Getenv("DATABASE_URL"),

		JWTSecret:     os.Getenv("JWT_SECRET"),
		JWTSessionTTL: parseDuration("JWT_SESSION_TTL", 168*time.Hour),
		JWTIssuer:     getEnv("JWT_ISSUER", "unipile-go"),
		JWTAudience:   getEnv("JWT_AUDIENCE", "unipile-go"),

		UnipileDSN:           os.Getenv("UNIPILE_DSN"),
		UnipileAPIKey:        os.Getenv("UNIPILE_API_KEY"),
		UnipileWebhookSecret: os.Getenv("UNIPILE_WEBHOOK_SECRET"),

		AnthropicAPIKey: os.Getenv("ANTHROPIC_API_KEY"),
		OpenAIAPIKey:    os.Getenv("OPENAI_API_KEY"),

		AIBaseURLAnthropic: os.Getenv("AI_BASE_URL_ANTHROPIC"),
		AIBaseURLOpenAI:    os.Getenv("AI_BASE_URL_OPENAI"),

		AnthropicModelFast:  getEnv("ANTHROPIC_MODEL_FAST", "claude-haiku-4-5"),
		AnthropicModelSmart: getEnv("ANTHROPIC_MODEL_SMART", "claude-sonnet-4-6"),
		OpenAIModelEnrich:   getEnv("OPENAI_MODEL_ENRICH", "gpt-4o-mini"),

		AIMonthlyCapUSD: parseFloat("AI_MONTHLY_CAP_USD", 200),
		MaxConcurrentAI: parseInt("MAX_CONCURRENT_AI", 5),

		StorageDriver: getEnv("STORAGE_DRIVER", "fs"),
		StorageFSRoot: getEnv("STORAGE_FS_ROOT", "./cache"),

		CORSOrigins: getEnv("CORS_ORIGINS", "http://localhost:8080"),

		CampaignSchedulerInterval: parseDuration("CAMPAIGN_SCHEDULER_INTERVAL", 15*time.Minute),
		AIQueueInterval:           parseDuration("AIQUEUE_INTERVAL", 30*time.Second),
		FollowUpInterval:          parseDuration("FOLLOWUP_INTERVAL", 15*time.Minute),
		FollowUpTZ:                getEnv("FOLLOWUP_TZ", "America/Argentina/Buenos_Aires"),
		FollowUpWHStart:           parseInt("FOLLOWUP_WH_START", 9),
		FollowUpWHEnd:             parseInt("FOLLOWUP_WH_END", 19),
		FollowUpSkipWeekends:      parseBool("FOLLOWUP_SKIP_WEEKENDS", true),

		DryRun:           parseBool("DRY_RUN", true),
		KillswitchGlobal: parseBool("KILLSWITCH_GLOBAL", false),
	}

	if c.Env == "production" {
		if err := c.validateProduction(); err != nil {
			return nil, err
		}
	}

	return c, nil
}

func (c *Config) validateProduction() error {
	if len(c.JWTSecret) < 32 {
		return fmt.Errorf("JWT_SECRET must be at least 32 chars in production")
	}
	if c.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	return nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func parseDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func parseInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return def
}

func parseFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

func parseBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}
