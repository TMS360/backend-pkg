package utils

func Pointer[T any](v T) *T {
	return &v
}

// Deref safely dereferences a pointer.
// If the pointer is nil, it returns the zero value of type T (e.g. "" for string, 0 for int).
// If the pointer is not nil, it returns the value.
func Deref[T any](ptr *T) T {
	if ptr == nil {
		var zero T
		return zero
	}
	return *ptr
}

// DerefOr returns the pointer value if not nil, otherwise returns the provided default value.
// Useful if you want specific defaults: DerefOr(input.Name, "Unknown")
func DerefOr[T any](ptr *T, def T) T {
	if ptr == nil {
		return def
	}
	return *ptr
}
