// Package search holds the pieces of the office global search (DEV-2044) that
// every provider needs to agree on: how the user's text is normalized, which
// field kinds are worth searching for that text, and how a hit is scored.
//
// It deliberately contains no SQL and no transport: the catalog of searchable
// entities lives in backend-pkg/searchcatalog, the wire contract in
// backend-pkg/proto/search, and the queries in each owning service.
package search

import (
	"strings"
	"unicode"

	"github.com/TMS360/backend-pkg/searchcatalog"
	"github.com/google/uuid"
)

const (
	// MinQueryLen is the shortest text that is searched at all. One character
	// degrades a pg_trgm GIN index into a sequential scan and matches almost
	// everything, so a shorter query returns empty groups without touching the
	// database. Matches backend-messaging's message search.
	MinQueryLen = 2

	// MaxQueryLen caps the text server-side. A paste of a whole rate
	// confirmation must truncate, never fail the request.
	MaxQueryLen = 80

	// DefaultLimitPerEntity / MaxLimitPerEntity bound the hits per group. The
	// header palette shows a handful of rows per group; the cap keeps a client
	// from turning the search into an export.
	DefaultLimitPerEntity = 5
	MaxLimitPerEntity     = 25

	// MaxGroupTotal caps the reported total. The office needs "more than a
	// screenful", not an exact count, and an exact count over a trigram scan
	// costs as much as the search itself.
	MaxGroupTotal = 100

	// MaxResolvedIDs caps how many ids a cross-service relation resolve may
	// feed into a local `IN (...)`. A very generic query ("a") would otherwise
	// resolve half a tenant's drivers into one query plan.
	MaxResolvedIDs = 200

	// FuzzyThreshold is the pg_trgm word_similarity() floor that makes a
	// one-character typo still find the record ("Marcus" vs "Marcsu"). Chosen
	// against the default 0.6 similarity floor, which is too strict for a
	// single transposition in a short word.
	FuzzyThreshold = 0.4
)

// Query is normalized user text plus what it looks like. Providers use it to
// decide which columns are worth touching: a digits-only query must not drag
// every name and address column through a trigram scan.
type Query struct {
	// Text is the normalized query: trimmed, inner whitespace collapsed, cut
	// to MaxQueryLen runes.
	Text string
	// ID is set when the whole query is a UUID — the office pasted a record id
	// or a link. Providers answer with a direct primary-key lookup.
	ID *uuid.UUID
	// HasAt marks an email-looking query.
	HasAt bool
	// DigitsOnly marks a query that carries digits and separators only, after
	// phone/plate punctuation is ignored.
	DigitsOnly bool
}

// Normalize trims the text, collapses inner whitespace and caps the length.
// Truncation is rune-safe: cutting a multi-byte name in half must not produce
// invalid UTF-8 in a hit label.
func Normalize(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.Join(strings.Fields(s), " ")

	runes := []rune(s)
	if len(runes) > MaxQueryLen {
		s = strings.TrimSpace(string(runes[:MaxQueryLen]))
	}
	return s
}

// Parse normalizes the text and classifies it.
func Parse(s string) Query {
	text := Normalize(s)
	q := Query{Text: text}
	if text == "" {
		return q
	}

	if id, err := uuid.Parse(text); err == nil {
		q.ID = &id
		return q
	}

	q.HasAt = strings.ContainsRune(text, '@')

	hasDigit, hasOther := false, false
	for _, r := range text {
		switch {
		case unicode.IsDigit(r):
			hasDigit = true
		case r == '-' || r == '+' || r == '(' || r == ')' || r == ' ' || r == '.' || r == '#':
			// Separators people type inside phone numbers, plates and load
			// numbers. Neither digits nor letters for classification.
		default:
			hasOther = true
		}
	}
	q.DigitsOnly = hasDigit && !hasOther

	return q
}

// Valid reports whether the query is worth a database round trip.
func (q Query) Valid() bool {
	return q.ID != nil || len([]rune(q.Text)) >= MinQueryLen
}

// Wants reports whether a field of this kind is worth searching for this
// query. It is a cost guard, not a correctness one: a provider that ignores it
// returns the same rows, only slower.
func (q Query) Wants(kind searchcatalog.FieldKind) bool {
	switch {
	case q.ID != nil:
		// A pasted id is answered by primary key; no text column applies.
		return false
	case q.HasAt:
		return kind == searchcatalog.KindEmail
	case q.DigitsOnly:
		// Numbers, VIN/plate tails and phone numbers. Names, statuses and
		// addresses cannot match digits.
		return kind == searchcatalog.KindNumber ||
			kind == searchcatalog.KindCode ||
			kind == searchcatalog.KindPhone
	default:
		// Letters are present. load_id / reference_numbers / PRO are varchar
		// and routinely carry letters, so KindNumber stays in.
		return kind == searchcatalog.KindText ||
			kind == searchcatalog.KindStatus ||
			kind == searchcatalog.KindCode ||
			kind == searchcatalog.KindNumber
	}
}

// LimitPerEntity clamps a client-supplied limit into the allowed range.
func LimitPerEntity(requested int) int {
	switch {
	case requested <= 0:
		return DefaultLimitPerEntity
	case requested > MaxLimitPerEntity:
		return MaxLimitPerEntity
	default:
		return requested
	}
}

// CapTotal clamps a counted total to MaxGroupTotal and reports whether the cap
// was hit, which is what a group's hasMore is built from.
func CapTotal(total int) (capped int, more bool) {
	if total > MaxGroupTotal {
		return MaxGroupTotal, true
	}
	return total, false
}
