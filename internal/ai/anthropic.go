package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// AnthropicClient is a minimal HTTP client for the Claude Messages API.
// Safe for concurrent use after construction.
type AnthropicClient struct {
	apiKey     string
	apiVersion string
	httpClient *http.Client
	baseURL    string
}

// NewAnthropicClient builds a Claude client. Returns error if apiKey is empty.
func NewAnthropicClient(apiKey string) (*AnthropicClient, error) {
	if apiKey == "" {
		return nil, errors.New("ai: ANTHROPIC_API_KEY required")
	}
	return &AnthropicClient{
		apiKey:     apiKey,
		apiVersion: "2023-06-01",
		httpClient: &http.Client{Timeout: 60 * time.Second},
		baseURL:    "https://api.anthropic.com",
	}, nil
}

// WithBaseURL overrides the API base URL (for tests pointing at httptest).
func (c *AnthropicClient) WithBaseURL(base string) *AnthropicClient {
	cp := *c
	cp.baseURL = base
	return &cp
}

// WithHTTPClient swaps the underlying http.Client (for tests).
func (c *AnthropicClient) WithHTTPClient(hc *http.Client) *AnthropicClient {
	cp := *c
	cp.httpClient = hc
	return &cp
}

// SystemBlock is one entry of the system prompt array. cache_control={type:ephemeral}
// enables Anthropic's prompt cache (free for our usage tier).
type SystemBlock struct {
	Type         string            `json:"type"`
	Text         string            `json:"text"`
	CacheControl *anthropicCache   `json:"cache_control,omitempty"`
}

type anthropicCache struct {
	Type string `json:"type"`
}

// WithCache returns a system block with ephemeral cache_control set.
func (b SystemBlock) WithCache() SystemBlock {
	b.CacheControl = &anthropicCache{Type: "ephemeral"}
	return b
}

// Message is one turn of conversation (role: "user" | "assistant").
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// MarshalJSON serializes content as a one-element block array
// ([{type:"text",text:...}]) rather than a bare string. Real Anthropic accepts
// both forms, but OpenCode's Anthropic→OpenAI proxy silently drops bare-string
// content (the model then sees only the system prompt). Block form keeps both
// backends working.
func (m Message) MarshalJSON() ([]byte, error) {
	type block struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	return json.Marshal(struct {
		Role    string  `json:"role"`
		Content []block `json:"content"`
	}{
		Role:    m.Role,
		Content: []block{{Type: "text", Text: m.Content}},
	})
}

// MessagesRequest is the request body for /v1/messages.
type MessagesRequest struct {
	Model     string        `json:"model"`
	MaxTokens int           `json:"max_tokens"`
	System    []SystemBlock `json:"system,omitempty"`
	Messages  []Message     `json:"messages"`
}

// MessagesResponse is the success body of /v1/messages (subset of fields).
type MessagesResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Role    string `json:"role"`
	Type    string `json:"type"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Usage      Usage  `json:"usage"`
}

// Text returns the concatenated text from all content blocks.
func (r *MessagesResponse) Text() string {
	var buf bytes.Buffer
	for _, c := range r.Content {
		if c.Type == "text" {
			buf.WriteString(c.Text)
		}
	}
	return buf.String()
}

// Create calls POST /v1/messages with retry+backoff for 429/5xx (3 attempts).
// Returns the decoded response or an error.
func (c *AnthropicClient) Create(ctx context.Context, req MessagesRequest) (*MessagesResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("ai: marshal anthropic request: %w", err)
	}

	var last error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoffDelay(attempt)):
			}
		}
		resp, err := c.doCreate(ctx, body)
		if err == nil {
			return resp, nil
		}
		last = err
		if !isRetryable(err) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("ai: anthropic create failed after 3 attempts: %w", last)
}

func (c *AnthropicClient) doCreate(ctx context.Context, body []byte) (*MessagesResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ai: new anthropic req: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", c.apiVersion)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &ProviderError{Status: 0, Body: err.Error(), Retryable: true}
	}
	defer resp.Body.Close()

	rb, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, &ProviderError{
			Status:    resp.StatusCode,
			Body:      string(rb),
			Retryable: resp.StatusCode == 429 || (resp.StatusCode >= 500 && resp.StatusCode < 600),
		}
	}

	var out MessagesResponse
	if err := json.Unmarshal(rb, &out); err != nil {
		return nil, fmt.Errorf("ai: decode anthropic response: %w", err)
	}
	return &out, nil
}
