package ai

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
)

// Intent is the classification output's primary tag.
type Intent string

const (
	IntentInterest             Intent = "interest"
	IntentObjection            Intent = "objection"
	IntentNotInterested        Intent = "not_interested"
	IntentOutOfOffice          Intent = "out_of_office"
	IntentInfoRequest          Intent = "info_request"
	IntentQuestionAboutSender  Intent = "question_about_sender"
	IntentAggressive           Intent = "aggressive"
	IntentOther                Intent = "other"
)

// Classification is the full result of ClassifyMessage. FastDetected is set
// when we short-circuited via STOP_WORDS / OBVIOUS_PATTERNS (zero tokens spent).
type Classification struct {
	Intent       Intent  `json:"intent"`
	Confidence   float64 `json:"confidence"`
	Temperature  string  `json:"temperature"`   // hot | warm | cold
	MessageType  string  `json:"message_type"`  // question_about_sender | open_door | passive | objection | aggressive
	RawQuestion  string  `json:"raw_question,omitempty"`
	FastDetected string  `json:"fast_detected,omitempty"` // populated when matched by a fast pattern
	Model        string  `json:"model,omitempty"`
	Usage        Usage   `json:"usage,omitempty"`
	CostUSD      float64 `json:"cost_usd,omitempty"`
}

// StopWords mark explicit opt-out — we never call the LLM for these.
var StopWords = []string{
	"no me interesa", "no gracias", "no estoy interesado",
	"stop", "detén", "para", "baja", "remove", "unsubscribe",
	"no me molestes", "no contactar", "por favor no", "no quiero",
}

// AggressiveWords short-circuit to Intent="aggressive" without LLM.
var AggressiveWords = []string{
	"déjame en paz", "deja de molestar", "no me vuelvas a escribir",
	"spam", "reportar", "bloquear", "eres un bot", "eres bot",
	"molesto", "acosador", "idiota", "estúpido", "tonto",
}

// ObviousPattern is a regex that, when matched on an isolated message (no prior
// substantive context), shortcuts the classification.
type ObviousPattern struct {
	RE          *regexp.Regexp
	Intent      Intent
	Temperature string
	MessageType string
	FastTag     string
}

// ObviousPatterns is the same list as OBVIOUS_PATTERNS in classify.js.
var ObviousPatterns = []ObviousPattern{
	{regexp.MustCompile(`(?i)^\s*(gracias|thanks|thx|ty|tks)\s*[!.?]*\s*$`), IntentOther, "warm", "passive", "thanks"},
	{regexp.MustCompile(`(?i)^\s*(ok|okay|dale|listo|perfecto|👍|👌|✅|saludos)\s*[!.?]*\s*$`), IntentOther, "cold", "passive", "ack"},
	{regexp.MustCompile(`(?i)^\s*(hola|buen[ao]s?|hi|hello|hey)\s*[!.?]*\s*$`), IntentOther, "cold", "passive", "greeting"},
	{regexp.MustCompile(`(?i)\b(estoy de vacaciones|out of office|fuera de la oficina|aviso autom[áa]tico)\b`), IntentOutOfOffice, "cold", "passive", "ooo"},
	{regexp.MustCompile(`(?i)\b(precio|costo|cu[áa]nto cuesta|cu[áa]nto sale|cotizaci[óo]n)\b`), IntentInfoRequest, "hot", "open_door", "price"},
}

// IsExplicitOptOut reports whether text matches any STOP_WORDS (case-insensitive).
func IsExplicitOptOut(text string) bool {
	if text == "" {
		return false
	}
	lower := strings.ToLower(text)
	for _, w := range StopWords {
		if strings.Contains(lower, w) {
			return true
		}
	}
	return false
}

// IsAggressive reports whether text matches any AggressiveWords (case-insensitive).
func IsAggressive(text string) bool {
	if text == "" {
		return false
	}
	lower := strings.ToLower(text)
	for _, w := range AggressiveWords {
		if strings.Contains(lower, w) {
			return true
		}
	}
	return false
}

// HistoryMessage is one item of the prior conversation between sender and prospect.
type HistoryMessage struct {
	Text     string
	IsSender bool // true = sent by us; false = sent by the prospect
}

// classifySystem is the system prompt sent to Anthropic. Identical text as
// CLASSIFY_SYSTEM in classify.js — do NOT edit without checking the JS first.
const classifySystem = `Clasificas mensajes de LinkedIn. Responde SOLO en JSON válido con este schema exacto:
{
  "intent": "interest|objection|not_interested|out_of_office|info_request|question_about_sender|aggressive|other",
  "confidence": 0.0-1.0,
  "temperature": "hot|warm|cold",
  "message_type": "question_about_sender|open_door|passive|objection|aggressive",
  "raw_question": "texto literal de la pregunta si aplica, o null"
}

Definiciones de intent:
- interest: muestra curiosidad, quiere saber más, pregunta cómo funciona, agenda una llamada
- objection: dice que está ocupado, que tiene otra solución, que no es el momento
- not_interested: rechaza claramente, pide que no se le contacte
- out_of_office: aviso automático de ausencia, respuesta de vacaciones
- info_request: pide información concreta (precio, demo, características)
- question_about_sender: pregunta directamente sobre la persona que escribió ("¿a qué te dedicas?", "¿qué tipo de equipos diriges?", "¿en qué consiste tu trabajo?")
- aggressive: respuesta hostil, insultos, pide que no se le contacte con enojo
- other: cualquier otra cosa (saludo simple, respuesta confusa, cortesía neutra)

Definiciones de temperature:
- hot: entusiasmo claro, pregunta directa de interés, dice que quiere saber más
- warm: respuesta positiva pero sin comprometerse, abre una puerta, cortesía amable
- cold: monosílabo, emoji solo, "ok", "saludos", cortesía fría sin interés

Definiciones de message_type:
- question_about_sender: preguntó algo sobre quien le escribió (perfil, trabajo, qué hace)
- open_door: dio luz verde implícita ("qué tienes en mente", "cuéntame", "me interesa")
- passive: respondió sin energía ni interés real
- objection: empujó hacia atrás
- aggressive: fue hostil`

// ClassifyConfig configures which models to use.
type ClassifyConfig struct {
	FastModel  string // default: claude-haiku-4-5-20251001
	SmartModel string // default: claude-sonnet-4-6
}

// Classifier holds the Anthropic client + model preferences.
type Classifier struct {
	client     *AnthropicClient
	fastModel  string
	smartModel string
}

// NewClassifier builds a Classifier.
func NewClassifier(client *AnthropicClient, cfg ClassifyConfig) *Classifier {
	c := &Classifier{client: client, fastModel: cfg.FastModel, smartModel: cfg.SmartModel}
	if c.fastModel == "" {
		c.fastModel = "claude-haiku-4-5-20251001"
	}
	if c.smartModel == "" {
		c.smartModel = "claude-sonnet-4-6"
	}
	return c
}

// Classify returns the intent of messageText, optionally using prior messages
// for context. Pipeline:
//  1. Explicit opt-out → not_interested 1.0 (no tokens)
//  2. Aggressive → aggressive 1.0 (no tokens)
//  3. Obvious patterns (only when no substantive prior context) → match (no tokens)
//  4. Haiku call; on failure, escalate to Sonnet
func (c *Classifier) Classify(ctx context.Context, messageText string, history []HistoryMessage) (*Classification, error) {
	if IsExplicitOptOut(messageText) {
		return &Classification{
			Intent: IntentNotInterested, Confidence: 1.0, Temperature: "cold",
			MessageType: "passive", FastDetected: "opt_out",
		}, nil
	}
	if IsAggressive(messageText) {
		return &Classification{
			Intent: IntentAggressive, Confidence: 1.0, Temperature: "cold",
			MessageType: "aggressive", FastDetected: "aggressive",
		}, nil
	}

	priorProspect := filterProspect(history)
	hasContext := false
	for _, m := range priorProspect {
		if len(strings.TrimSpace(m.Text)) > 15 {
			hasContext = true
			break
		}
	}
	if !hasContext {
		for _, pat := range ObviousPatterns {
			if pat.RE.MatchString(messageText) {
				return &Classification{
					Intent: pat.Intent, Confidence: 0.9,
					Temperature: pat.Temperature, MessageType: pat.MessageType,
					FastDetected: pat.FastTag,
				}, nil
			}
		}
	}

	userMsg := buildClassifyUserMsg(messageText, priorProspect)
	req := MessagesRequest{
		Model:     c.fastModel,
		MaxTokens: 150,
		System:    []SystemBlock{(SystemBlock{Type: "text", Text: classifySystem}).WithCache()},
		Messages:  []Message{{Role: "user", Content: userMsg}},
	}

	used := c.fastModel
	resp, err := c.client.Create(ctx, req)
	if err != nil {
		// Escalate to smart model
		req.Model = c.smartModel
		used = c.smartModel
		resp, err = c.client.Create(ctx, req)
		if err != nil {
			return nil, err
		}
	}

	parsed := parseClassifyJSON(resp.Text())
	out := &Classification{
		Intent:      Intent(stringOr(parsed.Intent, "other")),
		Confidence:  parsed.Confidence,
		Temperature: stringOr(parsed.Temperature, "warm"),
		MessageType: stringOr(parsed.MessageType, "passive"),
		RawQuestion: parsed.RawQuestion,
		Model:       used,
		Usage:       resp.Usage,
		CostUSD:     CalcCost(used, resp.Usage),
	}
	if !validIntent(out.Intent) {
		out.Intent = IntentOther
		out.Confidence = 0
	}
	return out, nil
}

func buildClassifyUserMsg(messageText string, priorProspect []HistoryMessage) string {
	var b strings.Builder
	b.WriteString(`Mensaje más reciente del prospect: "`)
	b.WriteString(messageText)
	b.WriteString(`"`)
	if len(priorProspect) > 0 {
		start := len(priorProspect) - 3
		if start < 0 {
			start = 0
		}
		b.WriteString("\n\nMensajes anteriores del mismo prospect en este hilo (para calibrar temperatura):\n")
		for _, m := range priorProspect[start:] {
			text := strings.TrimSpace(m.Text)
			if len(text) > 150 {
				text = text[:150]
			}
			b.WriteString(`  - "`)
			b.WriteString(text)
			b.WriteString("\"\n")
		}
		b.WriteString("\nClasifica considerando el hilo completo, no solo el mensaje más reciente.")
	}
	return b.String()
}

type classifyJSON struct {
	Intent      string  `json:"intent"`
	Confidence  float64 `json:"confidence"`
	Temperature string  `json:"temperature"`
	MessageType string  `json:"message_type"`
	RawQuestion string  `json:"raw_question"`
}

// parseClassifyJSON tries to find and decode a JSON object inside text. Robust
// against the LLM prefixing/suffixing the JSON with prose (rare with cache hits
// but possible).
func parseClassifyJSON(text string) classifyJSON {
	var out classifyJSON
	trimmed := strings.TrimSpace(text)
	start := strings.IndexByte(trimmed, '{')
	end := strings.LastIndexByte(trimmed, '}')
	if start == -1 || end == -1 || end < start {
		return out
	}
	_ = json.Unmarshal([]byte(trimmed[start:end+1]), &out)
	return out
}

func validIntent(i Intent) bool {
	switch i {
	case IntentInterest, IntentObjection, IntentNotInterested, IntentOutOfOffice,
		IntentInfoRequest, IntentQuestionAboutSender, IntentAggressive, IntentOther:
		return true
	}
	return false
}

func stringOr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func filterProspect(h []HistoryMessage) []HistoryMessage {
	out := make([]HistoryMessage, 0, len(h))
	for _, m := range h {
		if !m.IsSender {
			out = append(out, m)
		}
	}
	return out
}
