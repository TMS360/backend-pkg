package here

import (
	"encoding/json"
	"fmt"
	"math"
)

// DecodedCoordinate represents a single decoded lat/lng pair from a flexible polyline.
type DecodedCoordinate struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
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

	// Decode header: precision (4 bits) | thirdDimType (3 bits) | thirdDimPrecision (4 bits)
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

// DecodePolylineToJSON decodes a flexible polyline and returns a JSON string: [[lat,lng],...]
func DecodePolylineToJSON(encoded string) (string, error) {
	coords, err := DecodeFlexiblePolyline(encoded)
	if err != nil {
		return "[]", err
	}
	if len(coords) == 0 {
		return "[]", nil
	}

	pairs := make([][2]float64, len(coords))
	for i, c := range coords {
		pairs[i] = [2]float64{c.Lat, c.Lng}
	}

	b, err := json.Marshal(pairs)
	if err != nil {
		return "[]", fmt.Errorf("flexpolyline: failed to marshal: %w", err)
	}
	return string(b), nil
}

func decodePolylineChar(c byte) (int64, error) {
	v := int64(c) - 63
	if v < 0 || v > 63 {
		return 0, fmt.Errorf("invalid character %q", c)
	}
	return v, nil
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
