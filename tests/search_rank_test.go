package tests

import (
	"testing"

	"github.com/stretchr/testify/require"

	searchpb "github.com/TMS360/backend-pkg/proto/search"
	"github.com/TMS360/backend-pkg/search"
)

// The decline this exists for: a full VIN scores ScoreExact, sixteen trigram
// matches score ScoreFuzzy, and the page was cut before anything was scored.
func TestRankHits_ExactMatchComesFirst(t *testing.T) {
	hits := make([]*searchpb.SearchHit, 0, 17)
	for i := 0; i < 16; i++ {
		hits = append(hits, &searchpb.SearchHit{Id: "fuzzy", Score: search.ScoreFuzzy})
	}
	hits = append(hits, &searchpb.SearchHit{Id: "exact", Score: search.ScoreExact})

	got := search.RankHits(hits, 5)

	require.Len(t, got, 5)
	require.Equal(t, "exact", got[0].GetId())
}

func TestRankHits_BandOrder(t *testing.T) {
	hits := []*searchpb.SearchHit{
		{Id: "fuzzy", Score: search.ScoreFuzzy},
		{Id: "prefix", Score: search.ScorePrefix},
		{Id: "exact", Score: search.ScoreExact},
		{Id: "substring", Score: search.ScoreSubstring},
	}

	got := search.RankHits(hits, 0)

	require.Equal(t, []string{"exact", "prefix", "substring", "fuzzy"},
		[]string{got[0].GetId(), got[1].GetId(), got[2].GetId(), got[3].GetId()})
}

// Equal scores must keep the query's own ORDER BY, or a stable list of trucks
// would reshuffle between identical searches.
func TestRank_EqualScoresKeepLoadedOrder(t *testing.T) {
	type row struct{ number string }
	rows := []row{{"1041"}, {"1042"}, {"1043"}}

	got := search.Rank(rows, func(row) float64 { return search.ScoreFuzzy }, 0)

	require.Equal(t, []row{{"1041"}, {"1042"}, {"1043"}}, got)
}

func TestRank_LimitZeroKeepsEverything(t *testing.T) {
	rows := []int{1, 2, 3}

	got := search.Rank(rows, func(int) float64 { return 1 }, 0)

	require.Len(t, got, 3)
}

func TestRank_LimitLargerThanInput(t *testing.T) {
	rows := []int{1, 2}

	got := search.Rank(rows, func(int) float64 { return 1 }, 10)

	require.Len(t, got, 2)
}

// A relation match is discounted by RelationFactor, so the entity that owns the
// typed text must outrank the rows hanging off it.
func TestRankHits_OwnFieldBeatsRelation(t *testing.T) {
	hits := []*searchpb.SearchHit{
		{Id: "load-under-driver", Score: search.ScoreExact * search.RelationFactor},
		{Id: "driver", Score: search.ScoreExact},
	}

	got := search.RankHits(hits, 0)

	require.Equal(t, "driver", got[0].GetId())
}

func TestRankWindow_CoversTheWholeCandidateSet(t *testing.T) {
	require.Equal(t, search.MaxGroupTotal, search.RankWindow)
	require.GreaterOrEqual(t, search.RankWindow, search.MaxLimitPerEntity)
}
