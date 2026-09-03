package search

import (
	"strings"

	searchpb "github.com/TMS360/backend-pkg/proto/search"
	"github.com/TMS360/backend-pkg/searchcatalog"
)

// Score bands for a match on the entity's OWN field. A hit's score is the best
// band any of its matched fields reaches, so a load whose own number is typed
// exactly always outranks a load found through its driver's name.
const (
	ScoreExact     = 100.0
	ScorePrefix    = 80.0
	ScoreSubstring = 60.0
	ScoreFuzzy     = 25.0

	// RelationFactor discounts a match that happened on a related record: the
	// office typed a driver's name, so the driver is the better answer and the
	// loads under that driver come after it.
	RelationFactor = 0.5
)

// ScoreValue grades one matched value against the query. relation says the
// value came from a related record rather than the entity itself.
//
// Comparison is case-insensitive, and the query's own separators are kept:
// "1043" against truck number "1043" is exact, "104" is a prefix, and "043" a
// substring. Anything the SQL matched but the text comparison cannot explain
// was a trigram (fuzzy) match.
func ScoreValue(q Query, value string, relation bool) float64 {
	base := ScoreFuzzy

	v := strings.ToLower(strings.TrimSpace(value))
	t := strings.ToLower(q.Text)
	switch {
	case v == "" || t == "":
		base = ScoreFuzzy
	case v == t:
		base = ScoreExact
	case strings.HasPrefix(v, t):
		base = ScorePrefix
	case strings.Contains(v, t):
		base = ScoreSubstring
	}

	if relation {
		base *= RelationFactor
	}
	return base
}

// Match is a scored field match on its way into a hit.
type Match struct {
	Path  string
	Label string
	Value string
	Score float64
}

// Matcher grades the fields of one entity for one query and keeps the matches
// worth showing. It exists so every provider builds SearchMatch lists the same
// way instead of hand-rolling labels and ordering.
type Matcher struct {
	entity searchcatalog.Entity
	query  Query
}

// NewMatcher returns a Matcher for an entity code from the shared catalog.
func NewMatcher(entityCode string, q Query) (*Matcher, bool) {
	e, ok := searchcatalog.ByCode(entityCode)
	if !ok {
		return nil, false
	}
	return &Matcher{entity: e, query: q}, true
}

// Add grades one non-empty value for a catalog path and returns the match, or
// nil when the value is empty or the path is not in the catalog. A value the
// text comparison cannot explain (the SQL matched it by trigram) still scores,
// at the ScoreFuzzy band.
//
// A path that is not in the catalog is a programming error in the caller, and
// silently dropping it is the safe behaviour: the row still comes back, just
// without that chip.
func (m *Matcher) Add(path, value string) *Match {
	if value == "" {
		return nil
	}
	label, ok := m.entity.LabelForPath(path)
	if !ok {
		return nil
	}
	return &Match{
		Path:  path,
		Label: label,
		Value: value,
		Score: ScoreValue(m.query, value, m.isRelation(path)),
	}
}

func (m *Matcher) isRelation(path string) bool {
	for _, r := range m.entity.Relations {
		if r.Path == path {
			return true
		}
	}
	return false
}

// Best returns the score of a hit built from these matches: the highest single
// match, since one exact number is a better answer than three vague ones.
func Best(matches []*Match) float64 {
	best := 0.0
	for _, mt := range matches {
		if mt != nil && mt.Score > best {
			best = mt.Score
		}
	}
	return best
}

// ToProto converts matches into the wire type, dropping nils and ordering the
// strongest match first so the client can show one chip and be right.
func ToProto(matches []*Match) []*searchpb.SearchMatch {
	kept := make([]*Match, 0, len(matches))
	for _, mt := range matches {
		if mt != nil {
			kept = append(kept, mt)
		}
	}
	// Insertion sort: a hit carries a handful of matches, never enough for
	// sort.Slice to earn its allocation. Stable, so equally scored matches
	// keep the order the caller added them in (catalog order).
	for i := 1; i < len(kept); i++ {
		for j := i; j > 0 && kept[j].Score > kept[j-1].Score; j-- {
			kept[j], kept[j-1] = kept[j-1], kept[j]
		}
	}

	out := make([]*searchpb.SearchMatch, 0, len(kept))
	for _, mt := range kept {
		out = append(out, &searchpb.SearchMatch{
			Field: mt.Path,
			Label: mt.Label,
			Value: mt.Value,
		})
	}
	return out
}
