package tmsdb

import "gorm.io/gorm"

// SearchFuzzy matches term against every column with a substring test AND a
// trigram-similarity fallback, OR-ed together:
//
//	col ILIKE '%term%' OR word_similarity('term', col) > threshold
//
// The similarity half is what makes a one-character typo still find the record
// ("Marcsu" → "Marcus"), which plain Search cannot do. Both halves are served
// by the same `GIN (col gin_trgm_ops)` index, so a column worth passing here is
// a column worth indexing — an unindexed one degrades to a sequential scan.
//
// word_similarity (not similarity) is deliberate: it scores the query against
// the best-matching WORD of the column, so "hale" still matches the full name
// "Marcus Hale", where whole-string similarity would score far below any
// usable threshold.
//
// term is a pointer to mirror Search: nil or empty is a no-op, so a caller can
// pass an optional filter field straight through. Columns may be expressions
// (e.g. "shipment_number::TEXT").
func (fb *FilterBuilder) SearchFuzzy(term *string, threshold float64, columns ...string) *FilterBuilder {
	if term == nil || *term == "" || len(columns) == 0 {
		return fb
	}

	pattern := "%" + *term + "%"
	query := fb.db.Session(&gorm.Session{NewDB: true})

	for i, col := range columns {
		if i == 0 {
			query = query.Where(col+" ILIKE ?", pattern)
		} else {
			query = query.Or(col+" ILIKE ?", pattern)
		}
		query = query.Or("word_similarity(?, "+col+") > ?", *term, threshold)
	}

	fb.db = fb.db.Where(query)
	return fb
}
