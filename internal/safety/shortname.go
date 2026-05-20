package safety

import "strings"

// surnameParticles are words that typically PRECEDE a surname rather than
// being one by themselves. Used by ShortenFullName to attach prefixes like
// "de la" or "van" to the surname they belong to.
var surnameParticles = map[string]struct{}{
	"de": {}, "del": {}, "la": {}, "las": {}, "los": {},
	"van": {}, "von": {}, "da": {}, "das": {}, "do": {}, "dos": {},
	"di": {}, "du": {}, "al": {}, "el": {}, "le": {},
	"mc": {}, "mac": {}, "o'": {}, "st": {}, "st.": {},
	"san": {}, "santa": {}, "y": {}, "e": {},
}

// IsParticle reports whether word is a known surname particle. Case- and
// punctuation-insensitive (strips '.' and ',').
func IsParticle(word string) bool {
	if word == "" {
		return false
	}
	w := strings.ToLower(word)
	w = strings.ReplaceAll(w, ".", "")
	w = strings.ReplaceAll(w, ",", "")
	_, ok := surnameParticles[w]
	return ok
}

// ShortenFullName collapses a full name to "FirstName FirstSurname".
// Rules (pragmatic for Latin-American naming conventions):
//
//   - "Juan Pérez"                       → "Juan Pérez"           (2 tokens, unchanged)
//   - "Juan Carlos Pérez"                → "Juan Pérez"           (3 tokens: first + last)
//   - "Juan Carlos Pérez Hernández"      → "Juan Pérez"           (4 tokens: first + second-to-last)
//   - "María del Carmen Rodríguez González" → "María Rodríguez"
//   - "Ana De La Vega"                   → "Ana De La Vega"       (particle inside 3-token name → unchanged)
//   - "María de la Vega Rodríguez"       → "María de la Vega"     (particle block attaches to surname)
//   - "Pedro"                            → "Pedro"                (1 token)
//   - ""                                 → ""
func ShortenFullName(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return ""
	}
	// Collapse whitespace
	tokens := strings.Fields(trimmed)
	switch len(tokens) {
	case 0:
		return ""
	case 1:
		return tokens[0]
	case 2:
		return tokens[0] + " " + tokens[1]
	case 3:
		// Ambiguous: could be "First Middle Last" OR "First Surname1 Surname2".
		// If the middle token is a particle, treat as compound surname → keep all 3.
		if IsParticle(tokens[1]) {
			return strings.Join(tokens, " ")
		}
		return tokens[0] + " " + tokens[2]
	}

	// 4+ tokens: Spanish convention is two surnames at the end (paternal then
	// maternal). The "first surname" is at position len-2. If that position is a
	// particle, walk backward until we find a non-particle.
	firstName := tokens[0]
	surnameIdx := len(tokens) - 2
	for surnameIdx > 1 && IsParticle(tokens[surnameIdx]) {
		surnameIdx--
	}

	// Edge case: "Ana De La Vega" — after the loop surnameIdx=1 but tokens[1]
	// is still a particle. The whole tail from index 1 is the compound surname.
	if surnameIdx == 1 && IsParticle(tokens[1]) {
		return firstName + " " + strings.Join(tokens[1:], " ")
	}

	// Attach any leading particles (e.g. "de la Vega"): walk backward from
	// surnameIdx-1 while we see particles, but stop before the first name.
	prefix := ""
	for i := surnameIdx - 1; i > 0 && IsParticle(tokens[i]); i-- {
		prefix = tokens[i] + " " + prefix
	}

	surname := strings.TrimSpace(prefix + tokens[surnameIdx])
	return firstName + " " + surname
}
