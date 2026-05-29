package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// JobRankInput is everything the ranker needs to score one posting against the
// job-seeker's profile. CVSummary + Preferences come from client_profiles
// (merged_summary) and the job-search profile; the rest is the posting.
type JobRankInput struct {
	CVSummary   string
	Preferences string
	Title       string
	Company     string
	Location    string
	Description string
}

// JobRankResult is the scored output. Score is 0-100 (higher = better fit).
type JobRankResult struct {
	Score     int      `json:"score"`
	Reasoning string   `json:"reasoning"`
	Tags      []string `json:"tags"`
	Model     string   `json:"model"`
	Usage     Usage    `json:"usage"`
	CostUSD   float64  `json:"cost_usd"`
}

// JobRanker scores job postings for fit. It runs over the same AnthropicClient
// the rest of the pipeline uses, which in Buscalaburos points at OpenCode Go
// (Anthropic-compatible endpoint) with a model like kimi-k2.6.
type JobRanker struct {
	client *AnthropicClient
	model  string
}

// NewJobRanker builds a JobRanker. An empty model falls back to the smart model.
func NewJobRanker(client *AnthropicClient, model string) *JobRanker {
	if model == "" {
		model = "claude-sonnet-4-6"
	}
	return &JobRanker{client: client, model: model}
}

const jobRankSystem = `Eres un asesor de carrera que evalúa qué tan bien encaja una oferta de empleo con un candidato concreto. Respondes SOLO en JSON válido con este schema exacto:
{
  "score": 0-100,
  "reasoning": "1-2 oraciones en español explicando el puntaje",
  "tags": ["etiquetas cortas, ej: remoto, AI, Next.js, red flag: on-call, sueldo bajo"]
}

Criterios de puntaje (0-100, mayor = mejor encaje):
- Coincidencia de stack y rol con el perfil del candidato (lo más importante)
- Modalidad remota o híbrida (presencial = penalizar fuerte)
- Idioma del rol compatible (español o inglés intermedio)
- Seniority alcanzable (no roles claramente Senior/Staff salvo que sea AI puro)
- Sin red flags graves (sueldo muy bajo, on-call agresivo, "fast-paced + grit" sin detalle)

Reglas:
- Si la oferta es presencial obligatorio o pide relocación: score < 30.
- Si pide un idioma que el candidato no maneja (ej: alemán, portugués nativo): score < 25.
- Si encaja stack + remoto + rol objetivo: score > 75.
- Sé honesto y conciso. No inventes datos que no estén en la descripción.
- En "tags" incluí siempre la modalidad detectada (remoto/híbrido/presencial) y cualquier red flag.`

// RankJob scores one posting. On LLM/parse failure it returns an error; the
// caller decides whether to park the posting.
func (r *JobRanker) RankJob(ctx context.Context, in JobRankInput) (*JobRankResult, error) {
	user := buildJobRankUserMsg(in)

	req := MessagesRequest{
		Model:     r.model,
		MaxTokens: 3000, // reasoning models (kimi-k2.6) spend hidden tokens before the JSON; leave room
		System:    []SystemBlock{(SystemBlock{Type: "text", Text: jobRankSystem}).WithCache()},
		Messages:  []Message{{Role: "user", Content: user}},
	}

	resp, err := r.client.Create(ctx, req)
	if err != nil {
		return nil, err
	}

	raw := resp.Text()
	parsed, err := parseJobRankJSON(raw)
	if err != nil {
		snippet := raw
		if len(snippet) > 300 {
			snippet = snippet[:300]
		}
		return nil, fmt.Errorf("ai: job rank parse (%w): raw=%q", err, snippet)
	}

	score := min(100, max(0, parsed.Score))

	return &JobRankResult{
		Score:     score,
		Reasoning: strings.TrimSpace(parsed.Reasoning),
		Tags:      parsed.Tags,
		Model:     r.model,
		Usage:     resp.Usage,
		CostUSD:   CalcCost(r.model, resp.Usage),
	}, nil
}

func buildJobRankUserMsg(in JobRankInput) string {
	desc := in.Description
	if len(desc) > 6000 {
		desc = desc[:6000] // keep token cost bounded; JDs past 6k chars are boilerplate
	}
	return fmt.Sprintf(`PERFIL DEL CANDIDATO:
%s

PREFERENCIAS DE BÚSQUEDA:
%s

OFERTA A EVALUAR:
- Título: %s
- Empresa: %s
- Ubicación: %s
- Descripción:
%s

Evalúa el encaje y responde solo con el JSON.`,
		stringOr(in.CVSummary, "(sin resumen de CV)"),
		stringOr(in.Preferences, "(sin preferencias)"),
		stringOr(in.Title, "N/A"),
		stringOr(in.Company, "N/A"),
		stringOr(in.Location, "N/A"),
		stringOr(desc, "(sin descripción)"),
	)
}

type jobRankJSON struct {
	Score     int      `json:"score"`
	Reasoning string   `json:"reasoning"`
	Tags      []string `json:"tags"`
}

// parseJobRankJSON extracts the JSON object even when the model wraps it in prose.
func parseJobRankJSON(text string) (jobRankJSON, error) {
	var out jobRankJSON
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
