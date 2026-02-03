package tmsdb

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// ============================================================================
// ENUMS
// ============================================================================

type QueryMode string

const (
	QueryModeDefault     QueryMode = "default"
	QueryModeInsensitive QueryMode = "insensitive"
)

type SortOrder string

const (
	SortOrderAsc  SortOrder = "asc"
	SortOrderDesc SortOrder = "desc"
)

// ============================================================================
// FILTER TYPES
// ============================================================================

type StringFilter struct {
	Equals     *string   `json:"equals,omitempty"`
	Not        *string   `json:"not,omitempty"`
	In         []string  `json:"in,omitempty"`
	NotIn      []string  `json:"notIn,omitempty"`
	Contains   *string   `json:"contains,omitempty"`
	StartsWith *string   `json:"startsWith,omitempty"`
	EndsWith   *string   `json:"endsWith,omitempty"`
	Like       *string   `json:"like,omitempty"`
	Regex      *string   `json:"regex,omitempty"`
	Mode       QueryMode `json:"mode,omitempty"`
	IsNull     *bool     `json:"isNull,omitempty"`
}

type IntFilter struct {
	Equals *int  `json:"equals,omitempty"`
	Not    *int  `json:"not,omitempty"`
	In     []int `json:"in,omitempty"`
	NotIn  []int `json:"notIn,omitempty"`
	Lt     *int  `json:"lt,omitempty"`
	Lte    *int  `json:"lte,omitempty"`
	Gt     *int  `json:"gt,omitempty"`
	Gte    *int  `json:"gte,omitempty"`
	IsNull *bool `json:"isNull,omitempty"`
}

type FloatFilter struct {
	Equals *float64  `json:"equals,omitempty"`
	Not    *float64  `json:"not,omitempty"`
	In     []float64 `json:"in,omitempty"`
	NotIn  []float64 `json:"notIn,omitempty"`
	Lt     *float64  `json:"lt,omitempty"`
	Lte    *float64  `json:"lte,omitempty"`
	Gt     *float64  `json:"gt,omitempty"`
	Gte    *float64  `json:"gte,omitempty"`
	IsNull *bool     `json:"isNull,omitempty"`
}

type BoolFilter struct {
	Equals *bool `json:"equals,omitempty"`
	IsNull *bool `json:"isNull,omitempty"`
}

type DateTimeFilter struct {
	Equals *time.Time  `json:"equals,omitempty"`
	Not    *time.Time  `json:"not,omitempty"`
	In     []time.Time `json:"in,omitempty"`
	NotIn  []time.Time `json:"notIn,omitempty"`
	Lt     *time.Time  `json:"lt,omitempty"`
	Lte    *time.Time  `json:"lte,omitempty"`
	Gt     *time.Time  `json:"gt,omitempty"`
	Gte    *time.Time  `json:"gte,omitempty"`
	IsNull *bool       `json:"isNull,omitempty"`
}

type IDFilter struct {
	Equals *string  `json:"equals,omitempty"`
	Not    *string  `json:"not,omitempty"`
	In     []string `json:"in,omitempty"`
	NotIn  []string `json:"notIn,omitempty"`
	IsNull *bool    `json:"isNull,omitempty"`
}

type UUIDFilter = IDFilter

type JSONFilter struct {
	Equals      any      `json:"equals,omitempty"`
	Contains    any      `json:"contains,omitempty"`
	ContainedBy any      `json:"containedBy,omitempty"`
	HasKey      *string  `json:"hasKey,omitempty"`
	HasKeys     []string `json:"hasKeys,omitempty"`
	HasAnyKey   []string `json:"hasAnyKey,omitempty"`
	IsNull      *bool    `json:"isNull,omitempty"`
}

// ============================================================================
// PAGINATION
// ============================================================================

type PaginationInput struct {
	Page  int32 `json:"page"`
	Limit int32 `json:"limit"`
}

func (p *PaginationInput) GetOffset() int {
	if p == nil || p.Page <= 1 {
		return 0
	}
	return int(p.Page-1) * p.GetLimit()
}

func (p *PaginationInput) GetLimit() int {
	if p == nil || p.Limit <= 0 {
		return 20
	}
	return int(p.Limit)
}

func (p *PaginationInput) GetPage() int {
	if p == nil || p.Page <= 0 {
		return 1
	}
	return int(p.Page)
}

type Pagination struct {
	Page       int32 `json:"page"`
	Limit      int32 `json:"limit"`
	Total      int32 `json:"total"`
	TotalPages int32 `json:"totalPages"`
}

func NewPagination(input *PaginationInput, total int64) *Pagination {
	var page int32 = 1
	var limit int32 = 20

	if input != nil {
		if input.Page > 0 {
			page = input.Page
		}
		if input.Limit > 0 {
			limit = input.Limit
		}
	}

	totalInt := int32(total)
	totalPages := totalInt / limit
	if totalInt%limit > 0 {
		totalPages++
	}
	if totalPages == 0 {
		totalPages = 1
	}

	return &Pagination{
		Page:       page,
		Limit:      limit,
		Total:      totalInt,
		TotalPages: totalPages,
	}
}

// ============================================================================
// FILTER BUILDER
// ============================================================================

type FilterBuilder struct {
	db       *gorm.DB
	model    interface{}
	maxLimit int
}

func newFilterBuilder(db *gorm.DB, model interface{}) *FilterBuilder {
	return &FilterBuilder{
		db:       db.Model(model),
		model:    model,
		maxLimit: 100,
	}
}

func (fb *FilterBuilder) DB() *gorm.DB {
	return fb.db
}

func (fb *FilterBuilder) SetMaxLimit(max int) *FilterBuilder {
	fb.maxLimit = max
	return fb
}

// String применяет StringFilter
func (fb *FilterBuilder) String(col string, f *StringFilter) *FilterBuilder {
	if f == nil {
		return fb
	}

	like, regex := "LIKE", "~"
	if f.Mode == QueryModeInsensitive {
		like, regex = "ILIKE", "~*"
	}

	if f.Equals != nil {
		fb.db = fb.db.Where(col+" = ?", *f.Equals)
	}
	if f.Not != nil {
		fb.db = fb.db.Where(col+" != ?", *f.Not)
	}
	if len(f.In) > 0 {
		fb.db = fb.db.Where(col+" IN ?", f.In)
	}
	if len(f.NotIn) > 0 {
		fb.db = fb.db.Where(col+" NOT IN ?", f.NotIn)
	}
	if f.Contains != nil {
		fb.db = fb.db.Where(col+" "+like+" ?", "%"+*f.Contains+"%")
	}
	if f.StartsWith != nil {
		fb.db = fb.db.Where(col+" "+like+" ?", *f.StartsWith+"%")
	}
	if f.EndsWith != nil {
		fb.db = fb.db.Where(col+" "+like+" ?", "%"+*f.EndsWith)
	}
	if f.Like != nil {
		fb.db = fb.db.Where(col+" "+like+" ?", *f.Like)
	}
	if f.Regex != nil {
		fb.db = fb.db.Where(col+" "+regex+" ?", *f.Regex)
	}
	if f.IsNull != nil {
		if *f.IsNull {
			fb.db = fb.db.Where(col + " IS NULL")
		} else {
			fb.db = fb.db.Where(col + " IS NOT NULL")
		}
	}
	return fb
}

// Int применяет IntFilter
func (fb *FilterBuilder) Int(col string, f *IntFilter) *FilterBuilder {
	if f == nil {
		return fb
	}
	if f.Equals != nil {
		fb.db = fb.db.Where(col+" = ?", *f.Equals)
	}
	if f.Not != nil {
		fb.db = fb.db.Where(col+" != ?", *f.Not)
	}
	if f.Lt != nil {
		fb.db = fb.db.Where(col+" < ?", *f.Lt)
	}
	if f.Lte != nil {
		fb.db = fb.db.Where(col+" <= ?", *f.Lte)
	}
	if f.Gt != nil {
		fb.db = fb.db.Where(col+" > ?", *f.Gt)
	}
	if f.Gte != nil {
		fb.db = fb.db.Where(col+" >= ?", *f.Gte)
	}
	if len(f.In) > 0 {
		fb.db = fb.db.Where(col+" IN ?", f.In)
	}
	if len(f.NotIn) > 0 {
		fb.db = fb.db.Where(col+" NOT IN ?", f.NotIn)
	}
	if f.IsNull != nil {
		if *f.IsNull {
			fb.db = fb.db.Where(col + " IS NULL")
		} else {
			fb.db = fb.db.Where(col + " IS NOT NULL")
		}
	}
	return fb
}

// Float применяет FloatFilter
func (fb *FilterBuilder) Float(col string, f *FloatFilter) *FilterBuilder {
	if f == nil {
		return fb
	}
	if f.Equals != nil {
		fb.db = fb.db.Where(col+" = ?", *f.Equals)
	}
	if f.Not != nil {
		fb.db = fb.db.Where(col+" != ?", *f.Not)
	}
	if f.Lt != nil {
		fb.db = fb.db.Where(col+" < ?", *f.Lt)
	}
	if f.Lte != nil {
		fb.db = fb.db.Where(col+" <= ?", *f.Lte)
	}
	if f.Gt != nil {
		fb.db = fb.db.Where(col+" > ?", *f.Gt)
	}
	if f.Gte != nil {
		fb.db = fb.db.Where(col+" >= ?", *f.Gte)
	}
	if len(f.In) > 0 {
		fb.db = fb.db.Where(col+" IN ?", f.In)
	}
	if len(f.NotIn) > 0 {
		fb.db = fb.db.Where(col+" NOT IN ?", f.NotIn)
	}
	if f.IsNull != nil {
		if *f.IsNull {
			fb.db = fb.db.Where(col + " IS NULL")
		} else {
			fb.db = fb.db.Where(col + " IS NOT NULL")
		}
	}
	return fb
}

// Bool применяет BoolFilter
func (fb *FilterBuilder) Bool(col string, f *BoolFilter) *FilterBuilder {
	if f == nil {
		return fb
	}
	if f.Equals != nil {
		fb.db = fb.db.Where(col+" = ?", *f.Equals)
	}
	if f.IsNull != nil {
		if *f.IsNull {
			fb.db = fb.db.Where(col + " IS NULL")
		} else {
			fb.db = fb.db.Where(col + " IS NOT NULL")
		}
	}
	return fb
}

// DateTime применяет DateTimeFilter
func (fb *FilterBuilder) DateTime(col string, f *DateTimeFilter) *FilterBuilder {
	if f == nil {
		return fb
	}
	if f.Equals != nil {
		fb.db = fb.db.Where(col+" = ?", *f.Equals)
	}
	if f.Not != nil {
		fb.db = fb.db.Where(col+" != ?", *f.Not)
	}
	if f.Lt != nil {
		fb.db = fb.db.Where(col+" < ?", *f.Lt)
	}
	if f.Lte != nil {
		fb.db = fb.db.Where(col+" <= ?", *f.Lte)
	}
	if f.Gt != nil {
		fb.db = fb.db.Where(col+" > ?", *f.Gt)
	}
	if f.Gte != nil {
		fb.db = fb.db.Where(col+" >= ?", *f.Gte)
	}
	if len(f.In) > 0 {
		fb.db = fb.db.Where(col+" IN ?", f.In)
	}
	if len(f.NotIn) > 0 {
		fb.db = fb.db.Where(col+" NOT IN ?", f.NotIn)
	}
	if f.IsNull != nil {
		if *f.IsNull {
			fb.db = fb.db.Where(col + " IS NULL")
		} else {
			fb.db = fb.db.Where(col + " IS NOT NULL")
		}
	}
	return fb
}

// ID применяет IDFilter
func (fb *FilterBuilder) ID(col string, f *IDFilter) *FilterBuilder {
	if f == nil {
		return fb
	}
	if f.Equals != nil {
		fb.db = fb.db.Where(col+" = ?", *f.Equals)
	}
	if f.Not != nil {
		fb.db = fb.db.Where(col+" != ?", *f.Not)
	}
	if len(f.In) > 0 {
		fb.db = fb.db.Where(col+" IN ?", f.In)
	}
	if len(f.NotIn) > 0 {
		fb.db = fb.db.Where(col+" NOT IN ?", f.NotIn)
	}
	if f.IsNull != nil {
		if *f.IsNull {
			fb.db = fb.db.Where(col + " IS NULL")
		} else {
			fb.db = fb.db.Where(col + " IS NOT NULL")
		}
	}
	return fb
}

// UUID алиас для ID
func (fb *FilterBuilder) UUID(col string, f *UUIDFilter) *FilterBuilder {
	return fb.ID(col, f)
}

// JSON применяет JSONFilter
func (fb *FilterBuilder) JSON(col string, f *JSONFilter) *FilterBuilder {
	if f == nil {
		return fb
	}
	if f.Equals != nil {
		data, _ := json.Marshal(f.Equals)
		fb.db = fb.db.Where(col+" = ?::jsonb", string(data))
	}
	if f.Contains != nil {
		data, _ := json.Marshal(f.Contains)
		fb.db = fb.db.Where(col+" @> ?::jsonb", string(data))
	}
	if f.ContainedBy != nil {
		data, _ := json.Marshal(f.ContainedBy)
		fb.db = fb.db.Where(col+" <@ ?::jsonb", string(data))
	}
	if f.HasKey != nil {
		fb.db = fb.db.Where(col+" ? ?", *f.HasKey)
	}
	if len(f.HasKeys) > 0 {
		fb.db = fb.db.Where(col+" ?& ?", f.HasKeys)
	}
	if len(f.HasAnyKey) > 0 {
		fb.db = fb.db.Where(col+" ?| ?", f.HasAnyKey)
	}
	if f.IsNull != nil {
		if *f.IsNull {
			fb.db = fb.db.Where(col + " IS NULL")
		} else {
			fb.db = fb.db.Where(col + " IS NOT NULL")
		}
	}
	return fb
}

// Enum применяет enum фильтр
func (fb *FilterBuilder) Enum(col string, equals, not *string, in, notIn []string) *FilterBuilder {
	if equals != nil {
		fb.db = fb.db.Where(col+" = ?", *equals)
	}
	if not != nil {
		fb.db = fb.db.Where(col+" != ?", *not)
	}
	if len(in) > 0 {
		fb.db = fb.db.Where(col+" IN ?", in)
	}
	if len(notIn) > 0 {
		fb.db = fb.db.Where(col+" NOT IN ?", notIn)
	}
	return fb
}

// ============================================================================
// LOGICAL OPERATORS
// ============================================================================

// OR объединяет условия через OR
func (fb *FilterBuilder) OR(conditions ...func(*FilterBuilder)) *FilterBuilder {
	if len(conditions) == 0 {
		return fb
	}

	fb.db = fb.db.Where(fb.db.Session(&gorm.Session{NewDB: true}).Scopes(func(db *gorm.DB) *gorm.DB {
		var result *gorm.DB
		for i, cond := range conditions {
			subDB := db.Session(&gorm.Session{NewDB: true}).Model(fb.model)
			subFB := &FilterBuilder{db: subDB, model: fb.model, maxLimit: fb.maxLimit}
			cond(subFB)

			if i == 0 {
				result = subFB.db
			} else {
				result = result.Or(subFB.db)
			}
		}
		return result
	}))
	return fb
}

// AND объединяет условия через AND
func (fb *FilterBuilder) AND(conditions ...func(*FilterBuilder)) *FilterBuilder {
	for _, cond := range conditions {
		subDB := fb.db.Session(&gorm.Session{NewDB: true}).Model(fb.model)
		subFB := &FilterBuilder{db: subDB, model: fb.model, maxLimit: fb.maxLimit}
		cond(subFB)
		fb.db = fb.db.Where(subFB.db)
	}
	return fb
}

// NOT инвертирует условие
func (fb *FilterBuilder) NOT(condition func(*FilterBuilder)) *FilterBuilder {
	subDB := fb.db.Session(&gorm.Session{NewDB: true}).Model(fb.model)
	subFB := &FilterBuilder{db: subDB, model: fb.model, maxLimit: fb.maxLimit}
	condition(subFB)
	fb.db = fb.db.Not(subFB.db)
	return fb
}

// ============================================================================
// RELATION FILTERS
// ============================================================================

// Some - EXISTS подзапрос
func (fb *FilterBuilder) Some(subTable, fk, pk string, condition func(*FilterBuilder)) *FilterBuilder {
	subDB := fb.db.Session(&gorm.Session{NewDB: true}).Table(subTable)
	subFB := &FilterBuilder{db: subDB, model: fb.model, maxLimit: fb.maxLimit}
	condition(subFB)
	subQuery := subFB.db.Select("1").Where(fk + " = " + pk)
	fb.db = fb.db.Where("EXISTS (?)", subQuery)
	return fb
}

// None - NOT EXISTS подзапрос
func (fb *FilterBuilder) None(subTable, fk, pk string, condition func(*FilterBuilder)) *FilterBuilder {
	subDB := fb.db.Session(&gorm.Session{NewDB: true}).Table(subTable)
	subFB := &FilterBuilder{db: subDB, model: fb.model, maxLimit: fb.maxLimit}
	condition(subFB)
	subQuery := subFB.db.Select("1").Where(fk + " = " + pk)
	fb.db = fb.db.Where("NOT EXISTS (?)", subQuery)
	return fb
}

// InIDs добавляет WHERE col IN (ids)
func (fb *FilterBuilder) InIDs(col string, ids []string) *FilterBuilder {
	if len(ids) == 0 {
		fb.db = fb.db.Where("1 = 0")
		return fb
	}
	fb.db = fb.db.Where(col+" IN ?", ids)
	return fb
}

// ============================================================================
// ORDER BY
// ============================================================================

// OrderBy добавляет сортировку
func (fb *FilterBuilder) OrderBy(col string, order *SortOrder) *FilterBuilder {
	if order == nil {
		return fb
	}
	dir := "ASC"
	if *order == SortOrderDesc {
		dir = "DESC"
	}
	fb.db = fb.db.Order(col + " " + dir)
	return fb
}

// OrderByDefault добавляет дефолтную сортировку
func (fb *FilterBuilder) OrderByDefault(defaultOrder string) *FilterBuilder {
	fb.db = fb.db.Order(defaultOrder)
	return fb
}

// ============================================================================
// PAGINATION
// ============================================================================

// Paginate применяет пагинацию
func (fb *FilterBuilder) Paginate(p *PaginationInput) *FilterBuilder {
	limit := 20
	offset := 0

	if p != nil {
		limit = p.GetLimit()
		offset = p.GetOffset()
	}

	if limit > fb.maxLimit {
		limit = fb.maxLimit
	}

	fb.db = fb.db.Limit(limit).Offset(offset)
	return fb
}

// ============================================================================
// EXECUTE
// ============================================================================

// Count возвращает количество записей
func (fb *FilterBuilder) Count() (int64, error) {
	var count int64
	err := fb.db.Limit(-1).Offset(-1).Count(&count).Error
	return count, err
}

// Find выполняет запрос
func (fb *FilterBuilder) Find(dest interface{}) error {
	return fb.db.Find(dest).Error
}

// First возвращает первую запись
func (fb *FilterBuilder) First(dest interface{}) error {
	return fb.db.First(dest).Error
}

// FindWithCount - основной метод: данные + count + pagination
func (fb *FilterBuilder) FindWithCount(dest interface{}, p *PaginationInput) (*Pagination, error) {
	total, err := fb.Count()
	if err != nil {
		return nil, err
	}

	fb.Paginate(p)

	if err := fb.Find(dest); err != nil {
		return nil, err
	}

	return NewPagination(p, total), nil
}

// Where добавляет произвольное условие
func (fb *FilterBuilder) Where(query interface{}, args ...interface{}) *FilterBuilder {
	fb.db = fb.db.Where(query, args...)
	return fb
}

// ============================================================================
// APPLY LOGICAL HELPER
// ============================================================================

// ApplyLogical применяет OR/AND/NOT операторы
func ApplyLogical[T any](fb *FilterBuilder, or []*T, and []*T, not *T, applyFn func(*FilterBuilder, *T)) {
	if len(or) > 0 {
		funcs := make([]func(*FilterBuilder), len(or))
		for i, w := range or {
			w := w
			funcs[i] = func(b *FilterBuilder) {
				applyFn(b, w)
			}
		}
		fb.OR(funcs...)
	}

	if len(and) > 0 {
		fb.AND(func(b *FilterBuilder) {
			for _, w := range and {
				applyFn(b, w)
			}
		})
	}

	if not != nil {
		fb.NOT(func(b *FilterBuilder) {
			applyFn(b, not)
		})
	}
}
