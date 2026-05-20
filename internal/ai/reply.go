package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// Persona is the asesor's voice/style configuration for AI replies.
type Persona struct {
	SystemPrompt string
	GuionesJSON  map[string]string // {"pide_precio": "responde con el rango X-Y", ...}
}

// Prospect carries the subset of prospect fields relevant for reply generation.
type Prospect struct {
	FirstName      string
	Company        string
	Headline       string
	EnrichmentJSON map[string]any
	ContactInfo    map[string]any
}

// ReplyResult is the output of GenerateReply.
type ReplyResult struct {
	Text       string  `json:"text"`
	Model      string  `json:"model"`
	Usage      Usage   `json:"usage"`
	CostUSD    float64 `json:"cost_usd"`
	PIIBlocked bool    `json:"pii_blocked"`
	PIILeaks   []string `json:"pii_leaks,omitempty"`
}

// ReplyGenerator builds replies via Anthropic with Haiku-first / Sonnet-fallback.
type ReplyGenerator struct {
	client     *AnthropicClient
	fastModel  string
	smartModel string
}

// NewReplyGenerator builds a ReplyGenerator.
func NewReplyGenerator(client *AnthropicClient, fastModel, smartModel string) *ReplyGenerator {
	if fastModel == "" {
		fastModel = "claude-haiku-4-5-20251001"
	}
	if smartModel == "" {
		smartModel = "claude-sonnet-4-6"
	}
	return &ReplyGenerator{client: client, fastModel: fastModel, smartModel: smartModel}
}

// GenerateReply produces an automatic reply. Tries the fast model first; if it
// fails after retries, escalates to the smart model. Validates the output for
// PII not present in the authorized context — if a phone/email/URL was
// invented, PIIBlocked=true and the caller must escalate to a human (do NOT
// auto-send).
func (g *ReplyGenerator) GenerateReply(ctx context.Context, persona Persona, prospect Prospect, history []HistoryMessage, classification Intent, incomingText string) (*ReplyResult, error) {
	system := buildReplySystemPrompt(persona)
	user := buildReplyUserContent(prospect, history, classification, incomingText)

	req := MessagesRequest{
		Model:     g.fastModel,
		MaxTokens: 180,
		System:    []SystemBlock{(SystemBlock{Type: "text", Text: system}).WithCache()},
		Messages:  []Message{{Role: "user", Content: user}},
	}

	used := g.fastModel
	resp, err := g.client.Create(ctx, req)
	if err != nil {
		req.Model = g.smartModel
		used = g.smartModel
		resp, err = g.client.Create(ctx, req)
		if err != nil {
			return nil, err
		}
	}

	text := strings.TrimSpace(resp.Text())

	// PII validation: anything the model emitted that isn't in the authorized
	// context (system prompt, guiones, enrichment, contact_info, history) is a leak.
	authBlob := buildAuthorizedContextBlob(persona, prospect, history, incomingText)
	leaks := piiLeaks(text, authBlob)

	return &ReplyResult{
		Text:       text,
		Model:      used,
		Usage:      resp.Usage,
		CostUSD:    CalcCost(used, resp.Usage),
		PIIBlocked: len(leaks) > 0,
		PIILeaks:   leaks,
	}, nil
}

// buildReplySystemPrompt mirrors buildSystemPrompt in reply.js. Style rules are
// load-bearing — do not paraphrase.
func buildReplySystemPrompt(p Persona) string {
	var guiones strings.Builder
	for situacion, guion := range p.GuionesJSON {
		fmt.Fprintf(&guiones, "- Si el prospect %s: %s\n", situacion, guion)
	}
	guionesText := strings.TrimRight(guiones.String(), "\n")
	if guionesText == "" {
		guionesText = "(sin guiones configurados — responde de forma profesional y natural)"
	}

	sp := p.SystemPrompt
	if sp == "" {
		sp = "Eres un representante comercial profesional."
	}

	return sp + `

GUIONES DE RESPUESTA:
` + guionesText + `

ESTILO DE ESCRITURA — MÁXIMA PRIORIDAD, anula cualquier otra regla de formato:
• NUNCA termines una oración con punto final (.) — ninguna línea termina con punto
• NUNCA uses ¿ — las preguntas solo llevan ? al final: "coordinamos?" no "¿coordinamos?"
• NUNCA uses — ni – (em dash / en dash). Para pausas: coma o salto de línea
• NUNCA abras con "Te lo agradezco", "Te entiendo" ni "Entiendo que..." — suenan a chatbot
• Espeja el saludo del prospect: si dice "hola", responde "hola". Si dice "mucho gusto", responde "el gusto es mío"
• Vocabulario de Didier cuando encaja: "me late", "me latió", "qué onda", "te late", "jaja"
• Emojis válidos: 🫡 🙌🏼 🤓 🥲 😅 ✌🏼 — solo si el prospect usó emojis primero

REGLAS DE CONTENIDO:
- Máximo 2–3 oraciones. Las respuestas largas en LinkedIn son ignoradas.
- Tono conversacional y directo. Sin presentaciones ("Hola, soy...").
- Sin negritas ni formato markdown. Solo texto plano.
- Sin promesas que no estén en los guiones.
- NO inventes números de teléfono, emails o URLs que no estén en los guiones.
- Si el prospect pide una llamada, dar disponibilidad concreta o enlace de calendario si lo hay.
- Responder en el mismo idioma que el prospect (si escribe en inglés, responder en inglés).`
}

func buildReplyUserContent(p Prospect, history []HistoryMessage, classification Intent, incomingText string) string {
	historyFormatted := formatHistory(history, 8)
	if historyFormatted == "" {
		historyFormatted = "(primera respuesta)"
	}

	enrichmentJSON, _ := json.Marshal(p.EnrichmentJSON)

	return fmt.Sprintf(`Datos del prospect:
- Nombre: %s
- Empresa: %s
- Cargo: %s
- Análisis de perfil: %s

Intención detectada: %s

Historial de conversación:
%s

Mensaje actual del prospect: "%s"

Escribe la respuesta. Solo el texto del mensaje, sin comillas ni prefijos.`,
		stringOr(p.FirstName, "el prospect"),
		stringOr(p.Company, "N/A"),
		stringOr(p.Headline, "N/A"),
		string(enrichmentJSON),
		classification,
		historyFormatted,
		incomingText,
	)
}

func formatHistory(h []HistoryMessage, lastN int) string {
	start := len(h) - lastN
	if start < 0 {
		start = 0
	}
	var b strings.Builder
	for _, m := range h[start:] {
		who := "Prospect"
		if m.IsSender {
			who = "Nosotros"
		}
		fmt.Fprintf(&b, "%s: %s\n", who, m.Text)
	}
	return strings.TrimRight(b.String(), "\n")
}

func buildAuthorizedContextBlob(p Persona, pr Prospect, h []HistoryMessage, incoming string) string {
	var b strings.Builder
	b.WriteString(p.SystemPrompt)
	b.WriteByte('\n')
	if j, err := json.Marshal(p.GuionesJSON); err == nil {
		b.Write(j)
		b.WriteByte('\n')
	}
	if j, err := json.Marshal(pr.EnrichmentJSON); err == nil {
		b.Write(j)
		b.WriteByte('\n')
	}
	if j, err := json.Marshal(pr.ContactInfo); err == nil {
		b.Write(j)
		b.WriteByte('\n')
	}
	for _, m := range h {
		b.WriteString(m.Text)
		b.WriteByte('\n')
	}
	b.WriteString(incoming)
	return b.String()
}

// PII detectors. Patterns mirror lib/ai-safety.js extractAndCheckPii.
var (
	rePhone = regexp.MustCompile(`(\+?\d[\d\s\-().]{7,}\d)`)
	reEmail = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)
	reURL   = regexp.MustCompile(`https?://[^\s)]+`)
)

// piiLeaks returns the list of PII tokens present in reply but NOT in
// authBlob. Phones are normalized (strip non-digits) before comparison so
// formatting differences don't trigger false positives.
func piiLeaks(reply, authBlob string) []string {
	var leaks []string

	for _, m := range reEmail.FindAllString(reply, -1) {
		if !strings.Contains(strings.ToLower(authBlob), strings.ToLower(m)) {
			leaks = append(leaks, "email:"+m)
		}
	}
	for _, m := range reURL.FindAllString(reply, -1) {
		mNorm := strings.TrimRight(m, ".,;:!?")
		// Also strip trailing url-encoded punctuation
		if u, err := url.Parse(mNorm); err == nil && u.Host != "" {
			if !strings.Contains(authBlob, u.Host) {
				leaks = append(leaks, "url:"+u.Host)
			}
		}
	}
	authDigits := stripNonDigits(authBlob)
	for _, m := range rePhone.FindAllString(reply, -1) {
		digits := stripNonDigits(m)
		if len(digits) < 8 {
			continue
		}
		if !strings.Contains(authDigits, digits) {
			leaks = append(leaks, "phone:"+m)
		}
	}
	return leaks
}

func stripNonDigits(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
