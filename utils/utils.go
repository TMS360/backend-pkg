package utils

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/go-viper/mapstructure/v2"
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

// StructToMap Особенности:
// 1. Пропускает nil-поля (для указателей).
// 2. Использует тег "mapstructure" или "json" для ключей мапы.
// 3. Если поле не указатель, оно попадет в мапу "как есть" (будьте осторожны с zero-values).
func StructToMap(input interface{}) map[string]interface{} {
	out := make(map[string]interface{})

	v := reflect.ValueOf(input)

	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return out
		}
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
		}

		if idx := strings.Index(tag, ","); idx != -1 {
			tag = tag[:idx]
		}

		if tag == "" || tag == "-" {
			continue
		}

		if fieldVal.Kind() == reflect.Ptr {
			if !fieldVal.IsNil() {
				out[tag] = fieldVal.Elem().Interface()
			}
		} else {
			out[tag] = fieldVal.Interface()
		}
	}

	return out
}

// MergeUpdates применяет значения из mapChanges к структуре target.
// target должен быть указателем на структуру.
// Использует mapstructure/v2 для безопасного приведения типов.
func MergeUpdates(target interface{}, mapChanges map[string]interface{}) error {
	config := &mapstructure.DecoderConfig{
		Metadata:         nil,
		Result:           target,
		TagName:          "mapstructure",
		WeaklyTypedInput: true,
		ZeroFields:       false,
		Squash:           true,
	}

	decoder, err := mapstructure.NewDecoder(config)
	if err != nil {
		return fmt.Errorf("failed to create decoder: %w", err)
	}

	if err := decoder.Decode(mapChanges); err != nil {
		return fmt.Errorf("failed to decode updates: %w", err)
	}

	return nil
}

func StringPtr(s string) *string {
	return &s
}
