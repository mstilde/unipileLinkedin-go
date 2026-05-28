package unipile

import (
	"os"
	"sync"
)

// Provider returns a fresh *Client built from current credentials. Used by the
// scheduler so weekly free-tier rotation of UNIPILE_DSN / UNIPILE_API_KEY takes
// effect without a restart — each tick re-reads env and rebuilds the client.
//
// Construction is cheap (no connection pool); the underlying http.Client
// transport is shared so keep-alives still work.
type Provider struct {
	read   func() (dsn, apiKey string, dryRun bool)
	mu     sync.Mutex
	last   string // "dsn|apiKey|dryRun" — to log only on change
	shared *Client
}

// NewEnvProvider reads UNIPILE_DSN, UNIPILE_API_KEY, and DRY_RUN from the
// environment on every Get(). dryRun defaults to true if the env var is missing
// or unparseable.
func NewEnvProvider() *Provider {
	return &Provider{
		read: func() (string, string, bool) {
			dryRun := true
			if v := os.Getenv("DRY_RUN"); v == "false" || v == "0" {
				dryRun = false
			}
			return os.Getenv("UNIPILE_DSN"), os.Getenv("UNIPILE_API_KEY"), dryRun
		},
	}
}

// NewStaticProvider returns a Provider that always yields the same Client.
// Useful in tests.
func NewStaticProvider(c *Client) *Provider {
	return &Provider{shared: c}
}

// Get returns a client for the current credentials. If the env values haven't
// changed since the last call it returns the same cached *Client; otherwise
// a fresh one is built (and the old one is discarded — no open sockets to
// release since http.Client manages its own keep-alive pool).
func (p *Provider) Get() (*Client, error) {
	if p.shared != nil {
		return p.shared, nil
	}
	dsn, apiKey, dryRun := p.read()

	p.mu.Lock()
	defer p.mu.Unlock()

	key := dsn + "|" + apiKey + "|" + boolStr(dryRun)
	if p.shared != nil && key == p.last {
		return p.shared, nil
	}
	c, err := NewClient(dsn, apiKey, dryRun)
	if err != nil {
		return nil, err
	}
	p.shared = c
	p.last = key
	return c, nil
}

func boolStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
