package model

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/99designs/gqlgen/graphql"
)

func MarshalJSONObject(val interface{}) graphql.Marshaler {
	return graphql.WriterFunc(func(w io.Writer) {
		err := json.NewEncoder(w).Encode(val)
		if err != nil {
			fmt.Printf("error marshaling JSONObject: %v", err)
		}
	})
}

func UnmarshalJSONObject(v interface{}) (interface{}, error) {
	switch v := v.(type) {
	case map[string]interface{}, []interface{}:
		return v, nil
	default:
		return nil, fmt.Errorf("%T is not a valid JSONObject (map or array)", v)
	}
}
