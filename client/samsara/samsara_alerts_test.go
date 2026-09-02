package samsara

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newAlertTestClient поднимает фейковую Samsara и отдаёт клиент, который в неё смотрит.
// handler получает запросы как есть — тесты проверяют в том числе путь с курсором.
func newAlertTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return &Client{
		httpClient: srv.Client(),
		host:       srv.URL,
		apiKey:     "SECRET-KEY-DO-NOT-LOG",
	}
}

// TestTriggerTypeIDsMatchSamsaraDocs пиннит те id, которые мы реально отправляем в
// Samsara. Раньше пять из них были неверными, и заметить это было нечем: неверный
// id принимается API молча, а алерт просто ловит не то. DEV-1989.
func TestTriggerTypeIDsMatchSamsaraDocs(t *testing.T) {
	want := map[string]struct {
		got  int
		name string
	}{
		"TriggerTypeMovement":        {TriggerTypeMovement, "Asset starts moving"},
		"TriggerTypeInsideGeofence":  {TriggerTypeInsideGeofence, "Inside Geofence"},
		"TriggerTypeEngineIdle":      {TriggerTypeEngineIdle, "Vehicle Engine Idle"},
		"TriggerTypeOutsideGeofence": {TriggerTypeOutsideGeofence, "Outside Geofence"},
		"TriggerTypeEngineOn":        {TriggerTypeEngineOn, "Asset Engine On"},
		"TriggerTypeEngineOff":       {TriggerTypeEngineOff, "Asset Engine Off"},
		"TriggerTypeHarshEvent":      {TriggerTypeHarshEvent, "Harsh Event"},
		"TriggerTypeFaultCode":       {TriggerTypeFaultCode, "Fault Code"},
	}
	expect := map[string]int{
		"TriggerTypeMovement":        1013,
		"TriggerTypeInsideGeofence":  1014,
		"TriggerTypeEngineIdle":      1019,
		"TriggerTypeOutsideGeofence": 1020,
		"TriggerTypeEngineOn":        1021,
		"TriggerTypeEngineOff":       1022,
		"TriggerTypeHarshEvent":      1023,
		"TriggerTypeFaultCode":       1029,
	}

	for constName, w := range want {
		if w.got != expect[constName] {
			t.Errorf("%s = %d, а %q у Samsara это %d", constName, w.got, w.name, expect[constName])
		}
	}

	// Отдельно: idle и геозона не должны снова разъехаться друг с другом.
	if TriggerTypeEngineIdle == TriggerTypeOutsideGeofence {
		t.Fatal("EngineIdle и OutsideGeofence не могут быть одним id")
	}
}

func TestGetAllAlertConfigurations_WalksEveryPage(t *testing.T) {
	calls := 0
	client := newAlertTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if got := r.URL.Path; got != "/alerts/configurations" {
			t.Errorf("путь = %q, ожидали /alerts/configurations", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("after") {
		case "":
			_, _ = w.Write([]byte(`{"data":[{"id":"a","name":"[TMS360] alert 1023"}],
				"pagination":{"endCursor":"cur-2","hasNextPage":true}}`))
		case "cur-2":
			_, _ = w.Write([]byte(`{"data":[{"id":"b","name":"Idle > 15 min"}],
				"pagination":{"endCursor":"","hasNextPage":false}}`))
		default:
			t.Errorf("неожиданный курсор %q", r.URL.Query().Get("after"))
		}
	})

	got, err := client.GetAllAlertConfigurations(context.Background())
	if err != nil {
		t.Fatalf("GetAllAlertConfigurations: %v", err)
	}
	if calls != 2 {
		t.Fatalf("запросов = %d, ожидали 2 (две страницы)", calls)
	}
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "b" {
		t.Fatalf("страницы склеены неверно: %+v", got)
	}
}

// Samsara может отдать hasNextPage:true с пустым курсором. Без защиты это вечный цикл.
func TestGetAllAlertConfigurations_StopsOnEmptyCursor(t *testing.T) {
	calls := 0
	client := newAlertTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls > 3 {
			t.Fatal("зациклились на пустом курсоре")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"a"}],"pagination":{"endCursor":"","hasNextPage":true}}`))
	})

	if _, err := client.GetAllAlertConfigurations(context.Background()); err != nil {
		t.Fatalf("GetAllAlertConfigurations: %v", err)
	}
	if calls != 1 {
		t.Fatalf("запросов = %d, ожидали 1", calls)
	}
}

// 404 на удаление означает "уже удалено" — уборка не должна на этом падать.
func TestDeleteAlertConfiguration_404IsNotAnError(t *testing.T) {
	client := newAlertTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"not found"}`))
	})

	if err := client.DeleteAlertConfiguration(context.Background(), "gone"); err != nil {
		t.Fatalf("404 должен считаться успехом, получили: %v", err)
	}
}

func TestDeleteAlertConfiguration_500StillFails(t *testing.T) {
	client := newAlertTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	if err := client.DeleteAlertConfiguration(context.Background(), "boom"); err == nil {
		t.Fatal("500 должен возвращать ошибку")
	}
}

func TestGetAllVehicles_WalksEveryPage(t *testing.T) {
	calls := 0
	client := newAlertTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("after") == "" {
			_, _ = w.Write([]byte(`{"data":[{"id":"1","name":"T-1","vin":"VIN1"}],
				"pagination":{"endCursor":"p2","hasNextPage":true}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"2","name":"T-2","vin":"VIN2"}],
			"pagination":{"endCursor":"","hasNextPage":false}}`))
	})

	got, err := client.GetAllVehicles(context.Background())
	if err != nil {
		t.Fatalf("GetAllVehicles: %v", err)
	}
	if calls != 2 || len(got) != 2 || got[1].Vin != "VIN2" {
		t.Fatalf("страницы машин склеены неверно (%d запросов): %+v", calls, got)
	}
}
