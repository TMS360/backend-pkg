package tmsdb

import (
	"encoding/json"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"
)

// StructFilter parses a protobuf Struct into tmsdb filter types.
// Used on the server side of ResolverService to extract typed filters.
type StructFilter struct {
	raw map[string]json.RawMessage
}

// NewStructFilter creates a StructFilter from a protobuf Struct.
func NewStructFilter(s *structpb.Struct) *StructFilter {
	sf := &StructFilter{raw: make(map[string]json.RawMessage)}
	if s == nil {
		return sf
	}

	// Convert Struct → JSON bytes → map of raw messages
	jsonBytes, err := protojson.Marshal(s)
	if err != nil {
		return sf
	}

	_ = json.Unmarshal(jsonBytes, &sf.raw)
	return sf
}

// StringFilter extracts a StringFilter for the given key.
func (sf *StructFilter) StringFilter(key string) *StringFilter {
	data, ok := sf.raw[key]
	if !ok {
		return nil
	}
	var f StringFilter
	if err := json.Unmarshal(data, &f); err != nil {
		return nil
	}
	return &f
}

// IntFilter extracts an IntFilter for the given key.
func (sf *StructFilter) IntFilter(key string) *IntFilter {
	data, ok := sf.raw[key]
	if !ok {
		return nil
	}
	var f IntFilter
	if err := json.Unmarshal(data, &f); err != nil {
		return nil
	}
	return &f
}

// BoolFilter extracts a BoolFilter for the given key.
func (sf *StructFilter) BoolFilter(key string) *BoolFilter {
	data, ok := sf.raw[key]
	if !ok {
		return nil
	}
	var f BoolFilter
	if err := json.Unmarshal(data, &f); err != nil {
		return nil
	}
	return &f
}

// IDFilter extracts an IDFilter for the given key.
func (sf *StructFilter) IDFilter(key string) *IDFilter {
	data, ok := sf.raw[key]
	if !ok {
		return nil
	}
	var f IDFilter
	if err := json.Unmarshal(data, &f); err != nil {
		return nil
	}
	return &f
}

// ToFilterStruct converts a map of filter fields into a protobuf Struct.
// Used on the client side to build ResolveIDsRequest.Filter.
// Values can be *StringFilter, *IntFilter, *BoolFilter, *IDFilter, or any JSON-serializable type.
func ToFilterStruct(fields map[string]any) *structpb.Struct {
	if len(fields) == 0 {
		return nil
	}

	// Convert each filter to its JSON representation, then to Struct
	jsonMap := make(map[string]any, len(fields))
	for k, v := range fields {
		if v == nil {
			continue
		}
		// Marshal and unmarshal to get a clean map[string]any
		b, err := json.Marshal(v)
		if err != nil {
			continue
		}
		var m any
		if err := json.Unmarshal(b, &m); err != nil {
			continue
		}
		jsonMap[k] = m
	}

	s, err := structpb.NewStruct(jsonMap)
	if err != nil {
		return nil
	}
	return s
}
