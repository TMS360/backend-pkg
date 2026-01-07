package scalar

import (
	"fmt"
	"io"
	"log"
	"strconv"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/TMS360/backend-pkg/utils"
)

// Date Создаем свой тип на основе time.Time
type Date = time.Time

func MarshalDate(t time.Time) graphql.Marshaler {
	return graphql.WriterFunc(func(w io.Writer) {
		if _, err := io.WriteString(w, strconv.Quote(t.Format(utils.DateFormatISO))); err != nil {
			log.Fatal(err)
		}
	})
}

func UnmarshalDate(v interface{}) (time.Time, error) {
	str, ok := v.(string)
	if !ok {
		return time.Time{}, fmt.Errorf("date must be a string")
	}

	// Ваша кастомная логика парсинга
	return utils.ParseDateString(str)
}
