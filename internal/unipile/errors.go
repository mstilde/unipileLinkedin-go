// Package unipile is a hand-rolled Go client for the Unipile API.
//
// Why not the official SDK? There isn't one for Go (only Node + TS as of 2026).
// This package wraps net/http with the same error-classification semantics as
// unipile/errors.js + unipile/invite.js from the Node original.
package unipile

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// APIError is the parsed view of a non-2xx Unipile response. Use the Is* helpers
// to classify and decide retry / backoff strategy.
type APIError struct {
	Status int                    // HTTP status code (0 if network error)
	Type   string                 // Unipile error type (lowercased), or our inferred classification
	Title  string                 // human title
	Detail string                 // human detail
	Raw    map[string]any         // raw decoded body, for logging
}

// Error implements error.
func (e *APIError) Error() string {
	if e.Type != "" {
		return fmt.Sprintf("unipile %d: %s (%s)", e.Status, e.Type, e.Title)
	}
	return fmt.Sprintf("unipile %d: %s", e.Status, e.Title)
}

// parseAPIError decodes the response body of a Unipile error. Mirrors the
// inference rules of parseUnipileError in errors.js (premium_required,
// inmail_credit_exhausted, account_disconnected, invalid_provider_id).
func parseAPIError(status int, body []byte) *APIError {
	e := &APIError{Status: status}
	if len(body) > 0 {
		_ = json.Unmarshal(body, &e.Raw)
	}
	if e.Raw != nil {
		if v, ok := e.Raw["status"].(float64); ok && e.Status == 0 {
			e.Status = int(v)
		}
		if v, ok := e.Raw["type"].(string); ok {
			e.Type = strings.ToLower(v)
		}
		if v, ok := e.Raw["title"].(string); ok {
			e.Title = v
		}
		if e.Title == "" {
			if v, ok := e.Raw["message"].(string); ok {
				e.Title = v
			}
		}
		if v, ok := e.Raw["detail"].(string); ok {
			e.Detail = v
		} else if v, ok := e.Raw["error"].(string); ok {
			e.Detail = v
		}
	}
	// If Unipile didn't tell us the type, infer from blob.
	if e.Type == "" {
		blob := strings.ToLower(e.Type + " " + e.Title + " " + e.Detail)
		switch {
		case rePremium.MatchString(blob):
			e.Type = "premium_required"
		case reInMailCredit.MatchString(blob):
			e.Type = "inmail_credit_exhausted"
		case reAccountDisc.MatchString(blob):
			e.Type = "account_disconnected"
		case reInvalidPID.MatchString(blob):
			e.Type = "invalid_provider_id"
		}
	}
	return e
}

var (
	rePremium     = regexp.MustCompile(`(premium|sales_navigator|recruiter).{0,30}(required|inmail|not_premium)`)
	reInMailCredit = regexp.MustCompile(`inmail.{0,20}credit|no.{0,5}credit|credit.{0,10}exhaust`)
	reAccountDisc = regexp.MustCompile(`account.{0,5}disconnect|credentials.{0,5}expired|reauth|unauthorized`)
	reInvalidPID  = regexp.MustCompile(`invalid.{0,5}provider|provider.{0,5}not.{0,5}found`)
	rePermBlob    = regexp.MustCompile(`private_profile|profile_not_found|user_not_found|cannot_send_message|account_restricted|blocked|disconnected|credentials.expired|expired`)
	reRateBlob    = regexp.MustCompile(`cannot_resend_yet|invitation_limit|weekly_limit|daily_limit_reached|rate.?limit`)
	reWeeklyBlob  = regexp.MustCompile(`cannot_resend_yet|weekly|weekly_invitation_limit`)
)

// IsLinkedInRateLimit reports whether the error is a LinkedIn-side rate limit
// (not Unipile's). These should usually be retried after a day; weekly caps
// should be retried after the next Monday.
func (e *APIError) IsLinkedInRateLimit() bool {
	if e == nil {
		return false
	}
	if e.Status == 422 {
		return true
	}
	blob := strings.ToLower(e.Type + " " + e.Title + " " + e.Detail)
	return reRateBlob.MatchString(blob)
}

// IsWeeklyCap returns true when the rate limit is specifically the weekly
// invitation cap (next retry should be next Monday morning).
func (e *APIError) IsWeeklyCap() bool {
	if e == nil {
		return false
	}
	blob := strings.ToLower(e.Type + " " + e.Title + " " + e.Detail)
	return reWeeklyBlob.MatchString(blob)
}

// IsThrottled reports a 429 from Unipile's own throttle (not LinkedIn).
// Retry with exponential backoff in the "throttle" curve.
func (e *APIError) IsThrottled() bool {
	return e != nil && e.Status == 429
}

// IsTransient reports a 5xx — Unipile-side or LinkedIn-side hiccup. Retry.
func (e *APIError) IsTransient() bool {
	return e != nil && e.Status >= 500 && e.Status < 600
}

// IsPermanent reports a non-retryable error: 403/404/451, or one of the
// classified types (premium_required, account_disconnected, etc.).
func (e *APIError) IsPermanent() bool {
	if e == nil {
		return false
	}
	if e.Status == 403 || e.Status == 404 || e.Status == 451 {
		return true
	}
	switch e.Type {
	case "premium_required", "inmail_credit_exhausted", "account_disconnected", "invalid_provider_id":
		return true
	}
	blob := strings.ToLower(e.Type + " " + e.Title + " " + e.Detail)
	return rePermBlob.MatchString(blob)
}

// AsAPIError extracts *APIError from err if present. Otherwise returns nil, false.
func AsAPIError(err error) (*APIError, bool) {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr, true
	}
	return nil, false
}
