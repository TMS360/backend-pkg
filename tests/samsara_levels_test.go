package tests

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/TMS360/backend-pkg/client/samsara"
	"github.com/TMS360/backend-pkg/config"
	"github.com/stretchr/testify/require"
)

// DEV-2006 — уровень топлива и уровень DEF как поля уже выполняемого запроса
// статистики. Тесты держат ровно то, ради чего пакет трогали: состав types
// задаёт вызывающий, DEF разбирается из ответа, а отказ по тарифу отличается от
// неверного ключа.

func samsaraTestClient(t *testing.T, handler http.HandlerFunc) *samsara.Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	client, err := samsara.NewClient(config.SamsaraConfig{Host: server.URL}, "test-key")
	require.NoError(t, err)
	return client
}

func capturedTypes(t *testing.T, r *http.Request) string {
	t.Helper()
	parsed, err := url.Parse(r.URL.String())
	require.NoError(t, err)
	return parsed.Query().Get("types")
}

func TestStatsSnapshotAsksExactlyTheRequestedTypes(t *testing.T) {
	var got string
	client := samsaraTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = capturedTypes(t, r)
		fmt.Fprint(w, `{"data":[],"pagination":{"hasNextPage":false}}`)
	})

	_, err := client.GetAllVehiclesStatsWithTypes(context.Background(),
		[]string{samsara.StatTypeGPS, samsara.StatTypeDefFluidMilliPercent})
	require.NoError(t, err)
	require.Equal(t, "gps,defFluidMilliPercent", got)
	require.NotContains(t, got, samsara.StatTypeFuelPercents,
		"возможность выключена — поле не должно попадать в запрос")
}

func TestStatsSnapshotKeepsTheOldTypesForCallersThatDoNotChoose(t *testing.T) {
	var got string
	client := samsaraTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = capturedTypes(t, r)
		fmt.Fprint(w, `{"data":[],"pagination":{"hasNextPage":false}}`)
	})

	_, err := client.GetAllVehiclesStats(context.Background())
	require.NoError(t, err)
	require.Equal(t, "gps,fuelPercents,obdOdometerMeters,gpsOdometerMeters", got)
}

func TestStatsFeedReadsDefFluidAsMilliPercent(t *testing.T) {
	var got string
	client := samsaraTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = capturedTypes(t, r)
		fmt.Fprint(w, `{"data":[{"id":"212","name":"T-1",
			"gps":[{"time":"2026-09-28T10:00:00Z","latitude":41.1,"longitude":-74.2}],
			"defFluidMilliPercent":[{"time":"2026-09-28T09:00:00Z","value":41000},
			                        {"time":"2026-09-28T10:00:00Z","value":52500}]}],
			"pagination":{"hasNextPage":false,"endCursor":"c1"}}`)
	})

	result, err := client.GetVehicleStatsFeedWithTypes(context.Background(), "",
		[]string{samsara.StatTypeGPS, samsara.StatTypeDefFluidMilliPercent})
	require.NoError(t, err)
	require.Equal(t, "gps,defFluidMilliPercent", got)
	require.Len(t, result.Data, 1)
	require.NotNil(t, result.Data[0].DefFluidMilliPercent)
	// Милли-проценты отдаются как есть; в проценты их переводит сервис.
	require.InDelta(t, 52500.0, result.Data[0].DefFluidMilliPercent.Value, 0.001)
}

func TestStatsFeedLeavesDefEmptyWhenTheTruckHasNoSensor(t *testing.T) {
	client := samsaraTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"data":[{"id":"212","name":"T-1",
			"gps":[{"time":"2026-09-28T10:00:00Z","latitude":41.1,"longitude":-74.2}]}],
			"pagination":{"hasNextPage":false}}`)
	})

	result, err := client.GetVehicleStatsFeedWithTypes(context.Background(), "", []string{samsara.StatTypeGPS})
	require.NoError(t, err)
	require.Len(t, result.Data, 1)
	require.Nil(t, result.Data[0].DefFluidMilliPercent, "нет датчика — пусто, а не ноль")
}

func TestPlanRefusalIsNotTreatedAsABadKey(t *testing.T) {
	client := samsaraTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"No access to required licenses"}`)
	})

	_, err := client.GetAllVehiclesStatsWithTypes(context.Background(),
		[]string{samsara.StatTypeGPS, samsara.StatTypeDefFluidMilliPercent})
	require.Error(t, err)
	require.True(t, samsara.IsCapabilityError(err))
	require.Equal(t, "No access to required licenses", samsara.RefusalMessage(err))
}

func TestBadKeyIsNotTreatedAsAPlanRefusal(t *testing.T) {
	client := samsaraTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"message":"Invalid token"}`)
	})

	_, err := client.GetAllVehiclesStats(context.Background())
	require.Error(t, err)
	require.True(t, samsara.IsAuthError(err))
	require.False(t, samsara.IsCapabilityError(err),
		"401 — это сломанный ключ, интеграцию по нему выключать можно")
}
