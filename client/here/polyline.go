package here

import (
	"encoding/json"
	"fmt"
	"math"
)

// DecodedCoordinate represents a single decoded lat/lng pair from a flexible polyline.
type DecodedCoordinate struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"long"`
}

// HERE Flexible Polyline alphabet (NOT the same as Google Polyline).
// Each character maps to its index (0–63).
const flexPolylineAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"

// flexCharToValue is a lookup table for fast character decoding.
var flexCharToValue [256]int

func init() {
	for i := range flexCharToValue {
		flexCharToValue[i] = -1
	}
	for i := 0; i < len(flexPolylineAlphabet); i++ {
		flexCharToValue[flexPolylineAlphabet[i]] = i
	}
}

// DecodeFlexiblePolyline decodes a HERE Maps flexible polyline string into coordinates.
// HERE Routing v8 API returns polylines in this format by default.
// Reference: https://github.com/heremaps/flexible-polyline
func DecodeFlexiblePolyline(encoded string) ([]DecodedCoordinate, error) {
	if len(encoded) == 0 {
		return nil, nil
	}

	data := []byte(encoded)
	index := 0

	// Step 1: Read version (usually 1)
	_, idx, err := decodeUnsignedValue(data, index)
	if err != nil {
		return nil, fmt.Errorf("flexpolyline: failed to decode version: %w", err)
	}
	index = idx

	// Step 2: Read header content
	headerValue, idx, err := decodeUnsignedValue(data, index)
	if err != nil {
		return nil, fmt.Errorf("flexpolyline: failed to decode header: %w", err)
	}
	index = idx

	precision := headerValue & 0x0F
	thirdDimType := (headerValue >> 4) & 0x07
	hasThirdDim := thirdDimType != 0

	scale := math.Pow(10, float64(precision))

	var coords []DecodedCoordinate
	var lastLat, lastLng, lastZ int64

	for index < len(data) {
		latDelta, idx, err := decodeSignedValue(data, index)
		if err != nil {
			return nil, fmt.Errorf("flexpolyline: failed to decode lat at pos %d: %w", index, err)
		}
		index = idx
		lastLat += latDelta

		if index >= len(data) {
			return nil, fmt.Errorf("flexpolyline: unexpected end after lat")
		}
		lngDelta, idx, err := decodeSignedValue(data, index)
		if err != nil {
			return nil, fmt.Errorf("flexpolyline: failed to decode lng at pos %d: %w", index, err)
		}
		index = idx
		lastLng += lngDelta

		if hasThirdDim {
			if index >= len(data) {
				return nil, fmt.Errorf("flexpolyline: unexpected end after lng")
			}
			zDelta, idx, err := decodeSignedValue(data, index)
			if err != nil {
				return nil, fmt.Errorf("flexpolyline: failed to decode z at pos %d: %w", index, err)
			}
			index = idx
			lastZ += zDelta
		}
		_ = lastZ

		coords = append(coords, DecodedCoordinate{
			Lat: float64(lastLat) / scale,
			Lng: float64(lastLng) / scale,
		})
	}

	return coords, nil
}

// DecodePolylineToJSON decodes a flexible polyline and returns a JSON string:
// [{"latitude":50.10228,"longitude":8.69821},...]
func DecodePolylineToJSON(encoded string) (string, error) {
	coords, err := DecodeFlexiblePolyline(encoded)
	if err != nil {
		return "[]", err
	}
	if len(coords) == 0 {
		return "[]", nil
	}

	b, err := json.Marshal(coords)
	if err != nil {
		return "[]", fmt.Errorf("flexpolyline: failed to marshal: %w", err)
	}
	return string(b), nil
}

func decodePolylineChar(c byte) (int64, error) {
	v := flexCharToValue[c]
	if v < 0 {
		return 0, fmt.Errorf("invalid character %q", c)
	}
	return int64(v), nil
}

func decodeUnsignedValue(data []byte, index int) (int64, int, error) {
	var result int64
	shift := 0

	for index < len(data) {
		v, err := decodePolylineChar(data[index])
		if err != nil {
			return 0, index, err
		}
		index++
		result |= (v & 0x1F) << shift
		shift += 5
		if v < 0x20 {
			return result, index, nil
		}
	}
	return 0, index, fmt.Errorf("unexpected end of input")
}

func decodeSignedValue(data []byte, index int) (int64, int, error) {
	unsigned, idx, err := decodeUnsignedValue(data, index)
	if err != nil {
		return 0, idx, err
	}
	if unsigned&1 != 0 {
		return -(unsigned >> 1) - 1, idx, nil
	}
	return unsigned >> 1, idx, nil
}
