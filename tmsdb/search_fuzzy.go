package tmsdb

import "gorm.io/gorm"

// SearchFuzzy matches term against every column with a substring test OR a
// trigram word-similarity test:
//
//	col ILIKE '%term%' OR col %> 'term'
//
// The similarity half is what makes a one-character typo still find the record
// ("Marcsu" → "Marcus"), which plain Search cannot do.
//
// `%>` is the OPERATOR form on purpose. The function form,
// `word_similarity(term, col) > 0.4`, can never use an index — Postgres
// evaluates it per row, so on a large table it degrades the whole query into a
// sequential scan even when a GIN trigram index is sitting right there
// (measured: 6.3s vs 30ms on 2M rows). The operator is index-supported, and
// both halves of the predicate above resolve to a BitmapOr of two index scans
// on the same `GIN (col gin_trgm_ops)` index.
//
// The threshold is NOT inline: `%>` reads pg_trgm.word_similarity_threshold,
// which each search-owning database sets to 0.4 (see the DEV-2044 migrations).
// If a database is missing that setting the operator still works at Postgres's
// default 0.6 — the search stays correct and indexed, just less forgiving of
// typos.
//
// term is a pointer to mirror Search: nil or empty is a no-op, so a caller can
// pass an optional filter field straight through. Columns may be expressions
// (e.g. "shipment_number::TEXT"), but only a plain column or an indexed
// expression can be served by an index.
func (fb *FilterBuilder) SearchFuzzy(term *string, columns ...string) *FilterBuilder {
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
		query = query.Or(col+" %> ?", *term)
	}

	fb.db = fb.db.Where(query)
	return fb
}

// CountUpTo counts matching rows but stops at cap, and reports whether there
// were more. It is the counterpart to Count for search-style reads.
//
// Count runs an unbounded COUNT(*): on a query that matches 400k rows it reads
// all of them just to render "Loads (400000)" — 2.1s per keystroke, measured.
// CountUpTo wraps the same predicate in `SELECT count(*) FROM (… LIMIT cap+1)`,
// which stops as soon as the cap is reached: 2.5ms for the same query. The
// client shows an exact number up to cap and "more than cap" beyond it, which
// is all a search UI can use anyway.
//
// cap <= 0 counts nothing and reports no more.
func (fb *FilterBuilder) CountUpTo(cap int) (count int64, more bool, err error) {
	if cap <= 0 {
		return 0, false, nil
	}

	capped := fb.db.Session(&gorm.Session{}).
		Limit(cap + 1).
		Offset(-1).
		Select("1")

	var n int64
	if err := fb.db.Session(&gorm.Session{NewDB: true}).
		Table("(?) AS capped_rows", capped).
		Count(&n).Error; err != nil {
		return 0, false, err
	}

	if n > int64(cap) {
		return int64(cap), true, nil
	}
	return n, false, nil
}
