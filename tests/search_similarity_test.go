package tests

import (
	"testing"

	"github.com/TMS360/backend-pkg/search"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The QA decline this exists for: searching "MAP TRANST" (one letter off) gave
// every trigram match the same ScoreFuzzy, so the page kept the query's
// alphabetical order and MAP TRANSIT LLC was not on it.
func TestScoreValue_TypoRanksTheClosestValueFirst(t *testing.T) {
	q := search.Parse("MAP TRANST")

	wanted := search.ScoreValue(q, "MAP TRANSIT LLC", false)
	partial := search.ScoreValue(q, "TRANS EXPRESS INC", false)
	distant := search.ScoreValue(q, "MAPLE LEAF CARRIERS", false)

	assert.Greater(t, wanted, partial, "the name the office was typing comes first")
	assert.Greater(t, partial, distant)
	assert.LessOrEqual(t, wanted, search.ScoreFuzzy, "a typo never reaches an explainable band")
}

// A shorter prefix of the same name is an explainable match and must keep
// beating any typo — the bands still rank above the graded typo band.
func TestScoreValue_BandsOutrankEveryTypo(t *testing.T) {
	name := "MAP TRANSIT LLC"

	exact := search.ScoreValue(search.Parse(name), name, false)
	prefix := search.ScoreValue(search.Parse("MAP TRANSI"), name, false)
	substring := search.ScoreValue(search.Parse("TRANSIT"), name, false)
	typo := search.ScoreValue(search.Parse("MAP TRANST"), name, false)

	assert.Equal(t, search.ScoreExact, exact)
	assert.Equal(t, search.ScorePrefix, prefix)
	assert.Equal(t, search.ScoreSubstring, substring)
	assert.Less(t, typo, search.ScoreSubstring)
	assert.Greater(t, typo, 0.0, "a close typo still scores")
}

// A relation match stays discounted, graded band included.
func TestScoreValue_TypoOnARelationStaysDiscounted(t *testing.T) {
	q := search.Parse("MAP TRANST")

	own := search.ScoreValue(q, "MAP TRANSIT LLC", false)
	related := search.ScoreValue(q, "MAP TRANSIT LLC", true)

	assert.InDelta(t, own*search.RelationFactor, related, 0.0001)
}

func TestScoreValue_NothingInCommonScoresZero(t *testing.T) {
	q := search.Parse("MAP TRANST")

	assert.Zero(t, search.ScoreValue(q, "ZQX", false))
	assert.Zero(t, search.ScoreValue(q, "", false))
}

func TestSimilarity(t *testing.T) {
	t.Run("identical text covers the whole query", func(t *testing.T) {
		assert.Equal(t, 1.0, search.Similarity("acme", "acme"))
	})

	t.Run("a longer value is not punished for its extra words", func(t *testing.T) {
		// The symmetric pg_trgm similarity() would drop here; coverage of the
		// query is what matters, since the office types less than the record
		// holds.
		assert.Equal(t, 1.0, search.Similarity("map transit", "map transit llc logistics"))
	})

	t.Run("case and punctuation are word breaks, not content", func(t *testing.T) {
		assert.Equal(t, 1.0, search.Similarity("ACME, INC.", "acme inc"))
	})

	t.Run("an empty side scores zero", func(t *testing.T) {
		assert.Zero(t, search.Similarity("", "acme"))
		assert.Zero(t, search.Similarity("acme", ""))
		assert.Zero(t, search.Similarity("...", "acme"))
	})

	t.Run("a one-letter typo stays well above an unrelated name", func(t *testing.T) {
		close := search.Similarity("map transt", "map transit llc")
		far := search.Similarity("map transt", "acme logistics")

		require.Greater(t, close, 0.7)
		assert.Zero(t, far)
	})

	t.Run("digits are word characters", func(t *testing.T) {
		assert.Greater(t, search.Similarity("1043", "1043a"), 0.5)
	})
}
