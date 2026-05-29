package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// PostClassifyInput is a feed post plus the job-seeker context needed to judge
// relevance. CVSummary comes from the account's client_profile.
type PostClassifyInput struct {
	CVSummary      string
	AuthorName     string
	AuthorHeadline string
	Text           string
}

// PostClassification is the classifier output.
type PostClassification struct {
	Relevant  bool     `json:"relevant"`
	Score     int      `json:"score"` // 0-100 relevance to the job search
	Reasoning string   `json:"reasoning"`
	Role      string   `json:"role"`    // role being hired for, if any
	Company   string   `json:"company"` // hiring company, if mentioned
	Tags      []string `json:"tags"`
	Model     string   `json:"model"`
	Usage     Usage    `json:"usage"`
	CostUSD   float64  `json:"cost_usd"`
}

// PostClassifier judges whether a LinkedIn post is a hiring/opportunity post
// relevant to the job-seeker, over the OpenCode-backed Anthropic client.
type PostClassifier struct {
	client *AnthropicClient
	model  string
}

// NewPostClassifier builds a PostClassifier. Empty model falls back to smart.
func NewPostClassifier(client *AnthropicClient, model string) *PostClassifier {
	if model == "" {
		model = "claude-sonnet-4-6"
	}
	return &PostClassifier{client: client, model: model}
}

const postClassifySystem = `Clasificas publicaciones de LinkedIn para alguien que busca trabajo. Decides si un post es una OPORTUNIDAD relevante (alguien contratando, una vacante, "we're hiring", "estamos buscando", un recruiter activo) que encaje con el perfil del candidato. Respondes SOLO en JSON válido con este schema exacto:
{
  "relevant": true|false,
  "score": 0-100,
  "reasoning": "1-2 oraciones en español",
  "role": "rol que se busca, o null",
  "company": "empresa que contrata, o null",
  "tags": ["etiquetas cortas, ej: hiring, remoto, recruiter, AI, no-relevante"]
}

Criterios:
- relevant=true SOLO si el post ofrece o señala una oportunidad laboral que el candidato podría perseguir (vacante, contratación, recruiter buscando, founder armando equipo).
- score = qué tan bien encaja la oportunidad con el perfil del candidato (stack, modalidad remota, seniority alcanzable). Si no es una oportunidad, score < 20.
- Posts que son solo opinión, noticias, autopromoción, "busco trabajo" de otra persona, o felicitaciones => relevant=false.
- Extraé role y company si el post los menciona; si no, null.
- Sé honesto y conciso. No inventes datos que no estén en el post.`

// ClassifyPost judges one post. On LLM/parse failure returns an error.
func (c *PostClassifier) ClassifyPost(ctx context.Context, in PostClassifyInput) (*PostClassification, error) {
	user := buildPostClassifyUserMsg(in)

	req := MessagesRequest{
		Model:     c.model,
		MaxTokens: 3000, // reasoning models (kimi-k2.6) spend hidden tokens before the JSON
		System:    []SystemBlock{(SystemBlock{Type: "text", Text: postClassifySystem}).WithCache()},
		Messages:  []Message{{Role: "user", Content: user}},
	}

	resp, err := c.client.Create(ctx, req)
	if err != nil {
		return nil, err
	}

	raw := resp.Text()
	parsed, err := parsePostClassifyJSON(raw)
	if err != nil {
		snippet := raw
		if len(snippet) > 300 {
			snippet = snippet[:300]
		}
		return nil, fmt.Errorf("ai: post classify parse (%w): raw=%q", err, snippet)
	}

	return &PostClassification{
		Relevant:  parsed.Relevant,
		Score:     min(100, max(0, parsed.Score)),
		Reasoning: strings.TrimSpace(parsed.Reasoning),
		Role:      strings.TrimSpace(parsed.Role),
		Company:   strings.TrimSpace(parsed.Company),
		Tags:      parsed.Tags,
		Model:     c.model,
		Usage:     resp.Usage,
		CostUSD:   CalcCost(c.model, resp.Usage),
	}, nil
}

func buildPostClassifyUserMsg(in PostClassifyInput) string {
	text := in.Text
	if len(text) > 4000 {
		text = text[:4000]
	}
	return fmt.Sprintf(`PERFIL DEL CANDIDATO:
%s

POST A CLASIFICAR:
- Autor: %s
- Headline del autor: %s
- Texto:
%s

Decidí si es una oportunidad relevante y respondé solo con el JSON.`,
		stringOr(in.CVSummary, "(sin resumen de CV)"),
		stringOr(in.AuthorName, "N/A"),
		stringOr(in.AuthorHeadline, "N/A"),
		stringOr(text, "(post vacío)"),
	)
}

type postClassifyJSON struct {
	Relevant  bool     `json:"relevant"`
	Score     int      `json:"score"`
	Reasoning string   `json:"reasoning"`
	Role      string   `json:"role"`
	Company   string   `json:"company"`
	Tags      []string `json:"tags"`
}

func parsePostClassifyJSON(text string) (postClassifyJSON, error) {
	var out postClassifyJSON
	trimmed := strings.TrimSpace(text)
	start := strings.IndexByte(trimmed, '{')
	end := strings.LastIndexByte(trimmed, '}')
	if start == -1 || end == -1 || end < start {
		return out, fmt.Errorf("no JSON object in response")
	}
	if err := json.Unmarshal([]byte(trimmed[start:end+1]), &out); err != nil {
		return out, err
	}
	return out, nil
}
