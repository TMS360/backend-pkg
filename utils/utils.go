package utils

// ValOrEmpty возвращает значение строки или пустую строку, если указатель nil.
// Используется для конвертации GraphQL Input (*string) -> DB Model (string).
func ValOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// Pointer возвращает указатель на значение любого типа.
// Удобно для создания inline указателей: utils.Pointer(uuid.New()) или utils.Pointer(10)
func Pointer[T any](v T) *T {
	return &v
}

// ValOrZero возвращает значение или zero-value для любого типа (int, bool, float).
// Generic версия ValOrEmpty.
func ValOrZero[T any](v *T) T {
	if v == nil {
		var zero T
		return zero
	}
	return *v
}
