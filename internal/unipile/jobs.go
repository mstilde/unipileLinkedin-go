package unipile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ---- LinkedIn Jobs search (front 2B: Buscalaburos 3000 postings ranker) ----
//
// Endpoints (classic API, work WITHOUT LinkedIn Premium):
//   POST /api/v1/linkedin/search?account_id=X   body {api:"classic", category:"jobs", ...}
//   GET  /api/v1/linkedin/jobs/{id}?account_id=X
//
// The response schemas below are decoded tolerantly (field names vary across
// Unipile API versions): each list item and the detail body keep their raw
// JSON so the caller can store it verbatim and we degrade gracefully if a field
// moves. Verify against a live response when wiring real (non-dry-run) traffic.

// SearchJobsParams configures a jobs search. Either Keywords or SearchURL must
// be set; SearchURL (a full LinkedIn jobs-search URL) takes precedence and lets
// us pin filters the keyword API doesn't expose.
type SearchJobsParams struct {
	AccountID string
	Keywords  string
	SearchURL string // optional full LinkedIn search URL (url-mode)
	GeoID     string // worldwide = "92000000"; empty lets LinkedIn apply the account geo
	Limit     int    // soft cap on hits returned (0 → 20)
}

// JobHit is one organic result of a jobs search. Promoted hits (sponsored ads)
// are flagged so the caller can skip them.
type JobHit struct {
	ID       string          `json:"id"`
	Title    string          `json:"title"`
	Company  string          `json:"company"`
	Location string          `json:"location"`
	URL      string          `json:"url"`
	Promoted bool            `json:"promoted"`
	Raw      json.RawMessage `json:"-"`
}

// SearchJobsResult is the decoded jobs-search response.
type SearchJobsResult struct {
	Items  []JobHit `json:"items"`
	Cursor string   `json:"cursor"`
	DryRun bool     `json:"dry_run,omitempty"`
}

// SearchJobs runs a LinkedIn jobs search. In dry-run it returns one synthetic
// hit so the scheduler loop is exercisable without live credentials.
func (c *Client) SearchJobs(ctx context.Context, p SearchJobsParams) (*SearchJobsResult, error) {
	if p.AccountID == "" {
		return nil, errors.New("unipile: search_jobs: account_id required")
	}
	if p.Keywords == "" && p.SearchURL == "" {
		return nil, errors.New("unipile: search_jobs: keywords or search_url required")
	}
	if c.dryRun {
		stamp := time.Now().Format("20060102T150405")
		return &SearchJobsResult{DryRun: true, Items: []JobHit{{
			ID:       "dry-job-" + stamp,
			Title:    "AI Automation Engineer (dry-run)",
			Company:  "Acme Remote",
			Location: "Remote - Worldwide",
			URL:      "https://www.linkedin.com/jobs/view/dry-" + stamp,
		}}}, nil
	}

	body := map[string]any{
		"api":      "classic",
		"category": "jobs",
	}
	if p.SearchURL != "" {
		body["url"] = p.SearchURL
	} else {
		body["keywords"] = p.Keywords
		if p.GeoID != "" {
			body["geo_id"] = p.GeoID
		}
	}

	path := "/api/v1/linkedin/search?account_id=" + url.QueryEscape(p.AccountID)

	var envelope struct {
		Items  []json.RawMessage `json:"items"`
		Cursor string            `json:"cursor"`
	}
	if err := c.do(ctx, "POST", path, body, &envelope); err != nil {
		return nil, err
	}

	limit := p.Limit
	if limit <= 0 {
		limit = 20
	}
	out := &SearchJobsResult{Cursor: envelope.Cursor}
	for _, raw := range envelope.Items {
		hit := parseJobHit(raw)
		if hit.ID == "" {
			continue
		}
		out.Items = append(out.Items, hit)
		if len(out.Items) >= limit {
			break
		}
	}
	return out, nil
}

// JobDetail is the full job description fetched by GetJob.
type JobDetail struct {
	ID              string          `json:"id"`
	Title           string          `json:"title"`
	Company         string          `json:"company"`
	Location        string          `json:"location"`
	URL             string          `json:"url"`
	Description     string          `json:"description"`
	ApplicantsCount int             `json:"applicants_count"`
	PostedAt        *time.Time      `json:"posted_at,omitempty"`
	Raw             json.RawMessage `json:"-"`
	DryRun          bool            `json:"dry_run,omitempty"`
}

// GetJob fetches the full description of a job by its LinkedIn id.
func (c *Client) GetJob(ctx context.Context, accountID, jobID string) (*JobDetail, error) {
	if accountID == "" {
		return nil, errors.New("unipile: get_job: account_id required")
	}
	if jobID == "" {
		return nil, errors.New("unipile: get_job: job_id required")
	}
	if c.dryRun {
		return &JobDetail{
			DryRun:          true,
			ID:              jobID,
			Title:           "AI Automation Engineer (dry-run)",
			Company:         "Acme Remote",
			Location:        "Remote - Worldwide",
			Description:     "We are hiring a remote AI automation engineer. Stack: TypeScript, Next.js, n8n, LLM integrations. English B2 ok. USD 2k-4k/mo.",
			ApplicantsCount: 12,
		}, nil
	}

	path := "/api/v1/linkedin/jobs/" + url.PathEscape(jobID) + "?account_id=" + url.QueryEscape(accountID)
	var raw json.RawMessage
	if err := c.do(ctx, "GET", path, nil, &raw); err != nil {
		return nil, err
	}
	return parseJobDetail(raw)
}

// parseJobHit decodes a search-result item tolerantly across schema variants.
func parseJobHit(raw json.RawMessage) JobHit {
	var r struct {
		ID          string          `json:"id"`
		JobID       string          `json:"job_id"`
		Title       string          `json:"title"`
		Location    string          `json:"location"`
		URL         string          `json:"url"`
		Promoted    bool            `json:"promoted"`
		Sponsored   bool            `json:"sponsored"`
		Company     json.RawMessage `json:"company"`
		CompanyName string          `json:"company_name"`
	}
	_ = json.Unmarshal(raw, &r)

	hit := JobHit{
		ID:       firstNonEmpty(r.ID, r.JobID),
		Title:    r.Title,
		Location: r.Location,
		URL:      r.URL,
		Promoted: r.Promoted || r.Sponsored,
		Company:  firstNonEmpty(r.CompanyName, companyName(r.Company)),
		Raw:      raw,
	}
	return hit
}

// parseJobDetail decodes a job-detail body tolerantly.
func parseJobDetail(raw json.RawMessage) (*JobDetail, error) {
	var r struct {
		ID              string          `json:"id"`
		JobID           string          `json:"job_id"`
		Title           string          `json:"title"`
		Location        string          `json:"location"`
		URL             string          `json:"url"`
		Description     string          `json:"description"`
		Body            string          `json:"body"`
		Company         json.RawMessage `json:"company"`
		CompanyName     string          `json:"company_name"`
		ApplicantsCount json.Number     `json:"applicants_count"`
		Applicants      json.Number     `json:"applicants"`
		PostedAt        string          `json:"posted_at"`
		ListedAt        string          `json:"listed_at"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("unipile: decode job detail: %w", err)
	}

	d := &JobDetail{
		ID:              firstNonEmpty(r.ID, r.JobID),
		Title:           r.Title,
		Location:        r.Location,
		URL:             r.URL,
		Description:     firstNonEmpty(r.Description, r.Body),
		Company:         firstNonEmpty(r.CompanyName, companyName(r.Company)),
		ApplicantsCount: parseCount(firstNonEmpty(r.ApplicantsCount.String(), r.Applicants.String())),
		Raw:             raw,
	}
	if t := parseTime(firstNonEmpty(r.PostedAt, r.ListedAt)); t != nil {
		d.PostedAt = t
	}
	return d, nil
}

// companyName extracts a name from a company field that may be a bare string or
// an object like {"name": "..."}.
func companyName(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var obj struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(raw, &obj) == nil {
		return obj.Name
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func parseCount(s string) int {
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}

func parseTime(s string) *time.Time {
	if s == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05Z07:00", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return &t
		}
	}
	// Some APIs return epoch millis as a string.
	if ms, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err == nil && ms > 0 {
		t := time.UnixMilli(ms)
		return &t
	}
	return nil
}
