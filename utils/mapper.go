package utils

import "reflect"

// ApplyPartialUpdates iterates over 'updates' (which should be a struct of pointers)
// and applies any non-nil values to 'target' (which must be a pointer to a struct).
// It matches fields strictly by their exact struct field Name.
func ApplyPartialUpdates(target any, updates any) {
	targetVal := reflect.ValueOf(target).Elem() // Must be a pointer
	updatesVal := reflect.ValueOf(updates)      // Can be a struct by value

	for i := 0; i < updatesVal.NumField(); i++ {
		updateField := updatesVal.Field(i)
		structField := updatesVal.Type().Field(i)

		// Only process fields that are pointers and are NOT nil
		if updateField.Kind() == reflect.Ptr && !updateField.IsNil() {
			// Find the corresponding field in the target struct by name
			targetField := targetVal.FieldByName(structField.Name)

			if targetField.IsValid() && targetField.CanSet() {
				// If the target field is also a pointer, assign directly
				if targetField.Kind() == reflect.Ptr {
					targetField.Set(updateField)
				} else {
					// Otherwise, dereference the update pointer and set the value
					targetField.Set(updateField.Elem())
				}
			}
		}
	}
}
