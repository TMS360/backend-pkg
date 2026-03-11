package tmsdb

import (
	"time"

	filterspb "github.com/TMS360/backend-pkg/proto/filters"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ============================================================================
// Proto → tmsdb filter converters
// ============================================================================

// ConvertStringFilter converts proto StringFilter to tmsdb StringFilter.
func ConvertStringFilter(pf *filterspb.StringFilter) *StringFilter {
	if pf == nil {
		return nil
	}

	f := &StringFilter{
		In:    pf.GetIn(),
		NotIn: pf.GetNotIn(),
	}

	if pf.Equals != nil {
		f.Equals = pf.Equals
	}
	if pf.Not != nil {
		f.Not = pf.Not
	}
	if pf.Contains != nil {
		f.Contains = pf.Contains
	}
	if pf.StartsWith != nil {
		f.StartsWith = pf.StartsWith
	}
	if pf.EndsWith != nil {
		f.EndsWith = pf.EndsWith
	}
	if pf.Like != nil {
		f.Like = pf.Like
	}
	if pf.Regex != nil {
		f.Regex = pf.Regex
	}
	if pf.IsNull != nil {
		f.IsNull = pf.IsNull
	}

	switch pf.GetMode() {
	case filterspb.QueryMode_QUERY_MODE_INSENSITIVE:
		f.Mode = QueryModeInsensitive
	default:
		f.Mode = QueryModeDefault
	}

	return f
}

// ConvertIntFilter converts proto IntFilter to tmsdb IntFilter.
func ConvertIntFilter(pf *filterspb.IntFilter) *IntFilter {
	if pf == nil {
		return nil
	}

	f := &IntFilter{}

	if pf.Equals != nil {
		v := int(*pf.Equals)
		f.Equals = &v
	}
	if pf.Not != nil {
		v := int(*pf.Not)
		f.Not = &v
	}
	if len(pf.GetIn()) > 0 {
		f.In = int32sToInts(pf.GetIn())
	}
	if len(pf.GetNotIn()) > 0 {
		f.NotIn = int32sToInts(pf.GetNotIn())
	}
	if pf.Lt != nil {
		v := int(*pf.Lt)
		f.Lt = &v
	}
	if pf.Lte != nil {
		v := int(*pf.Lte)
		f.Lte = &v
	}
	if pf.Gt != nil {
		v := int(*pf.Gt)
		f.Gt = &v
	}
	if pf.Gte != nil {
		v := int(*pf.Gte)
		f.Gte = &v
	}
	if pf.IsNull != nil {
		f.IsNull = pf.IsNull
	}

	return f
}

// ConvertFloatFilter converts proto FloatFilter to tmsdb FloatFilter.
func ConvertFloatFilter(pf *filterspb.FloatFilter) *FloatFilter {
	if pf == nil {
		return nil
	}

	f := &FloatFilter{
		In:    pf.GetIn(),
		NotIn: pf.GetNotIn(),
	}

	if pf.Equals != nil {
		f.Equals = pf.Equals
	}
	if pf.Not != nil {
		f.Not = pf.Not
	}
	if pf.Lt != nil {
		f.Lt = pf.Lt
	}
	if pf.Lte != nil {
		f.Lte = pf.Lte
	}
	if pf.Gt != nil {
		f.Gt = pf.Gt
	}
	if pf.Gte != nil {
		f.Gte = pf.Gte
	}
	if pf.IsNull != nil {
		f.IsNull = pf.IsNull
	}

	return f
}

// ConvertBoolFilter converts proto BoolFilter to tmsdb BoolFilter.
func ConvertBoolFilter(pf *filterspb.BoolFilter) *BoolFilter {
	if pf == nil {
		return nil
	}

	f := &BoolFilter{}

	if pf.Equals != nil {
		f.Equals = pf.Equals
	}
	if pf.IsNull != nil {
		f.IsNull = pf.IsNull
	}

	return f
}

// ConvertDateTimeFilter converts proto DateTimeFilter to tmsdb DateTimeFilter.
func ConvertDateTimeFilter(pf *filterspb.DateTimeFilter) *DateTimeFilter {
	if pf == nil {
		return nil
	}

	f := &DateTimeFilter{}

	if pf.Equals != nil {
		f.Equals = tsToTimePtr(pf.Equals)
	}
	if pf.Not != nil {
		f.Not = tsToTimePtr(pf.Not)
	}
	if len(pf.GetIn()) > 0 {
		f.In = tsSliceToTimes(pf.GetIn())
	}
	if len(pf.GetNotIn()) > 0 {
		f.NotIn = tsSliceToTimes(pf.GetNotIn())
	}
	if pf.Lt != nil {
		f.Lt = tsToTimePtr(pf.Lt)
	}
	if pf.Lte != nil {
		f.Lte = tsToTimePtr(pf.Lte)
	}
	if pf.Gt != nil {
		f.Gt = tsToTimePtr(pf.Gt)
	}
	if pf.Gte != nil {
		f.Gte = tsToTimePtr(pf.Gte)
	}
	if pf.IsNull != nil {
		f.IsNull = pf.IsNull
	}

	return f
}

// ConvertIDFilter converts proto IDFilter to tmsdb IDFilter.
func ConvertIDFilter(pf *filterspb.IDFilter) *IDFilter {
	if pf == nil {
		return nil
	}

	f := &IDFilter{
		In:    pf.GetIn(),
		NotIn: pf.GetNotIn(),
	}

	if pf.Equals != nil {
		f.Equals = pf.Equals
	}
	if pf.Not != nil {
		f.Not = pf.Not
	}
	if pf.IsNull != nil {
		f.IsNull = pf.IsNull
	}

	return f
}

// ============================================================================
// Helpers
// ============================================================================

func int32sToInts(in []int32) []int {
	out := make([]int, len(in))
	for i, v := range in {
		out[i] = int(v)
	}
	return out
}

func tsToTimePtr(ts *timestamppb.Timestamp) *time.Time {
	if ts == nil {
		return nil
	}
	t := ts.AsTime()
	return &t
}

func tsSliceToTimes(tss []*timestamppb.Timestamp) []time.Time {
	out := make([]time.Time, len(tss))
	for i, ts := range tss {
		out[i] = ts.AsTime()
	}
	return out
}
