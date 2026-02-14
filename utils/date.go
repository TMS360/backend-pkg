package utils

import (
	"fmt"
	"time"
)

const (
	// DateFormatISO - YYYY-MM-DD (2006-01-02)
	DateFormatISO = "2006-01-02"

	// DateFormatEuropean - DD-MM-YYYY (02-01-2006)
	DateFormatEuropean = "02-01-2006"

	// RFC3339Nano - для обработки точных дат с фронтенда (2025-12-12T00:00:00Z)
	// time.RFC3339Nano уже является константой в пакете time
)

// ParseDateString пытается преобразовать входную строку даты в time.Time.
// Поддерживает YYYY-MM-DD, DD-MM-YYYY и RFC3339/RFC3339Nano.
// Все успешные результаты возвращаются в 00:00:00 UTC.
func ParseDateString(dateStr string) (time.Time, error) {
	// Список поддерживаемых форматов в порядке приоритета
	formats := []string{
		time.RFC3339Nano,
		time.RFC3339,
		DateFormatISO,      // DateFormatISO
		DateFormatEuropean, // DateFormatEuropean
	}

	for _, layout := range formats {
		if t, err := time.Parse(layout, dateStr); err == nil {
			// Успех! Нормализуем к UTC (ваша функция)
			return AsDateInUTC(t), nil
		}
	}

	return time.Time{}, fmt.Errorf("invalid date format: '%s'", dateStr)
}

// AsDateInUTC возвращает time.Time в 00:00:00 UTC,
// игнорируя исходное время и локацию.
func AsDateInUTC(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// GetWeekRange возвращает начало и конец недели (Понедельник 00:00:00 и Воскресенье 23:59:59)
func GetWeekRange(refDate time.Time) (time.Time, time.Time) {
	return GetWeekStart(refDate), GetWeekEnd(refDate)
}

func GetWeekStart(refDate time.Time) time.Time {
	weekday := refDate.Weekday()
	// Go's time.Weekday is an enum where Sunday is 0.
	// We adjust it so Monday is 1 and Sunday is 7.
	if weekday == time.Sunday {
		weekday = 7
	}
	offset := int(weekday) - 1

	start := refDate.AddDate(0, 0, -offset)
	return time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
}

func GetWeekEnd(refDate time.Time) time.Time {
	// Reuse the Start logic to find Monday
	start := GetWeekStart(refDate)

	// Move forward to Sunday
	end := start.AddDate(0, 0, 6)
	return time.Date(end.Year(), end.Month(), end.Day(), 23, 59, 59, 999999999, time.UTC)
}

// TruncateToDay сбрасывает время в 00:00:00
func TruncateToDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// GetTimeOrMax Helper для SQL (бесконечность)
func GetTimeOrMax(t *time.Time) time.Time {
	if t == nil {
		return time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	return *t
}

func CombineDateAndTime(dateStr, timeStr string) (*time.Time, error) {
	if dateStr == "" {
		return nil, nil
	}

	layout := "2006-01-02 15:04"
	combined := fmt.Sprintf("%s %s", dateStr, timeStr)
	t, err := time.Parse(layout, combined)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
