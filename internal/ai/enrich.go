package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// EnrichmentJSON is the structured output of EnrichProspect.
type EnrichmentJSON struct {
	Icebreaker         string   `json:"icebreaker"`
	PainPoints         []string `json:"pain_points"`
	CommunicationStyle string   `json:"communication_style"` // formal | semiformal | casual
	ICPFitScore        int      `json:"icp_fit_score"`        // 1..10
	ICPFitReason       string   `json:"icp_fit_reason"`
	CleanPosition      *string  `json:"clean_position"`
	CleanCompany       *string  `json:"clean_company"`
	CleanSchool        *string  `json:"clean_school"`
	CleanIndustry      *string  `json:"clean_industry"`
}

// EnrichmentInput carries the fields needed to build the enrichment prompt.
type EnrichmentInput struct {
	FullName       string
	Headline       string
	Company        string
	Location       string
	Industry       any // can be string or JSON-encodable structure
	WorkExperience any
	Education      any
}

// EnrichResult is the cost-tracked output of Enricher.Enrich.
type EnrichResult struct {
	Enrichment EnrichmentJSON `json:"enrichment"`
	Model      string         `json:"model"`
	Usage      Usage          `json:"usage"`
	CostUSD    float64        `json:"cost_usd"`
}

// Enricher uses OpenAI GPT-4o-mini to extract structured profile data.
type Enricher struct {
	client *OpenAIClient
	model  string
}

// NewEnricher builds an Enricher. model defaults to "gpt-4o-mini".
func NewEnricher(client *OpenAIClient, model string) *Enricher {
	if model == "" {
		model = "gpt-4o-mini"
	}
	return &Enricher{client: client, model: model}
}

// Enrich returns a structured profile analysis. On parse failure, returns a
// neutral fallback enrichment (icebreaker="", communication_style="semiformal",
// score=5) and the underlying parse error.
func (e *Enricher) Enrich(ctx context.Context, in EnrichmentInput) (*EnrichResult, error) {
	prompt := buildEnrichPrompt(in)

	resp, err := e.client.Create(ctx, ChatRequest{
		Model:       e.model,
		MaxTokens:   600,
		Temperature: 0.3,
		Messages:    []ChatMessage{{Role: "user", Content: prompt}},
	})
	if err != nil {
		return nil, err
	}

	parsed, parseErr := parseEnrichJSON(resp.Text())
	usage := resp.Usage()
	out := &EnrichResult{
		Enrichment: parsed,
		Model:      e.model,
		Usage:      usage,
		CostUSD:    CalcCost(e.model, usage),
	}
	return out, parseErr
}

// FallbackEnrichment is what callers should use when Enrich fails or returns
// a parse error: neutral defaults that won't break downstream template
// rendering.
func FallbackEnrichment(reason string) EnrichmentJSON {
	return EnrichmentJSON{
		Icebreaker:         "",
		PainPoints:         []string{},
		CommunicationStyle: "semiformal",
		ICPFitScore:        5,
		ICPFitReason:       reason,
	}
}

func buildEnrichPrompt(in EnrichmentInput) string {
	industry := jsonTrunc(in.Industry, 200)
	workExp := jsonTrunc(in.WorkExperience, 800)
	education := jsonTrunc(in.Education, 400)

	return fmt.Sprintf(`Analiza este perfil de LinkedIn. Responde ÚNICAMENTE en JSON válido con este schema exacto (sin texto adicional):
{
  "icebreaker": "referencia conversacional al perfil — cómo lo mencionaría un humano scrolleando LinkedIn, no un extractor de CV",
  "pain_points": ["dolor 1 probable de su rol", "dolor 2", "dolor 3"],
  "communication_style": "formal | semiformal | casual",
  "icp_fit_score": 7,
  "icp_fit_reason": "por qué ese score en 1 oración",
  "clean_position": "cargo actual en palabras limpias y cortas (ej: 'Director Comercial'). Null si no hay datos.",
  "clean_company": "empresa actual en nombre limpio (ej: 'Acme Corp'). Null si no hay datos.",
  "clean_school": "universidad más relevante completada (ej: 'UNAM' o 'Tec de Monterrey'). Null si no hay datos.",
  "clean_industry": "sector o industria en español (ej: 'Seguros', 'Finanzas', 'Tecnología'). Null si no hay datos."
}

Datos del perfil:
- Nombre: %s
- Cargo/Headline: %s
- Empresa: %s
- Ubicación: %s
- Industria: %s
- Experiencia laboral: %s
- Educación: %s

═══════════════════════════════════════════════════════════════
INSTRUCCIONES PARA "icebreaker" (máxima atención — es lo más importante):
═══════════════════════════════════════════════════════════════

El icebreaker NO es un dato extraído del CV. Es cómo un humano informal lo mencionaría en chat.

Diferencia crítica entre BOT y HUMANO al referenciar el mismo perfil:

❌ BOT (extracto de CV, suena a LinkedIn API):
- "Diseñaste un SDK en Python para integrar plataformas IoT con SmartThings"
- "Llevas 4 años como Director Comercial en Acme Corp"
- "Construiste el equipo de ventas de Acme desde cero"
- "Has trabajado en Cartier, GoGoFix y SmartThings"

✅ HUMANO (referencia conversacional, suena a "te vi scrolleando"):
- "vi que andas metido en el mundo IoT desde hace un rato"
- "noté que llevas un tiempo ya construyendo equipos comerciales"
- "me llamó la atención lo que armaste en Acme"
- "vi que pasaste por varias empresas de tech bastante distintas"

Reglas duras para el icebreaker:
- Empieza con verbos de OBSERVACIÓN ("vi que", "noté que", "me llamó la atención", "me topé con")
  NUNCA con verbos de ACCIÓN del prospect ("diseñaste", "construiste", "lideraste", "lograste")
- Vaguedad calculada: "llevas un tiempo", "varios años", "varias empresas" > números exactos
- Cero jerga de CV: nada de "SDK", "stack", "OKRs", "P&L", "vertical", "go-to-market"
- Si la jerga es obvia (ej. tech), usar palabras del dominio en español casual: "cosas IoT", "mundo del desarrollo", "tema de automatización"
- Máx 15 palabras. En español mexicano con tuteo (NUNCA "vos", "tenés"). Primera persona informal.
- Si no hay nada genuinamente interesante o personal → devuelve "" (string vacío). No inventes.

Instrucciones para los otros campos:
- "clean_position": extrae el cargo actual del work_experience o headline. Sin seniority inflado.
- "clean_company": empresa donde trabaja actualmente según work_experience[current=true] o la más reciente.
- "clean_school": grado completado más relevante (carrera > posgrado corto). Si solo hay cursos o diplomados, null.
- "clean_industry": infiere de la experiencia + industria. En español, máx 2 palabras.
- NO inventes cargos, empresas o universidades que no aparezcan en el perfil.
- Si faltan datos para un campo clean_*, devuelve null (no string vacío).
- Responde SOLO el JSON, sin markdown, sin explicaciones.`,
		stringOr(in.FullName, "N/A"),
		stringOr(in.Headline, "N/A"),
		stringOr(in.Company, "N/A"),
		stringOr(in.Location, "N/A"),
		industry,
		workExp,
		education,
	)
}

// parseEnrichJSON extracts the JSON object from the LLM response. Returns the
// parsed enrichment plus the parse error (nil on success). On failure returns
// a neutral fallback enrichment so callers can still proceed.
func parseEnrichJSON(text string) (EnrichmentJSON, error) {
	trimmed := strings.TrimSpace(text)
	start := strings.IndexByte(trimmed, '{')
	end := strings.LastIndexByte(trimmed, '}')
	if start == -1 || end == -1 || end < start {
		return FallbackEnrichment("Sin JSON parseable en respuesta IA"), fmt.Errorf("ai: no JSON object in enrich response")
	}
	var out EnrichmentJSON
	if err := json.Unmarshal([]byte(trimmed[start:end+1]), &out); err != nil {
		return FallbackEnrichment("Error al parsear respuesta IA"), err
	}
	if out.CommunicationStyle == "" {
		out.CommunicationStyle = "semiformal"
	}
	if out.PainPoints == nil {
		out.PainPoints = []string{}
	}
	return out, nil
}

// jsonTrunc returns a truncated JSON string representation. If v is already a
// string, returns it (truncated). Used to fit large work_experience arrays
// into the prompt without blowing token budgets.
func jsonTrunc(v any, maxLen int) string {
	if v == nil {
		return "N/A"
	}
	if s, ok := v.(string); ok {
		if len(s) > maxLen {
			return s[:maxLen]
		}
		return s
	}
	j, err := json.Marshal(v)
	if err != nil {
		return "N/A"
	}
	if len(j) > maxLen {
		return string(j[:maxLen])
	}
	return string(j)
}
