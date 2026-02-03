package utils

import (
	"reflect"
	"strings"
)

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

// ValOrZero возвращает значение или zero-value для любого типа (int, bool, float).
// Generic версия ValOrEmpty.
func ValOrZero[T any](v *T) T {
	if v == nil {
		var zero T
		return zero
	}
	return *v
}

func StructToMap(input interface{}) map[string]interface{} {
	out := make(map[string]interface{})

	v := reflect.ValueOf(input)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return out
	}

	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		fieldVal := v.Field(i)
		fieldType := t.Field(i)

		tag := fieldType.Tag.Get("mapstructure")
		if tag == "" {
			tag = fieldType.Tag.Get("json")
			if idx := strings.Index(tag, ","); idx != -1 {
				tag = tag[:idx]
			}
		}

		if tag == "" || tag == "-" {
			continue
		}

		if fieldVal.Kind() == reflect.Ptr {
			if fieldVal.IsNil() {
				continue
			}
			out[tag] = fieldVal.Elem().Interface()
		} else {
			out[tag] = fieldVal.Interface()
		}
	}

	return out
}
