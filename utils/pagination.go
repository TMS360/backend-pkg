package utils

import (
	"math"

	"gorm.io/gorm"
)

// Pagination хранит данные для ответа
type Pagination struct {
	Page       int32 `json:"page"`
	Limit      int32 `json:"limit"`
	Total      int64 `json:"total"`
	TotalPages int32 `json:"totalPages"`
}

// PaginateScope - это функция для GORM, которая применяет limit/offset
func PaginateScope(page, limit int32) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if page <= 0 {
			page = 1
		}
		switch {
		case limit > 100:
			limit = 100 // Защита от слишком больших запросов
		case limit <= 0:
			limit = 10
		}

		offset := (page - 1) * limit
		return db.Offset(int(offset)).Limit(int(limit))
	}
}

// CalculatePagination считает TotalPages на основе Total count
func CalculatePagination(total int64, page, limit int32) Pagination {
	if limit <= 0 {
		limit = 10
	}
	if page <= 0 {
		page = 1
	}

	totalPages := int32(math.Ceil(float64(total) / float64(limit)))

	return Pagination{
		Page:       page,
		Limit:      limit,
		Total:      total,
		TotalPages: totalPages,
	}
}
