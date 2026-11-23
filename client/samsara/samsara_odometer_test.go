package samsara

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newStatsTestClient поднимает фейковую Samsara и запоминает путь последнего
// запроса: тесты проверяют не только разбор ответа, но и то, что мы вообще
// попросили одометр.
func newStatsTestClient(t *testing.T, body string, gotPath *string) *Client {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*gotPath = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	return &Client{httpClient: srv.Client(), host: srv.URL, apiKey: "SECRET-KEY-DO-NOT-LOG"}
}

// TestStatsRequestsOdometerTypes: одометр едет тем же запросом, что и GPS. Если
// типы выпадут из строки запроса, Samsara молча вернёт ответ без одометра и
// пробег просто перестанет обновляться — ошибки не будет (DEV-2251).
func TestStatsRequestsOdometerTypes(t *testing.T) {
	for _, tc := range []struct {
		name string
		call func(c *Client) error
	}{
		{"snapshot", func(c *Client) error {
			_, err := c.GetAllVehiclesStats(context.Background())
			return err
		}},
		{"feed", func(c *Client) error {
			_, err := c.GetVehicleStatsFeed(context.Background(), "")
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var path string
			c := newStatsTestClient(t, `{"data":[],"pagination":{}}`, &path)

			if err := tc.call(c); err != nil {
				t.Fatalf("call: %v", err)
			}
			for _, want := range []string{"gps", "fuelPercents", "obdOdometerMeters", "gpsOdometerMeters"} {
				if !strings.Contains(path, want) {
					t.Errorf("path %q does not ask for %q", path, want)
				}
			}
		})
	}
}

// TestSnapshotParsesBothOdometers: obd и gps разбираются оба — выбор источника
// живёт у потребителя, клиент ничего не отбрасывает.
func TestSnapshotParsesBothOdometers(t *testing.T) {
	body := `{"data":[{"id":"281","name":"T-1","vin":"VIN1",
		"gps":{"time":"2026-09-14T10:00:00Z","latitude":1,"longitude":2},
		"obdOdometerMeters":{"time":"2026-09-14T10:00:00Z","value":345000000},
		"gpsOdometerMeters":{"time":"2026-09-14T09:55:00Z","value":344900000}}],"pagination":{}}`

	var path string
	c := newStatsTestClient(t, body, &path)

	got, err := c.GetAllVehiclesStats(context.Background())
	if err != nil {
		t.Fatalf("GetAllVehiclesStats: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 vehicle, got %d", len(got))
	}
	if got[0].ObdOdometerMeters == nil || got[0].ObdOdometerMeters.Value != 345000000 {
		t.Errorf("obd odometer not parsed: %+v", got[0].ObdOdometerMeters)
	}
	if got[0].GpsOdometerMeters == nil || got[0].GpsOdometerMeters.Value != 344900000 {
		t.Errorf("gps odometer not parsed: %+v", got[0].GpsOdometerMeters)
	}
}

// TestFeedKeepsVehicleWithOdometerButNoGPS: грузовик на стоянке отдаёт одометр
// без свежей GPS-точки. Раньше такая машина отбрасывалась целиком, и показание
// терялось молча (DEV-2251).
func TestFeedKeepsVehicleWithOdometerButNoGPS(t *testing.T) {
	body := `{"data":[
		{"id":"281","name":"PARKED","gps":[],
		 "obdOdometerMeters":[{"time":"2026-09-14T10:00:00Z","value":345000000}]},
		{"id":"282","name":"SILENT","gps":[]}
	],"pagination":{}}`

	var path string
	c := newStatsTestClient(t, body, &path)

	res, err := c.GetVehicleStatsFeed(context.Background(), "")
	if err != nil {
		t.Fatalf("GetVehicleStatsFeed: %v", err)
	}
	if len(res.Data) != 1 {
		t.Fatalf("want only the vehicle that reported something, got %d entries", len(res.Data))
	}
	v := res.Data[0]
	if v.ID != "281" {
		t.Fatalf("want vehicle 281, got %q", v.ID)
	}
	if v.Gps != nil {
		t.Errorf("want nil Gps for an odometer-only entry, got %+v", v.Gps)
	}
	if v.ObdOdometerMeters == nil || v.ObdOdometerMeters.Value != 345000000 {
		t.Errorf("odometer lost: %+v", v.ObdOdometerMeters)
	}
}

// TestFeedTakesLatestOdometerPerPoint: за окно приходит несколько показаний,
// берём последнее, и оно висит на каждой GPS-точке машины.
func TestFeedTakesLatestOdometerPerPoint(t *testing.T) {
	body := `{"data":[{"id":"281","name":"T-1",
		"gps":[{"time":"2026-09-14T09:50:00Z","latitude":1,"longitude":2},
		       {"time":"2026-09-14T10:00:00Z","latitude":3,"longitude":4}],
		"obdOdometerMeters":[{"time":"2026-09-14T09:50:00Z","value":344000000},
		                     {"time":"2026-09-14T10:00:00Z","value":345000000}]}],"pagination":{}}`

	var path string
	c := newStatsTestClient(t, body, &path)

	res, err := c.GetVehicleStatsFeed(context.Background(), "")
	if err != nil {
		t.Fatalf("GetVehicleStatsFeed: %v", err)
	}
	if len(res.Data) != 2 {
		t.Fatalf("want one entry per GPS point, got %d", len(res.Data))
	}
	for i, v := range res.Data {
		if v.ObdOdometerMeters == nil || v.ObdOdometerMeters.Value != 345000000 {
			t.Errorf("entry %d: want latest odometer 345000000, got %+v", i, v.ObdOdometerMeters)
		}
	}
}
