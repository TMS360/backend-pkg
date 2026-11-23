package toll

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// bestPassTransaction is the part of a Fleet API Transaction we read. The
// whole object is kept in Row.Raw, so fields not modelled here are not lost.
type bestPassTransaction struct {
	TransactionID    *int64              `json:"transactionId"`
	TollPostDate     string              `json:"tollPostDate"`
	ChargeAmount     decimal.Decimal     `json:"chargeAmount"`
	DiscountedAmount decimal.NullDecimal `json:"discountedAmount"`
	FeeAmount        decimal.NullDecimal `json:"feeAmount"`
	DeviceNumber     string              `json:"deviceNumber"`
	Unit             string              `json:"unit"`
	EntryDateLocal   string              `json:"entryDateLocal"`
	EntryPlazaName   string              `json:"entryPlazaName"`
	ExitDateLocal    string              `json:"exitDateLocal"`
	ExitPlazaName    string              `json:"exitPlazaName"`
	LicensePlate     string              `json:"licensePlate"`
	TransactionClass string              `json:"transactionClass"`
	Miles            *float64            `json:"miles"`
	Agency           string              `json:"agency"`
	ServiceName      string              `json:"serviceName"`
}

// bestPassAPITimeLayouts are the date-time shapes the API may send. Zone-less
// text is UTC, as the spec says.
var bestPassAPITimeLayouts = []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999", "2006-01-02T15:04:05", "2006-01-02"}

// rowsFromTransactions maps one API answer onto rows keyed exactly like the
// file reader keys them, so a toll read by file and by API is one row.
// RowNumber is the 1-based position in the answer; account stamps the import.
func rowsFromTransactions(raws []json.RawMessage, account string) ParseResult {
	res := ParseResult{Account: account}
	parsed := make([]bestPassParsed, 0, len(raws))
	for i, raw := range raws {
		p, err := parseBestPassTransaction(i+1, raw)
		if err != nil {
			res.Errors = append(res.Errors, asRowError(i+1, err))
			continue
		}
		parsed = append(parsed, p)
	}
	rows, keyErrs := keyBestPassRows(parsed)
	res.Rows = rows
	res.Errors = append(res.Errors, keyErrs...)
	return res
}

// parseBestPassTransaction is the API twin of parseBestPassRow: same fields,
// same charged amount, exit time as the plaza's wall clock.
func parseBestPassTransaction(rowNum int, raw json.RawMessage) (bestPassParsed, error) {
	var t bestPassTransaction
	if err := json.Unmarshal(raw, &t); err != nil {
		return bestPassParsed{}, RowError{RowNumber: rowNum, Err: fmt.Errorf("unreadable transaction: %w", err)}
	}
	post, hasPost := parseBestPassAPITime(t.TollPostDate, false)
	exit, hasExit := parseBestPassAPITime(t.ExitDateLocal, true)
	if !hasPost && !hasExit {
		return bestPassParsed{}, RowError{RowNumber: rowNum, Column: "tollPostDate", Value: t.TollPostDate, Err: fmt.Errorf("no post date and no exit date")}
	}
	row := Row{
		RowNumber:  rowNum,
		PostDate:   post,
		EntryAt:    optionalTime(parseBestPassAPITime(t.EntryDateLocal, true)),
		ExitAt:     optionalTime(exit, hasExit),
		Source:     t.ServiceName,
		Agency:     t.Agency,
		DeviceID:   NormalizeDeviceID(t.DeviceNumber),
		Plate:      NormalizePlate(t.LicensePlate),
		TruckRef:   NormalizeTruckRef(t.Unit),
		EntryPlaza: t.EntryPlazaName,
		ExitPlaza:  t.ExitPlazaName,
		Class:      t.TransactionClass,
		Miles:      formatMiles(t.Miles),
		Amount:     bestPassAPIAmount(t),
		Raw:        rawStrings(raw),
	}
	return bestPassParsed{row: row, txID: transactionIDText(t.TransactionID)}, nil
}

// bestPassAPIAmount is the file's rule on API fields: the discounted amount
// (the charge amount when it is missing) plus the fee (null = 0), sign kept.
func bestPassAPIAmount(t bestPassTransaction) decimal.Decimal {
	base := t.ChargeAmount
	if t.DiscountedAmount.Valid {
		base = t.DiscountedAmount.Decimal
	}
	fee := decimal.Zero
	if t.FeeAmount.Valid {
		fee = t.FeeAmount.Decimal
	}
	return base.Add(fee).Round(2)
}

// parseBestPassAPITime reads an API date-time. wallClock keeps the digits as
// printed and stamps them UTC — the convention for file times, which are the
// plaza's local clock, never instants (see Row).
func parseBestPassAPITime(s string, wallClock bool) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range bestPassAPITimeLayouts {
		ts, err := time.Parse(layout, s)
		if err != nil {
			continue
		}
		if wallClock {
			return time.Date(ts.Year(), ts.Month(), ts.Day(), ts.Hour(), ts.Minute(), ts.Second(), 0, time.UTC), true
		}
		return ts.UTC(), true
	}
	return time.Time{}, false
}

func optionalTime(t time.Time, ok bool) *time.Time {
	if !ok {
		return nil
	}
	return &t
}

func transactionIDText(id *int64) string {
	if id == nil {
		return ""
	}
	return strconv.FormatInt(*id, 10)
}

func formatMiles(m *float64) string {
	if m == nil {
		return ""
	}
	return strconv.FormatFloat(*m, 'f', -1, 64)
}

// rawStrings keeps the whole transaction: scalars as text, nested objects as
// their JSON, so unknown words and new fields survive into the pool.
func rawStrings(raw json.RawMessage) map[string]string {
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		return nil
	}
	out := make(map[string]string, len(obj))
	for k, v := range obj {
		var s string
		if json.Unmarshal(v, &s) == nil {
			out[k] = s
			continue
		}
		if string(v) != "null" {
			out[k] = string(v)
		}
	}
	return out
}
