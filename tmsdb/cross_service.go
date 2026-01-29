package tmsdb

import (
	"context"
	"fmt"
	"sync"
)

// CrossServiceResolver - интерфейс для cross-service фильтрации
type CrossServiceResolver interface {
	ResolveIDs(ctx context.Context, filter any) ([]string, error)
}

// CrossServiceFilter - конфигурация
type CrossServiceFilter struct {
	LocalField   string
	Resolver     CrossServiceResolver
	RemoteFilter any
}

// ApplyCrossService применяет cross-service фильтр
func (fb *FilterBuilder) ApplyCrossService(ctx context.Context, csf *CrossServiceFilter) error {
	if csf == nil || csf.Resolver == nil || csf.RemoteFilter == nil {
		return nil
	}

	ids, err := csf.Resolver.ResolveIDs(ctx, csf.RemoteFilter)
	if err != nil {
		return fmt.Errorf("cross-service resolve failed: %w", err)
	}

	fb.InIDs(csf.LocalField, ids)
	return nil
}

// CrossServiceResult - результат batch запроса
type CrossServiceResult struct {
	LocalField string
	IDs        []string
	Error      error
}

// ApplyCrossServiceBatch - параллельные cross-service запросы
func (fb *FilterBuilder) ApplyCrossServiceBatch(ctx context.Context, filters []*CrossServiceFilter) []CrossServiceResult {
	if len(filters) == 0 {
		return nil
	}

	results := make([]CrossServiceResult, len(filters))
	var wg sync.WaitGroup

	for i, csf := range filters {
		if csf == nil || csf.Resolver == nil || csf.RemoteFilter == nil {
			continue
		}

		wg.Add(1)
		go func(idx int, f *CrossServiceFilter) {
			defer wg.Done()
			ids, err := f.Resolver.ResolveIDs(ctx, f.RemoteFilter)
			results[idx] = CrossServiceResult{
				LocalField: f.LocalField,
				IDs:        ids,
				Error:      err,
			}
		}(i, csf)
	}

	wg.Wait()

	for _, r := range results {
		if r.Error == nil && r.LocalField != "" {
			fb.InIDs(r.LocalField, r.IDs)
		}
	}

	return results
}
