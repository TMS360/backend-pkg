package search

import "strings"

// Similarity reports how much of the query the value covers, on the same
// trigram measure Postgres used to find the row: 0 when nothing matches, 1 when
// the value contains every trigram the query is made of.
//
// It exists to order rows INSIDE the typo band. Postgres finds them with `%>`
// (pg_trgm word similarity) but the score is computed in Go, and until DEV-111
// every trigram match scored the same — so a search for "MAP TRANST" listed
// every vaguely similar customer alphabetically and left MAP TRANSIT LLC off
// the page.
//
// The measure is coverage of the QUERY's trigrams, not the symmetric
// pg_trgm similarity(), on purpose: a company name is nearly always longer than
// what the office types, and the symmetric ratio would punish "MAP TRANSIT LLC"
// for the words the typist did not bother with. This mirrors what
// word_similarity does — score the best-matching part of the value, not the
// whole string.
//
// It is an ordering signal, never a gate: the threshold that decides whether a
// row matches at all stays in the database (FuzzyThreshold).
func Similarity(query, value string) float64 {
	q := trigrams(query)
	if len(q) == 0 {
		return 0
	}
	v := trigrams(value)
	if len(v) == 0 {
		return 0
	}

	shared := 0
	for t := range q {
		if _, ok := v[t]; ok {
			shared++
		}
	}
	return float64(shared) / float64(len(q))
}

// trigrams splits text the way pg_trgm does: lowercased, every non-alphanumeric
// run treated as a word break, and each word padded with two leading spaces and
// one trailing space so its start and end carry weight ("map" contributes
// "  m", " ma", "map" and "ap ").
func trigrams(s string) map[string]struct{} {
	out := make(map[string]struct{})
	for _, word := range strings.FieldsFunc(strings.ToLower(s), isWordBreak) {
		padded := "  " + word + " "
		r := []rune(padded)
		for i := 0; i+3 <= len(r); i++ {
			out[string(r[i:i+3])] = struct{}{}
		}
	}
	return out
}

func isWordBreak(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z':
		return false
	case r >= '0' && r <= '9':
		return false
	case r > 127:
		// Keep non-ASCII letters (accented names) inside the word; Postgres
		// treats them as word characters too.
		return false
	default:
		return true
	}
}
