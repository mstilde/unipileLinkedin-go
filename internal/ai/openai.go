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

// OpenAIClient is a minimal HTTP client for /v1/chat/completions.
type OpenAIClient struct {
	apiKey     string
	httpClient *http.Client
	baseURL    string
}

// NewOpenAIClient builds an OpenAI client. Returns error if apiKey is empty.
func NewOpenAIClient(apiKey string) (*OpenAIClient, error) {
	if apiKey == "" {
		return nil, errors.New("ai: OPENAI_API_KEY required")
	}
	return &OpenAIClient{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 60 * time.Second},
		baseURL:    "https://api.openai.com",
	}, nil
}

// WithBaseURL overrides the API base URL (tests).
func (c *OpenAIClient) WithBaseURL(base string) *OpenAIClient {
	cp := *c
	cp.baseURL = base
	return &cp
}

// WithHTTPClient swaps the underlying http.Client (tests).
func (c *OpenAIClient) WithHTTPClient(hc *http.Client) *OpenAIClient {
	cp := *c
	cp.httpClient = hc
	return &cp
}

// ChatMessage is one entry of the messages array (role: "system"|"user"|"assistant").
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest is the request body of /v1/chat/completions.
type ChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
}

// ChatResponse is the success body (subset).
type ChatResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	UsageRaw struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// Text returns the assistant's reply text (choices[0].message.content).
func (r *ChatResponse) Text() string {
	if len(r.Choices) == 0 {
		return ""
	}
	return r.Choices[0].Message.Content
}

// Usage normalizes prompt_tokens/completion_tokens into our Usage type.
func (r *ChatResponse) Usage() Usage {
	return Usage{
		InputTokens:  r.UsageRaw.PromptTokens,
		OutputTokens: r.UsageRaw.CompletionTokens,
	}
}

// Create calls POST /v1/chat/completions with retry+backoff for 429/5xx.
func (c *OpenAIClient) Create(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("ai: marshal openai request: %w", err)
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
	return nil, fmt.Errorf("ai: openai create failed after 3 attempts: %w", last)
}

func (c *OpenAIClient) doCreate(ctx context.Context, body []byte) (*ChatResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ai: new openai req: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

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

	var out ChatResponse
	if err := json.Unmarshal(rb, &out); err != nil {
		return nil, fmt.Errorf("ai: decode openai response: %w", err)
	}
	return &out, nil
}

// ProviderError is the common error type for both Anthropic and OpenAI calls.
// Retryable=true marks 429 / 5xx / transport errors; false marks 4xx (bad input).
type ProviderError struct {
	Status    int
	Body      string
	Retryable bool
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("ai provider error %d: %s", e.Status, truncateForLog(e.Body, 200))
}

func isRetryable(err error) bool {
	var p *ProviderError
	if errors.As(err, &p) {
		return p.Retryable
	}
	return false
}

// backoffDelay returns the sleep before attempt N (0-indexed): 1s, 4s, 12s.
func backoffDelay(attempt int) time.Duration {
	switch attempt {
	case 1:
		return 1 * time.Second
	case 2:
		return 4 * time.Second
	default:
		return 12 * time.Second
	}
}

func truncateForLog(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
