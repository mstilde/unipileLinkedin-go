package template

import (
	"strings"
	"testing"
)

// fixed picks a deterministic option from a spintax/spin set.
// Returns 0 first, then 1, then 0, ... cycling — enough for the tests below.
func fixedRNG() func() float64 {
	calls := 0
	return func() float64 {
		// Return 0.0 always so the first option is picked.
		_ = calls
		return 0.0
	}
}

func TestRender_PlainText(t *testing.T) {
	r, err := Render("Hello world", &Lead{}, &Sender{}, nil, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Text != "Hello world" {
		t.Errorf("got %q", r.Text)
	}
}

func TestRender_BasicVariable(t *testing.T) {
	r, _ := Render("Hi {{firstName}}", &Lead{FirstName: "Juan"}, &Sender{}, nil, Options{})
	if r.Text != "Hi Juan" {
		t.Errorf("got %q", r.Text)
	}
	if len(r.MissingVars) != 0 {
		t.Errorf("expected no missing, got %v", r.MissingVars)
	}
}

func TestRender_VariableMissing_NoFallback(t *testing.T) {
	r, _ := Render("Hi {{firstName}}", &Lead{}, &Sender{}, nil, Options{})
	if r.Text != "Hi " {
		t.Errorf("got %q", r.Text)
	}
	if len(r.MissingVars) != 1 || r.MissingVars[0] != "firstName" {
		t.Errorf("expected missing=[firstName], got %v", r.MissingVars)
	}
}

func TestRender_VariableFallback(t *testing.T) {
	r, _ := Render("Hi {{firstName|amigo}}", &Lead{}, &Sender{}, nil, Options{})
	if r.Text != "Hi amigo" {
		t.Errorf("got %q", r.Text)
	}
	if len(r.MissingVars) != 0 {
		t.Errorf("fallback should consume missing, got %v", r.MissingVars)
	}
}

func TestRender_VariableFallbackQuoted(t *testing.T) {
	r, _ := Render(`Hi {{firstName|"there"}}`, &Lead{}, &Sender{}, nil, Options{})
	if r.Text != "Hi there" {
		t.Errorf("got %q", r.Text)
	}
}

func TestRender_Filters(t *testing.T) {
	cases := []struct {
		tpl, want string
	}{
		{"{{firstName|upcase}}", "JUAN"},
		{"{{firstName|downcase}}", "juan"},
		{"{{firstName|capitalize}}", "Juan"},
		{"{{firstName|trim}}", "JuAn"},
		{"{{firstName|truncate:2}}", "Ju"},
	}
	lead := &Lead{FirstName: "JuAn"}
	for _, tc := range cases {
		r, _ := Render(tc.tpl, lead, &Sender{}, nil, Options{})
		// Capitalize: J + uan
		if tc.tpl == "{{firstName|capitalize}}" && r.Text != "Juan" {
			t.Errorf("capitalize: got %q want Juan", r.Text)
		} else if tc.tpl != "{{firstName|capitalize}}" && r.Text != tc.want {
			t.Errorf("%s: got %q want %q", tc.tpl, r.Text, tc.want)
		}
	}
}

func TestRender_FilterChain(t *testing.T) {
	r, _ := Render("{{company|upcase|truncate:5}}", &Lead{Company: "acmecorp"}, &Sender{}, nil, Options{})
	if r.Text != "ACMEC" {
		t.Errorf("got %q want ACMEC", r.Text)
	}
}

func TestRender_FallbackWithFilters(t *testing.T) {
	r, _ := Render("{{firstName|amigo|upcase}}", &Lead{}, &Sender{}, nil, Options{})
	if r.Text != "AMIGO" {
		t.Errorf("got %q want AMIGO", r.Text)
	}
}

func TestRender_SenderTokens(t *testing.T) {
	r, _ := Render("Saludos, {{sender.name}}", &Lead{}, &Sender{Name: "Mati"}, nil, Options{})
	if r.Text != "Saludos, Mati" {
		t.Errorf("got %q", r.Text)
	}
}

func TestRender_AliasFirstNameNombre(t *testing.T) {
	// Span legacy: {nombre} -> firstName
	r, _ := Render("Hi {nombre}!", &Lead{FirstName: "Juan"}, &Sender{}, nil, Options{})
	if r.Text != "Hi Juan!" {
		t.Errorf("got %q", r.Text)
	}
}

func TestRender_AliasEmpresaCompany(t *testing.T) {
	r, _ := Render("{empresa}", &Lead{Company: "Acme"}, &Sender{}, nil, Options{})
	if r.Text != "Acme" {
		t.Errorf("got %q", r.Text)
	}
}

func TestRender_AliasCargoHeadline(t *testing.T) {
	r, _ := Render("{cargo}", &Lead{Headline: "CEO"}, &Sender{}, nil, Options{})
	if r.Text != "CEO" {
		t.Errorf("got %q", r.Text)
	}
}

func TestRender_Icebreaker_FromEnrichment(t *testing.T) {
	lead := &Lead{EnrichmentJSON: map[string]any{"icebreaker": "vi tu post sobre IA"}}
	r, _ := Render("{{icebreaker}}", lead, &Sender{}, nil, Options{})
	if r.Text != "vi tu post sobre IA" {
		t.Errorf("got %q", r.Text)
	}
}

func TestRender_Gender_H(t *testing.T) {
	r, _ := Render("{Hola colega|Hola amiga|Hola persona}", &Lead{Gender: "H"}, &Sender{}, nil, Options{})
	if r.Text != "Hola colega" {
		t.Errorf("got %q", r.Text)
	}
}

func TestRender_Gender_M(t *testing.T) {
	r, _ := Render("{Hola colega|Hola amiga|Hola persona}", &Lead{Gender: "M"}, &Sender{}, nil, Options{})
	if r.Text != "Hola amiga" {
		t.Errorf("got %q", r.Text)
	}
}

func TestRender_Gender_F(t *testing.T) {
	r, _ := Render("{Hola colega|Hola amiga|Hola persona}", &Lead{Gender: "F"}, &Sender{}, nil, Options{})
	if r.Text != "Hola amiga" {
		t.Errorf("got %q (F should map to opt[1])", r.Text)
	}
}

func TestRender_Gender_N(t *testing.T) {
	r, _ := Render("{Hola colega|Hola amiga|Hola persona}", &Lead{Gender: "N"}, &Sender{}, nil, Options{})
	if r.Text != "Hola persona" {
		t.Errorf("got %q", r.Text)
	}
}

func TestRender_Conditionals_True(t *testing.T) {
	tpl := `{% if jobTitle == "CEO" %}Hola CEO{% else %}Hola{% endif %}`
	r, _ := Render(tpl, &Lead{JobTitle: "CEO"}, &Sender{}, nil, Options{})
	if r.Text != "Hola CEO" {
		t.Errorf("got %q", r.Text)
	}
}

func TestRender_Conditionals_False(t *testing.T) {
	tpl := `{% if jobTitle == "CEO" %}Hola CEO{% else %}Hola{% endif %}`
	r, _ := Render(tpl, &Lead{JobTitle: "CTO"}, &Sender{}, nil, Options{})
	if r.Text != "Hola" {
		t.Errorf("got %q", r.Text)
	}
}

func TestRender_Conditionals_NoElse(t *testing.T) {
	tpl := `{% if jobTitle == "CEO" %}Hola CEO{% endif %}`
	r, _ := Render(tpl, &Lead{JobTitle: "CTO"}, &Sender{}, nil, Options{})
	if r.Text != "" {
		t.Errorf("got %q", r.Text)
	}
}

func TestRender_Conditionals_Contains(t *testing.T) {
	tpl := `{% if headline contains "ingeniero" %}OK{% else %}NO{% endif %}`
	r, _ := Render(tpl, &Lead{Headline: "Senior Ingeniero de Software"}, &Sender{}, nil, Options{})
	if r.Text != "OK" {
		t.Errorf("got %q (contains case-insensitive)", r.Text)
	}
}

func TestRender_Conditionals_AndOr(t *testing.T) {
	tpl := `{% if industry == "tech" or industry == "software" %}T{% else %}X{% endif %}`
	r, _ := Render(tpl, &Lead{Industry: "software"}, &Sender{}, nil, Options{})
	if r.Text != "T" {
		t.Errorf("or: got %q", r.Text)
	}

	tpl2 := `{% if jobTitle == "CEO" and industry == "tech" %}Y{% else %}N{% endif %}`
	r, _ = Render(tpl2, &Lead{JobTitle: "CEO", Industry: "tech"}, &Sender{}, nil, Options{})
	if r.Text != "Y" {
		t.Errorf("and: got %q", r.Text)
	}
	r, _ = Render(tpl2, &Lead{JobTitle: "CEO", Industry: "finance"}, &Sender{}, nil, Options{})
	if r.Text != "N" {
		t.Errorf("and false: got %q", r.Text)
	}
}

func TestRender_NestedConditionals(t *testing.T) {
	tpl := `{% if jobTitle == "CEO" %}{% if industry == "tech" %}TechCEO{% else %}OtherCEO{% endif %}{% else %}Plain{% endif %}`
	r, _ := Render(tpl, &Lead{JobTitle: "CEO", Industry: "tech"}, &Sender{}, nil, Options{})
	if r.Text != "TechCEO" {
		t.Errorf("got %q", r.Text)
	}
	r, _ = Render(tpl, &Lead{JobTitle: "CEO", Industry: "finance"}, &Sender{}, nil, Options{})
	if r.Text != "OtherCEO" {
		t.Errorf("got %q", r.Text)
	}
	r, _ = Render(tpl, &Lead{JobTitle: "VP"}, &Sender{}, nil, Options{})
	if r.Text != "Plain" {
		t.Errorf("got %q", r.Text)
	}
}

func TestRender_SpinBlock(t *testing.T) {
	tpl := `Hola {% spin %}{% variation %}colega{% variation %}amigo{% endspin %}!`
	r, _ := Render(tpl, &Lead{}, &Sender{}, nil, Options{RNG: fixedRNG()})
	// fixedRNG returns 0 → first variation
	if r.Text != "Hola colega!" {
		t.Errorf("got %q", r.Text)
	}
}

func TestRender_LegacySpintax(t *testing.T) {
	r, _ := Render("{Hola|Buenas|Saludos}", &Lead{}, &Sender{}, nil, Options{RNG: fixedRNG()})
	// fixedRNG returns 0 → first option
	if r.Text != "Hola" {
		t.Errorf("got %q", r.Text)
	}
}

func TestRender_Multimedia(t *testing.T) {
	resolver := func(token string) *Attachment {
		if token == "foto1" {
			return &Attachment{URL: "https://x/photo.jpg", Type: "image", FileName: "photo.jpg"}
		}
		return nil
	}
	r, _ := Render("Mira {foto1}", &Lead{}, &Sender{}, resolver, Options{})
	if r.Text != "Mira " {
		t.Errorf("got %q", r.Text)
	}
	if len(r.MediaAttachments) != 1 || r.MediaAttachments[0].URL != "https://x/photo.jpg" {
		t.Errorf("expected 1 attachment, got %v", r.MediaAttachments)
	}
}

func TestRender_MultimediaUnresolved(t *testing.T) {
	r, _ := Render("Mira {foto1}", &Lead{}, &Sender{}, nil, Options{})
	if len(r.HardErrors) != 1 || r.HardErrors[0] != "media_unresolved:foto1" {
		t.Errorf("expected hard error, got %v", r.HardErrors)
	}
}

func TestRender_MultimediaUnresolved_Strict(t *testing.T) {
	_, err := Render("Mira {foto1}", &Lead{}, &Sender{}, nil, Options{Strict: true})
	if err == nil {
		t.Fatal("expected error in strict mode")
	}
	te, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T", err)
	}
	if len(te.HardErrors) != 1 {
		t.Errorf("expected 1 hard error, got %v", te.HardErrors)
	}
}

func TestRender_Escapes(t *testing.T) {
	r, _ := Render(`Literal: \{{var}} y \{token}`, &Lead{}, &Sender{}, nil, Options{})
	if r.Text != "Literal: {{var}} y {token}" {
		t.Errorf("got %q", r.Text)
	}
}

func TestRender_CustomVariables(t *testing.T) {
	lead := &Lead{
		FirstName: "Ana",
		Variables: map[string]any{"mutual_name": "Carlos", "score": 42},
	}
	r, _ := Render("{{firstName}} conoce a {{mutual_name}} ({{score}})", lead, &Sender{}, nil, Options{})
	if r.Text != "Ana conoce a Carlos (42)" {
		t.Errorf("got %q", r.Text)
	}
}

func TestRender_RealisticInvite(t *testing.T) {
	tpl := `Hola {{firstName|amigo|capitalize}}, vi que trabajás en {{company}}. ` +
		`{% if industry == "tech" %}Soy {{sender.name}} y armo bots con IA.{% else %}Trabajo con empresas como la tuya.{% endif %}`
	lead := &Lead{FirstName: "juan", Company: "Acme", Industry: "tech"}
	sender := &Sender{Name: "Mati"}
	r, _ := Render(tpl, lead, sender, nil, Options{})
	want := "Hola Juan, vi que trabajás en Acme. Soy Mati y armo bots con IA."
	if r.Text != want {
		t.Errorf("\n got: %q\nwant: %q", r.Text, want)
	}
}

func TestRender_MissingVarsCollected(t *testing.T) {
	r, _ := Render("{{firstName}} - {{company}} - {{phone}}", &Lead{FirstName: "X"}, &Sender{}, nil, Options{})
	if len(r.MissingVars) != 2 {
		t.Errorf("expected 2 missing, got %v", r.MissingVars)
	}
	got := strings.Join(r.MissingVars, ",")
	if !strings.Contains(got, "company") || !strings.Contains(got, "phone") {
		t.Errorf("got %v", r.MissingVars)
	}
}
