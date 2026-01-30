package tmsdb

// Ptr создаёт указатель на любое значение
func Ptr[T any](v T) *T {
	return &v
}

// Col формирует имя столбца: Col("users", "email") → "users.email"
func Col(table, field string) string {
	if table == "" {
		return field
	}
	return table + "." + field
}

// Asc возвращает *SortOrderAsc
func Asc() *SortOrder {
	o := SortOrderAsc
	return &o
}

// Desc возвращает *SortOrderDesc
func Desc() *SortOrder {
	o := SortOrderDesc
	return &o
}
