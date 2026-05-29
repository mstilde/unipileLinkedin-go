package unipile

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultTimeout is the per-request timeout used when none is set on the
// Context. Unipile's slowest endpoints (search jobs) can take 20s+; for action
// endpoints 30s is a safe ceiling.
const DefaultTimeout = 30 * time.Second

// Client talks to the Unipile API at a given DSN with a given API key.
// Safe for concurrent use after construction. Zero-value Client is unusable —
// use NewClient.
type Client struct {
	dsn        string // e.g. "api1.unipile.com:13109"
	apiKey     string
	httpClient *http.Client
	dryRun     bool
}

// NewClient builds a Client. dsn should NOT include scheme; we always use
// https://. apiKey is the X-API-KEY header value.
//
// If dryRun is true, action methods (SendInvitation, SendMessage, …) return a
// stub response instead of hitting the wire. Useful for first-deploy safety.
func NewClient(dsn, apiKey string, dryRun bool) (*Client, error) {
	if dsn == "" {
		return nil, errors.New("unipile: dsn is required")
	}
	if apiKey == "" {
		return nil, errors.New("unipile: api key is required")
	}
	// Strip scheme if user accidentally included one.
	dsn = strings.TrimPrefix(dsn, "https://")
	dsn = strings.TrimPrefix(dsn, "http://")
	dsn = strings.TrimSuffix(dsn, "/")
	return &Client{
		dsn:        dsn,
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: DefaultTimeout},
		dryRun:     dryRun,
	}, nil
}

// WithHTTPClient lets tests swap in an httptest.Server-backed client.
// The returned Client shares the same dsn/apiKey but uses the given transport.
func (c *Client) WithHTTPClient(hc *http.Client) *Client {
	cp := *c
	cp.httpClient = hc
	return &cp
}

// IsDryRun reports whether the client suppresses real Unipile calls.
func (c *Client) IsDryRun() bool { return c.dryRun }

// baseURL builds "https://<dsn>" — overridable so tests can point at an
// httptest server (the test sets dsn = "127.0.0.1:NNNN" and we still emit
// "https://" but the WithHTTPClient transport ignores TLS).
func (c *Client) baseURL() string {
	return "https://" + c.dsn
}

// do builds a JSON request, sends it, decodes either a success body into out or
// an APIError on non-2xx. Pass nil for body to send no body (GET / DELETE).
// Pass nil for out to skip decoding the success body.
func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, DefaultTimeout)
		defer cancel()
	}

	var reqBody io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("unipile: marshal body: %w", err)
		}
		reqBody = bytes.NewReader(buf)
	}

	u, err := buildURL(c.baseURL(), path)
	if err != nil {
		return fmt.Errorf("unipile: build url: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, reqBody)
	if err != nil {
		return fmt.Errorf("unipile: new request: %w", err)
	}
	req.Header.Set("X-API-KEY", c.apiKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("unipile: http: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return parseAPIError(resp.StatusCode, respBody)
	}
	if out == nil || len(respBody) == 0 {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("unipile: decode response: %w", err)
	}
	return nil
}

// buildURL joins base + path while preserving any query string on path.
// url.JoinPath escapes the whole element (turning "?" into "%3F"), so we split
// the query off, join only the path part, and re-append the raw query.
func buildURL(base, path string) (string, error) {
	pathPart, query, hasQuery := strings.Cut(path, "?")
	u, err := url.JoinPath(base, pathPart)
	if err != nil {
		return "", err
	}
	if hasQuery {
		u += "?" + query
	}
	return u, nil
}

// doMultipart sends a multipart/form-data POST. fields is the set of
// form-text fields. out gets the decoded success body.
//
// startNewChat is the only known endpoint that needs multipart (rest take JSON).
func (c *Client) doMultipart(ctx context.Context, path string, fields map[string]string, out any) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, DefaultTimeout)
		defer cancel()
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			return fmt.Errorf("unipile: multipart write %s: %w", k, err)
		}
	}
	if err := mw.Close(); err != nil {
		return fmt.Errorf("unipile: multipart close: %w", err)
	}

	u, err := buildURL(c.baseURL(), path)
	if err != nil {
		return fmt.Errorf("unipile: build url: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, &buf)
	if err != nil {
		return fmt.Errorf("unipile: new request: %w", err)
	}
	req.Header.Set("X-API-KEY", c.apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("unipile: http: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return parseAPIError(resp.StatusCode, respBody)
	}
	if out == nil || len(respBody) == 0 {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("unipile: decode response: %w", err)
	}
	return nil
}
