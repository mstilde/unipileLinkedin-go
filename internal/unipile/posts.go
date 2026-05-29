package unipile

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"
)

// ---- LinkedIn content (posts) search — front 4: post-feed scanner ----
//
// Endpoint (classic API): POST /api/v1/linkedin/search?account_id=X with body
// {api:"classic", category:"posts", keywords:"..."}. Returns recent posts
// matching the keywords; each carries the author and the post text. Schemas are
// decoded tolerantly across API versions (see jobs.go for the same approach).

// SearchPostsParams configures a content (posts) search.
type SearchPostsParams struct {
	AccountID string
	Keywords  string
	SearchURL string // optional full LinkedIn content-search URL (url-mode)
	Limit     int    // soft cap (0 → 20)
}

// PostHit is one post result with its author.
type PostHit struct {
	ID               string          `json:"id"`
	Text             string          `json:"text"`
	URL              string          `json:"url"`
	AuthorName       string          `json:"author_name"`
	AuthorHeadline   string          `json:"author_headline"`
	AuthorProviderID string          `json:"author_provider_id"`
	AuthorProfileURL string          `json:"author_profile_url"`
	ReactionsCount   int             `json:"reactions_count"`
	CommentsCount    int             `json:"comments_count"`
	PostedAt         *time.Time      `json:"posted_at,omitempty"`
	Raw              json.RawMessage `json:"-"`
}

// SearchPostsResult is the decoded posts-search response.
type SearchPostsResult struct {
	Items  []PostHit `json:"items"`
	Cursor string    `json:"cursor"`
	DryRun bool      `json:"dry_run,omitempty"`
}

// SearchPosts runs a LinkedIn content (posts) search. In dry-run it returns one
// synthetic hiring post so the scheduler loop is exercisable offline.
func (c *Client) SearchPosts(ctx context.Context, p SearchPostsParams) (*SearchPostsResult, error) {
	if p.AccountID == "" {
		return nil, errors.New("unipile: search_posts: account_id required")
	}
	if p.Keywords == "" && p.SearchURL == "" {
		return nil, errors.New("unipile: search_posts: keywords or search_url required")
	}
	if c.dryRun {
		stamp := time.Now().Format("20060102T150405")
		return &SearchPostsResult{DryRun: true, Items: []PostHit{{
			ID:               "dry-post-" + stamp,
			Text:             "We're hiring! Looking for a remote AI Automation Engineer (TypeScript, n8n, LLMs). DM me or comment. #hiring",
			URL:              "https://www.linkedin.com/feed/update/dry-" + stamp,
			AuthorName:       "Jordan Recruiter",
			AuthorHeadline:   "Talent Partner @ Acme Remote",
			AuthorProviderID: "dry-author-" + stamp,
			AuthorProfileURL: "https://www.linkedin.com/in/dry-" + stamp,
			ReactionsCount:   12,
			CommentsCount:    4,
		}}}, nil
	}

	body := map[string]any{
		"api":      "classic",
		"category": "posts",
	}
	if p.SearchURL != "" {
		body["url"] = p.SearchURL
	} else {
		body["keywords"] = p.Keywords
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
	out := &SearchPostsResult{Cursor: envelope.Cursor}
	for _, raw := range envelope.Items {
		hit := parsePostHit(raw)
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

// parsePostHit decodes a posts-search item tolerantly across schema variants.
func parsePostHit(raw json.RawMessage) PostHit {
	var r struct {
		ID        string          `json:"id"`
		PostID    string          `json:"post_id"`
		ShareURN  string          `json:"share_urn"`
		Text      string          `json:"text"`
		Body      string          `json:"body"`
		Commentary string         `json:"commentary"`
		URL       string          `json:"url"`
		ShareURL  string          `json:"share_url"`
		Reactions json.Number     `json:"reaction_count"`
		Reactions2 json.Number    `json:"reactions_count"`
		Comments  json.Number     `json:"comment_count"`
		Comments2 json.Number     `json:"comments_count"`
		PostedAt  string          `json:"posted_at"`
		Date      string          `json:"date"`
		Author    json.RawMessage `json:"author"`
		Poster    json.RawMessage `json:"poster"`
	}
	_ = json.Unmarshal(raw, &r)

	name, headline, providerID, profileURL := parsePostAuthor(firstNonEmptyRaw(r.Author, r.Poster))

	return PostHit{
		ID:               firstNonEmpty(r.ID, r.PostID, r.ShareURN),
		Text:             firstNonEmpty(r.Text, r.Commentary, r.Body),
		URL:              firstNonEmpty(r.URL, r.ShareURL),
		AuthorName:       name,
		AuthorHeadline:   headline,
		AuthorProviderID: providerID,
		AuthorProfileURL: profileURL,
		ReactionsCount:   parseCount(firstNonEmpty(r.Reactions.String(), r.Reactions2.String())),
		CommentsCount:    parseCount(firstNonEmpty(r.Comments.String(), r.Comments2.String())),
		PostedAt:         parseTime(firstNonEmpty(r.PostedAt, r.Date)),
		Raw:              raw,
	}
}

// parsePostAuthor extracts author fields from a nested author/poster object.
func parsePostAuthor(raw json.RawMessage) (name, headline, providerID, profileURL string) {
	if len(raw) == 0 {
		return "", "", "", ""
	}
	var a struct {
		Name        string `json:"name"`
		FullName    string `json:"full_name"`
		Headline    string `json:"headline"`
		Occupation  string `json:"occupation"`
		ProviderID  string `json:"provider_id"`
		ID          string `json:"id"`
		PublicID    string `json:"public_identifier"`
		ProfileURL  string `json:"profile_url"`
		PublicURL   string `json:"public_profile_url"`
	}
	_ = json.Unmarshal(raw, &a)
	name = firstNonEmpty(a.Name, a.FullName)
	headline = firstNonEmpty(a.Headline, a.Occupation)
	providerID = firstNonEmpty(a.ProviderID, a.ID)
	profileURL = firstNonEmpty(a.ProfileURL, a.PublicURL)
	if profileURL == "" && a.PublicID != "" {
		profileURL = "https://www.linkedin.com/in/" + a.PublicID
	}
	return name, headline, providerID, profileURL
}

func firstNonEmptyRaw(vals ...json.RawMessage) json.RawMessage {
	for _, v := range vals {
		if len(strings.TrimSpace(string(v))) > 0 && string(v) != "null" {
			return v
		}
	}
	return nil
}
