// Package template renders Liquid-style + legacy-compat message templates.
//
// Supported syntax (port of lib/template-engine.js from the Node original):
//
//   - Variables:           {{firstName}}, {{company}}, etc.
//   - Fallback:            {{firstName|amigo}}    (pipe + literal fallback; quotes optional)
//   - Filters:             {{name|upcase}}, {{bio|truncate:80}}
//     (upcase, downcase, capitalize, trim, truncate:N — chainable)
//   - Conditionals:        {% if jobTitle == "CEO" %}A{% else %}B{% endif %}
//     (operators: ==, !=, contains, not_contains; combinable with `and`/`or`)
//   - Spin blocks:         {% spin %}{% variation %}A{% variation %}B{% endspin %}
//   - Legacy spintax:      {A|B|C}
//   - Legacy single tokens:{nombre}, {empresa}, {cargo}, {icebreaker}
//   - Legacy gender:       {H|M|N}  (chosen by lead.Gender ∈ {H, M, F, N})
//   - Multimedia:          {foto1}, {video2}, {imagen1}, {audio3}, {gif1}
//     (resolved via MediaResolver; if missing → hard error)
//   - Sender prefix:       {{sender.name}}, {{sender.signature}}
//   - Escaping:            \{{  and  \{  render literal
//
// The render pipeline runs steps in this order so legacy syntax doesn't collide
// with new Liquid syntax:
//
//  1. Escape \{{ and \{ to placeholders
//  2. Conditionals (operate on lead variables, not on already-rendered output)
//  3. Spin blocks {% spin %}...{% endspin %}
//  4. Variables {{ ... }}
//  5. Legacy tokens {nombre}, {foto1}, {H|M|N}
//  6. Legacy spintax {A|B|C}
//  7. Restore escapes
package template

import (
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// Lead is the source of variable values for variable resolution.
// First-class fields are looked up by name (with aliases); fall back to
// Variables (custom CSV columns).
type Lead struct {
	FirstName      string
	LastName       string
	FullName       string
	Company        string
	Position       string
	Headline       string
	JobTitle       string
	Industry       string
	Location       string
	LinkedInURL    string
	Phone          string
	Email          string
	Icebreaker     string
	School         string
	Gender         string         // "H" | "M" | "F" | "N" (case-insensitive)
	EnrichmentJSON map[string]any // free-form enrichment; "icebreaker" is read as fallback for {{icebreaker}}
	Variables      map[string]any // custom CSV columns / overrides
}

// Sender is the user/account sending the message (used for {{sender.X}} tokens).
type Sender struct {
	Name         string
	Signature    string
	CalendarLink string
}

// MediaResolver returns an attachment for a legacy media token (e.g. "foto1").
// Return nil to signal the token is unresolved (caller raises a hard error).
type MediaResolver func(token string) *Attachment

// Attachment is a media payload produced by resolving {foto1}, {video2}, etc.
type Attachment struct {
	URL      string
	Type     string
	FileName string
}

// Options tweaks rendering behavior.
type Options struct {
	RNG    func() float64 // 0.0 <= rng < 1.0; defaults to crypto-less rand.Float64
	Strict bool           // when true, Render returns error on hard errors
}

// Result is the outcome of rendering a template.
type Result struct {
	Text             string
	MediaAttachments []Attachment
	MissingVars      []string
	HardErrors       []string
}

// Error is returned by RenderStrict (or Render with Strict=true) when the
// template referenced media tokens the resolver could not provide.
type Error struct {
	HardErrors  []string
	MissingVars []string
}

func (e *Error) Error() string {
	return fmt.Sprintf("template render hard errors: %s", strings.Join(e.HardErrors, ", "))
}

// Render evaluates tpl against the given lead/sender context. It never panics;
// missing variables go into Result.MissingVars and unresolved media tokens go
// into Result.HardErrors. When opts.Strict is true and HardErrors is non-empty,
// the returned error is *Error.
func Render(tpl string, lead *Lead, sender *Sender, resolver MediaResolver, opts Options) (Result, error) {
	if tpl == "" {
		return Result{}, nil
	}

	rng := opts.RNG
	if rng == nil {
		rng = rand.Float64
	}
	if lead == nil {
		lead = &Lead{}
	}
	if sender == nil {
		sender = &Sender{}
	}

	res := Result{}
	out := tpl

	// 1. Escape sentinels for \{{ and \{
	out = strings.ReplaceAll(out, `\{{`, escDLB)
	out = strings.ReplaceAll(out, `\{`, escLB)

	// 2. Conditionals (innermost first, supports nesting)
	out = renderConditionals(out, lead, sender)

	// 3. Spin blocks {% spin %}{% variation %}...{% endspin %}
	out = renderSpinBlocks(out, rng)

	// 4. Variables {{ ... }}
	out = renderVariables(out, lead, sender, &res.MissingVars)

	// 5. Legacy tokens (gender + named + multimedia)
	out = renderLegacyTokens(out, lead, resolver, &res.MediaAttachments, &res.MissingVars, &res.HardErrors)

	// 6. Legacy spintax {A|B|C}
	out = renderLegacySpintax(out, rng)

	// 7. Restore escapes
	out = strings.ReplaceAll(out, escDLB, "{{")
	out = strings.ReplaceAll(out, escLB, "{")

	res.Text = out

	if opts.Strict && len(res.HardErrors) > 0 {
		return res, &Error{HardErrors: res.HardErrors, MissingVars: res.MissingVars}
	}
	return res, nil
}

// RenderStrict is Render with Strict=true.
func RenderStrict(tpl string, lead *Lead, sender *Sender, resolver MediaResolver, opts Options) (Result, error) {
	opts.Strict = true
	return Render(tpl, lead, sender, resolver, opts)
}

const (
	escDLB = "\x00ESC_DLB\x00"
	escLB  = "\x00ESC_LB\x00"
)

// ----------------------------------------------------------------------------
// Variable resolution
// ----------------------------------------------------------------------------

// aliases maps a variable name to other names to try if the primary is empty.
// Mirrors the readVarFromLead aliases in the JS original.
var aliases = map[string][]string{
	"firstName":   {"first_name", "primer_nombre", "nombre"},
	"first_name":  {"firstName", "primer_nombre", "nombre"},
	"nombre":      {"firstName", "first_name", "primer_nombre", "fullName"},
	"lastName":    {"last_name", "primer_apellido", "apellido"},
	"last_name":   {"lastName", "primer_apellido", "apellido"},
	"apellido":    {"primer_apellido", "last_name", "lastName"},
	"fullName":    {"full_name", "name", "nombre_completo"},
	"full_name":   {"fullName", "name"},
	"company":     {"companyName", "company_name", "empresa_actual", "empresa"},
	"companyName": {"company", "company_name", "empresa_actual", "empresa"},
	"empresa":     {"empresa_actual", "company", "companyName", "company_name"},
	"position":    {"cargo_actual", "jobTitle", "job_title", "headline", "cargo"},
	"jobTitle":    {"cargo_actual", "position", "job_title", "headline", "cargo"},
	"headline":    {"cargo_actual", "position", "jobTitle", "job_title", "cargo"},
	"cargo":       {"cargo_actual", "headline", "position", "jobTitle"},
	"linkedinUrl": {"profile_url", "linkedin_url"},
	"profile_url": {"linkedinUrl", "linkedin_url"},
	"icebreaker":  {"enrichment_icebreaker"},
}

// readVar returns the value for a variable name. "sender.X" reads from sender.
// Returns ("", false) if nothing was found / value is empty.
func readVar(name string, lead *Lead, sender *Sender) (string, bool) {
	if strings.HasPrefix(name, "sender.") {
		key := strings.TrimPrefix(name, "sender.")
		switch key {
		case "name":
			return nonEmpty(sender.Name)
		case "signature":
			return nonEmpty(sender.Signature)
		case "calendarLink", "calendar_link":
			return nonEmpty(sender.CalendarLink)
		}
		return "", false
	}

	// Direct fields on Lead struct
	if v, ok := readLeadField(name, lead); ok {
		return v, true
	}

	// Aliases
	for _, alias := range aliases[name] {
		if v, ok := readLeadField(alias, lead); ok {
			return v, true
		}
	}

	// Icebreaker fallback to EnrichmentJSON["icebreaker"]
	if name == "icebreaker" && lead.EnrichmentJSON != nil {
		if v, ok := lead.EnrichmentJSON["icebreaker"]; ok {
			s := toString(v)
			if s != "" {
				return s, true
			}
		}
	}

	// Custom Variables
	if lead.Variables != nil {
		if v, ok := lead.Variables[name]; ok {
			return toString(v), toString(v) != ""
		}
	}
	return "", false
}

// readLeadField looks up a name against the typed Lead fields (no alias / no
// variables fallback).
func readLeadField(name string, lead *Lead) (string, bool) {
	switch name {
	case "firstName", "first_name", "primer_nombre":
		return nonEmpty(lead.FirstName)
	case "lastName", "last_name", "primer_apellido":
		return nonEmpty(lead.LastName)
	case "fullName", "full_name", "nombre_completo":
		return nonEmpty(lead.FullName)
	case "company", "companyName", "company_name", "empresa", "empresa_actual":
		return nonEmpty(lead.Company)
	case "position", "cargo_actual":
		return nonEmpty(lead.Position)
	case "headline":
		return nonEmpty(lead.Headline)
	case "jobTitle", "job_title", "cargo":
		// jobTitle / cargo defer to JobTitle field; fall back to Headline.
		if v, ok := nonEmpty(lead.JobTitle); ok {
			return v, true
		}
		return nonEmpty(lead.Headline)
	case "industry", "sector":
		return nonEmpty(lead.Industry)
	case "location":
		return nonEmpty(lead.Location)
	case "linkedinUrl", "linkedin_url", "profile_url":
		return nonEmpty(lead.LinkedInURL)
	case "phone":
		return nonEmpty(lead.Phone)
	case "email":
		return nonEmpty(lead.Email)
	case "icebreaker":
		return nonEmpty(lead.Icebreaker)
	case "school":
		return nonEmpty(lead.School)
	}
	return "", false
}

func nonEmpty(s string) (string, bool) {
	if s == "" {
		return "", false
	}
	return s, true
}

func toString(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case fmt.Stringer:
		return t.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}

// ----------------------------------------------------------------------------
// Filters
// ----------------------------------------------------------------------------

var filterFns = map[string]func(string, []string) string{
	"upcase":     func(v string, _ []string) string { return strings.ToUpper(v) },
	"downcase":   func(v string, _ []string) string { return strings.ToLower(v) },
	"capitalize": capitalize,
	"trim":       func(v string, _ []string) string { return strings.TrimSpace(v) },
	"truncate":   truncate,
}

func capitalize(v string, _ []string) string {
	if v == "" {
		return v
	}
	rs := []rune(v)
	rs[0] = unicode.ToUpper(rs[0])
	for i := 1; i < len(rs); i++ {
		rs[i] = unicode.ToLower(rs[i])
	}
	return string(rs)
}

func truncate(v string, args []string) string {
	if len(args) == 0 {
		return v
	}
	n, err := strconv.Atoi(strings.TrimSpace(args[0]))
	if err != nil || n < 0 {
		return v
	}
	rs := []rune(v)
	if len(rs) > n {
		return string(rs[:n])
	}
	return v
}

func applyFilter(value, filterExpr string) string {
	head, rest, _ := strings.Cut(filterExpr, ":")
	head = strings.TrimSpace(head)
	fn, ok := filterFns[head]
	if !ok {
		return value
	}
	var args []string
	if rest != "" {
		args = []string{rest}
	}
	return fn(value, args)
}

func stripQuotes(s string) string {
	if len(s) >= 2 {
		first, last := s[0], s[len(s)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// ----------------------------------------------------------------------------
// Variable expression: name | filter | filter:N | fallback
// ----------------------------------------------------------------------------

type varEvalResult struct {
	value   string
	missing bool
	varName string
}

func evaluateVarExpression(expr string, lead *Lead, sender *Sender) varEvalResult {
	parts := splitAndTrim(expr, '|')
	if len(parts) == 0 {
		return varEvalResult{value: ""}
	}
	varName := parts[0]
	value, found := readVar(varName, lead, sender)
	missing := !found

	var fallback *string
	var filters []string

	for i := 1; i < len(parts); i++ {
		seg := parts[i]
		head, _, _ := strings.Cut(seg, ":")
		head = strings.TrimSpace(head)
		if _, isFilter := filterFns[head]; isFilter {
			filters = append(filters, seg)
			continue
		}
		// First non-filter pipe-section is the fallback; remaining filters apply
		fb := stripQuotes(seg)
		fallback = &fb
		for _, r := range parts[i+1:] {
			filters = append(filters, r)
		}
		break
	}

	if missing && fallback != nil {
		value = *fallback
		missing = false
	}

	for _, f := range filters {
		value = applyFilter(value, f)
	}

	return varEvalResult{value: value, missing: missing, varName: varName}
}

func splitAndTrim(s string, sep byte) []string {
	out := []string{}
	for _, p := range strings.Split(s, string(sep)) {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ----------------------------------------------------------------------------
// Conditionals: {% if X %}...{% else %}...{% endif %}
// ----------------------------------------------------------------------------

var (
	reIfTag   = regexp.MustCompile(`\{%\s*if\s+([^%]+?)\s*%\}`)
	reElseTag = regexp.MustCompile(`\{%\s*else\s*%\}`)
	reEndIf   = regexp.MustCompile(`\{%\s*endif\s*%\}`)
)

// renderConditionals iteratively expands innermost if/endif blocks.
// Implemented as a token-scan to avoid regex lookahead (RE2-incompatible).
func renderConditionals(tpl string, lead *Lead, sender *Sender) string {
	out := tpl
	for safety := 0; safety < 100; safety++ {
		ifMatch, elseMatch, endMatch, ok := findInnermostIfBlock(out)
		if !ok {
			break
		}
		condStr := strings.TrimSpace(ifMatch.captured)
		innerStart := ifMatch.end
		innerEnd := endMatch.start

		truePart, falsePart := "", ""
		if elseMatch != nil && elseMatch.start >= innerStart && elseMatch.end <= innerEnd {
			truePart = out[innerStart:elseMatch.start]
			falsePart = out[elseMatch.end:innerEnd]
		} else {
			truePart = out[innerStart:innerEnd]
		}

		replacement := falsePart
		if evaluateCondition(condStr, lead, sender) {
			replacement = truePart
		}
		out = out[:ifMatch.start] + replacement + out[endMatch.end:]
	}
	return out
}

type tagMatch struct {
	start    int    // start in input
	end      int    // end in input (exclusive)
	captured string // first capture group (for {% if X %} → "X")
}

type condToken struct {
	kind     string // "if" | "else" | "endif"
	start    int
	end      int
	captured string
}

// findInnermostIfBlock returns the innermost {% if %} ... {% endif %} block.
// Innermost = the {% if %} whose nearest following tag is {% endif %} or
// {% else %} (i.e., no {% if %} between it and its closing tag).
func findInnermostIfBlock(s string) (ifM, elseM, endM *tagMatch, ok bool) {
	var tokens []condToken
	for _, m := range reIfTag.FindAllStringSubmatchIndex(s, -1) {
		tokens = append(tokens, condToken{
			kind: "if", start: m[0], end: m[1], captured: s[m[2]:m[3]],
		})
	}
	for _, m := range reElseTag.FindAllStringIndex(s, -1) {
		tokens = append(tokens, condToken{kind: "else", start: m[0], end: m[1]})
	}
	for _, m := range reEndIf.FindAllStringIndex(s, -1) {
		tokens = append(tokens, condToken{kind: "endif", start: m[0], end: m[1]})
	}
	if len(tokens) == 0 {
		return nil, nil, nil, false
	}
	sortCondTokens(tokens)

	// Walk tokens with a stack. When we close an {% if %} ({% endif %}), check
	// if any tokens between the matching {% if %} and this {% endif %} are
	// themselves {% if %} (they're not, because the stack would still have them
	// pending). So at this point the block is innermost.
	stack := []int{}
	for i, t := range tokens {
		switch t.kind {
		case "if":
			stack = append(stack, i)
		case "endif":
			if len(stack) == 0 {
				continue
			}
			ifIdx := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			// Find {% else %} at this level (no nested ifs are possible here)
			var elseTok *condToken
			for k := ifIdx + 1; k < i; k++ {
				if tokens[k].kind == "else" {
					t := tokens[k]
					elseTok = &t
					break
				}
			}
			ift := tokens[ifIdx]
			et := tokens[i]
			ifMatch := &tagMatch{start: ift.start, end: ift.end, captured: ift.captured}
			endMatch := &tagMatch{start: et.start, end: et.end}
			var elseMatch *tagMatch
			if elseTok != nil {
				elseMatch = &tagMatch{start: elseTok.start, end: elseTok.end}
			}
			return ifMatch, elseMatch, endMatch, true
		}
	}
	return nil, nil, nil, false
}

func sortCondTokens(tokens []condToken) {
	for i := 1; i < len(tokens); i++ {
		for j := i; j > 0 && tokens[j].start < tokens[j-1].start; j-- {
			tokens[j], tokens[j-1] = tokens[j-1], tokens[j]
		}
	}
}

// evaluateCondition parses "name OP value [and|or ...]" left-to-right.
func evaluateCondition(cond string, lead *Lead, sender *Sender) bool {
	parts := splitConjunctions(cond)
	if len(parts) == 0 {
		return false
	}
	result := evalSimple(parts[0].expr, lead, sender)
	for i := 1; i < len(parts); i++ {
		next := evalSimple(parts[i].expr, lead, sender)
		switch parts[i].joiner {
		case "and":
			result = result && next
		case "or":
			result = result || next
		}
	}
	return result
}

type condPart struct {
	expr   string
	joiner string // "" for the first, "and" or "or" otherwise
}

var reJoiner = regexp.MustCompile(`(?i)\s+(and|or)\s+`)

// splitConjunctions splits "A and B or C" into [{expr:"A"}, {joiner:"and", expr:"B"}, {joiner:"or", expr:"C"}].
func splitConjunctions(s string) []condPart {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	idx := reJoiner.FindAllStringSubmatchIndex(s, -1)
	if len(idx) == 0 {
		return []condPart{{expr: s}}
	}
	out := make([]condPart, 0, len(idx)+1)
	prev := 0
	for i, m := range idx {
		joiner := ""
		if i > 0 {
			joiner = strings.ToLower(s[idx[i-1][2]:idx[i-1][3]])
		}
		out = append(out, condPart{
			expr:   strings.TrimSpace(s[prev:m[0]]),
			joiner: joiner,
		})
		prev = m[1]
	}
	lastJoiner := strings.ToLower(s[idx[len(idx)-1][2]:idx[len(idx)-1][3]])
	out = append(out, condPart{
		expr:   strings.TrimSpace(s[prev:]),
		joiner: lastJoiner,
	})
	return out
}

var reSimpleCond = regexp.MustCompile(`^(.+?)\s+(==|!=|contains|not_contains)\s+(.+)$`)

func evalSimple(expr string, lead *Lead, sender *Sender) bool {
	expr = strings.TrimSpace(expr)
	m := reSimpleCond.FindStringSubmatch(expr)
	if m == nil {
		v, ok := readVar(expr, lead, sender)
		return ok && v != ""
	}
	left, _ := readVar(strings.TrimSpace(m[1]), lead, sender)
	op := strings.ToLower(m[2])
	right := stripQuotes(strings.TrimSpace(m[3]))
	switch op {
	case "==":
		return left == right
	case "!=":
		return left != right
	case "contains":
		return strings.Contains(strings.ToLower(left), strings.ToLower(right))
	case "not_contains":
		return !strings.Contains(strings.ToLower(left), strings.ToLower(right))
	}
	return false
}

// ----------------------------------------------------------------------------
// Spin blocks {% spin %}{% variation %}A{% variation %}B{% endspin %}
// ----------------------------------------------------------------------------

var (
	reSpinOpen  = regexp.MustCompile(`\{%\s*spin\s*%\}`)
	reSpinClose = regexp.MustCompile(`\{%\s*endspin\s*%\}`)
	reVariation = regexp.MustCompile(`\{%\s*variation\s*%\}`)
)

func renderSpinBlocks(tpl string, rng func() float64) string {
	out := tpl
	for safety := 0; safety < 100; safety++ {
		openIdx := reSpinOpen.FindStringIndex(out)
		if openIdx == nil {
			break
		}
		closeIdx := reSpinClose.FindStringIndex(out[openIdx[1]:])
		if closeIdx == nil {
			break
		}
		bodyStart := openIdx[1]
		bodyEnd := openIdx[1] + closeIdx[0]
		closeEnd := openIdx[1] + closeIdx[1]

		body := out[bodyStart:bodyEnd]
		parts := []string{}
		for _, p := range reVariation.Split(body, -1) {
			p = strings.TrimSpace(p)
			if p != "" {
				parts = append(parts, p)
			}
		}
		pick := ""
		if len(parts) > 0 {
			pick = parts[int(rng()*float64(len(parts)))%len(parts)]
		}
		out = out[:openIdx[0]] + pick + out[closeEnd:]
	}
	return out
}

// ----------------------------------------------------------------------------
// Variables {{ ... }}
// ----------------------------------------------------------------------------

var reVar = regexp.MustCompile(`\{\{\s*([^{}]+?)\s*\}\}`)

func renderVariables(tpl string, lead *Lead, sender *Sender, missing *[]string) string {
	return reVar.ReplaceAllStringFunc(tpl, func(match string) string {
		// Capture inner expression
		sub := reVar.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		r := evaluateVarExpression(sub[1], lead, sender)
		if r.missing && r.varName != "" {
			*missing = append(*missing, r.varName)
		}
		return r.value
	})
}

// ----------------------------------------------------------------------------
// Legacy tokens: gender {H|M|N}, named {nombre}, multimedia {foto1}
// ----------------------------------------------------------------------------

var reBraced = regexp.MustCompile(`\{([^{}\n]+)\}`)
var reSimpleName = regexp.MustCompile(`(?i)^([a-zA-Záéíóúñ_][a-zA-Záéíóúñ0-9_]*)$`)
var reMedia = regexp.MustCompile(`^(foto|video|imagen|audio|gif)(\d+)$`)

func renderLegacyTokens(tpl string, lead *Lead, resolver MediaResolver, attachments *[]Attachment, missing *[]string, hardErrors *[]string) string {
	// Pass 1: gender {H|M|N}. Three-option pipe groups where lead.Gender is set.
	out := reBraced.ReplaceAllStringFunc(tpl, func(match string) string {
		inner := match[1 : len(match)-1]
		if !strings.Contains(inner, "|") {
			return match
		}
		opts := strings.Split(inner, "|")
		if len(opts) != 3 || lead == nil {
			return match
		}
		g := strings.ToUpper(lead.Gender)
		switch g {
		case "H":
			return opts[0]
		case "M", "F":
			return opts[1]
		case "N":
			return opts[2]
		}
		return match
	})

	// Pass 2: simple named tokens + multimedia.
	out = reBraced.ReplaceAllStringFunc(out, func(match string) string {
		inner := match[1 : len(match)-1]
		if !reSimpleName.MatchString(inner) {
			return match // not a simple identifier (could be spintax)
		}
		lower := strings.ToLower(inner)
		switch lower {
		case "nombre":
			if v, ok := readVar("firstName", lead, &Sender{}); ok {
				return v
			}
			if v, ok := readVar("fullName", lead, &Sender{}); ok {
				return v
			}
			*missing = append(*missing, inner)
			return ""
		case "empresa":
			if v, ok := readVar("company", lead, &Sender{}); ok {
				return v
			}
			*missing = append(*missing, inner)
			return ""
		case "cargo":
			if v, ok := readVar("headline", lead, &Sender{}); ok {
				return v
			}
			if v, ok := readVar("position", lead, &Sender{}); ok {
				return v
			}
			*missing = append(*missing, inner)
			return ""
		case "icebreaker":
			if v, ok := readVar("icebreaker", lead, &Sender{}); ok {
				return v
			}
			return "" // do NOT mark icebreaker as missing (legacy behavior)
		}

		// Multimedia
		if mm := reMedia.FindStringSubmatch(lower); mm != nil {
			if resolver != nil {
				if att := resolver(lower); att != nil {
					*attachments = append(*attachments, *att)
					return ""
				}
			}
			*hardErrors = append(*hardErrors, "media_unresolved:"+lower)
			return ""
		}
		return match
	})

	return out
}

// ----------------------------------------------------------------------------
// Legacy spintax {A|B|C}
// ----------------------------------------------------------------------------

func renderLegacySpintax(tpl string, rng func() float64) string {
	return reBraced.ReplaceAllStringFunc(tpl, func(match string) string {
		inner := match[1 : len(match)-1]
		if !strings.Contains(inner, "|") {
			return match
		}
		opts := strings.Split(inner, "|")
		trimmed := make([]string, 0, len(opts))
		for _, o := range opts {
			trimmed = append(trimmed, strings.TrimSpace(o))
		}
		if len(trimmed) < 2 {
			return match
		}
		return trimmed[int(rng()*float64(len(trimmed)))%len(trimmed)]
	})
}
