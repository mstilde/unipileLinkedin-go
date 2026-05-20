package safety

import "testing"

func TestShortenFullName(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"   ", ""},
		{"Pedro", "Pedro"},
		{"Juan Pérez", "Juan Pérez"},
		{"Juan Carlos Pérez", "Juan Pérez"},
		{"Juan Carlos Pérez Hernández", "Juan Pérez"},
		{"María del Carmen Rodríguez González", "María Rodríguez"},
		{"María de la Vega Rodríguez", "María de la Vega"},
		{"Ana De La Vega", "Ana De La Vega"}, // 4 tokens with particles: surname = Vega; leading particles attached
		{"  Juan   Pérez  ", "Juan Pérez"},   // whitespace collapse
	}
	for _, c := range cases {
		if got := ShortenFullName(c.in); got != c.want {
			t.Errorf("ShortenFullName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIsParticle(t *testing.T) {
	for _, w := range []string{"de", "DE", "del", "la", "LAS", "van", "von", "Mc", "Mac", "y", "e"} {
		if !IsParticle(w) {
			t.Errorf("%q should be particle", w)
		}
	}
	for _, w := range []string{"Pérez", "Rodríguez", "Juan", "González", ""} {
		if IsParticle(w) {
			t.Errorf("%q should NOT be particle", w)
		}
	}
}
