package savedfilter

import (
	"context"
	"encoding/json"
)

// CountFunc is a callback that counts entities matching the given filter.
// Each service registers its own implementation per entity type.
type CountFunc func(ctx context.Context, filter json.RawMessage) (int64, error)

type CreateInput struct {
	Name       string
	EntityType string
	Filter     json.RawMessage
	IsDefault  *bool
}

type UpdateInput struct {
	Name      *string
	Filter    *json.RawMessage
	IsDefault *bool
}

type SavedFilterWithCount struct {
	*SavedFilter
	Count int64
}
