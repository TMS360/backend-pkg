package samsara

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/TMS360/backend-pkg/config"
)

// VehicleInfo - информация о транспортном средстве
type VehicleInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Vin  string `json:"vin"`
}

type VehicleListResponse struct {
	Data []VehicleInfo `json:"data"`
}

// GpsCoordinates - GPS координаты транспорта
type GpsCoordinates struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Time      string  `json:"time"`
	Heading   float64 `json:"heading,omitempty"`           // Направление движения
	Speed     float64 `json:"speedMilesPerHour,omitempty"` // Скорость в милях/час
}

// VehicleLocation - местоположение транспорта с GPS
type VehicleLocation struct {
	ID   string          `json:"id"`
	Name string          `json:"name"`
	Vin  string          `json:"vin,omitempty"`
	Gps  *GpsCoordinates `json:"gps"`
}

type VehicleLocationResponse struct {
	Data []VehicleLocation `json:"data"`
}

// ============================================================================
// СТРУКТУРЫ ДЛЯ ДАТЧИКОВ ТЕМПЕРАТУРЫ/ВЛАЖНОСТИ
// ============================================================================

// SensorInfo - информация о датчике
type SensorInfo struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Serial     string `json:"serial,omitempty"`
	MacAddress string `json:"macAddress,omitempty"`
}

// TemperatureData - данные температуры
type TemperatureData struct {
	AmbientTemperatureMilliC int    `json:"ambientTemperatureMilliC"`         // Температура в милли-Цельсиях
	ProbeTemperatureMilliC   int    `json:"probeTemperatureMilliC,omitempty"` // Температура щупа
	Time                     int64  `json:"time"`                             // Unix timestamp в миллисекундах
	Name                     string `json:"name"`
	ID                       int64  `json:"id"`
}

// HumidityData - данные влажности
type HumidityData struct {
	HumidityPercent int    `json:"humidityPercent"` // Влажность в процентах
	Time            int64  `json:"time"`
	Name            string `json:"name"`
	ID              int64  `json:"id"`
}

// SensorHistoryData - исторические данные датчика
type SensorHistoryData struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Series []struct {
		Field  string `json:"field"` // "ambientTemperature", "probeTemperature", "humidity"
		Values []struct {
			Value int   `json:"value"`
			Time  int64 `json:"timeMs"`
		} `json:"values"`
	} `json:"series"`
}

// TemperatureResponse - ответ на запрос температуры
type TemperatureResponse struct {
	Sensors []TemperatureData `json:"sensors"`
}

// HumidityResponse - ответ на запрос влажности
type HumidityResponse struct {
	Sensors []HumidityData `json:"sensors"`
}

// SensorListResponse - список датчиков
type SensorListResponse struct {
	Sensors []SensorInfo `json:"sensors"`
}

// ============================================================================
// СТРУКТУРЫ ДЛЯ GEOFENCING (ГЕОЗОНЫ)
// ============================================================================

// CircleGeofence - круговая геозона
type CircleGeofence struct {
	RadiusMeters int     `json:"radiusMeters"`
	Latitude     float64 `json:"latitude,omitempty"`
	Longitude    float64 `json:"longitude,omitempty"`
}

// PolygonVertex - вершина полигона
type PolygonVertex struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// PolygonGeofence - полигональная геозона
type PolygonGeofence struct {
	Vertices []PolygonVertex `json:"vertices"`
}

// Geofence - геозона (круг или полигон)
type Geofence struct {
	Circle  *CircleGeofence  `json:"circle,omitempty"`
	Polygon *PolygonGeofence `json:"polygon,omitempty"`
}

// Address - адрес с геозоной
type Address struct {
	ID               string            `json:"id,omitempty"`
	Name             string            `json:"name"`
	FormattedAddress string            `json:"formattedAddress"`
	Geofence         Geofence          `json:"geofence"`
	Latitude         float64           `json:"latitude,omitempty"`
	Longitude        float64           `json:"longitude,omitempty"`
	CreatedAtTime    string            `json:"createdAtTime,omitempty"`
	ExternalIDs      map[string]string `json:"externalIds,omitempty"`
}

type AddressResponse struct {
	Data Address `json:"data"`
}

type AddressListResponse struct {
	Data       []Address  `json:"data"`
	Pagination Pagination `json:"pagination,omitempty"`
}

type Pagination struct {
	EndCursor   string `json:"endCursor"`
	HasNextPage bool   `json:"hasNextPage"`
}

// WebhookCustomHeader - кастомные заголовки для webhook
type WebhookCustomHeader struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// WebhookDefinition - конфигурация webhook
type WebhookDefinition struct {
	ID            string                `json:"id,omitempty"`
	Name          string                `json:"name"`
	URL           string                `json:"url"`
	Version       string                `json:"version,omitempty"` // e.g., "2018-01-01"
	EventTypes    []string              `json:"eventTypes,omitempty"`
	CustomHeaders []WebhookCustomHeader `json:"customHeaders,omitempty"`
	Enabled       bool                  `json:"enabled,omitempty"`
	CreatedAtTime string                `json:"createdAtTime,omitempty"`
	UpdatedAtTime string                `json:"updatedAtTime,omitempty"`
}

type WebhookResponse struct {
	Data WebhookDefinition `json:"data"`
}

type WebhookListResponse struct {
	Data       []WebhookDefinition `json:"data"`
	Pagination Pagination          `json:"pagination,omitempty"`
}

// WebhookEventType - типы событий webhook
type WebhookEventType string

const (
	EventTypeAlert          WebhookEventType = "Alert"
	EventTypeAddressCreated WebhookEventType = "AddressCreated"
	EventTypeAddressUpdated WebhookEventType = "AddressUpdated"
	EventTypeAddressDeleted WebhookEventType = "AddressDeleted"
	EventTypeVehicleUpdated WebhookEventType = "VehicleUpdated"
	EventTypeDriverUpdated  WebhookEventType = "DriverUpdated"
)

// AlertConditionID - типы условий для алертов
type AlertConditionID string

const (
	AlertConditionDeviceLocationInsideGeofence  AlertConditionID = "DeviceLocationInsideGeofence"
	AlertConditionDeviceLocationOutsideGeofence AlertConditionID = "DeviceLocationOutsideGeofence"
	AlertConditionDeviceMovement                AlertConditionID = "DeviceMovement"
	AlertConditionDeviceSpeedAbove              AlertConditionID = "DeviceSpeedAbove"
	AlertConditionDeviceSpeedAboveSpeedLimit    AlertConditionID = "DeviceSpeedAboveSpeedLimit"
	AlertConditionEngineIdle                    AlertConditionID = "EngineIdle"
	AlertConditionDeviceHasVehicleFault         AlertConditionID = "DeviceHasVehicleFault"
	AlertConditionDeviceUnplugged               AlertConditionID = "DeviceUnplugged"
	AlertConditionHarshEvent                    AlertConditionID = "HarshEvent"
	AlertConditionTemperatureAbove              AlertConditionID = "TemperatureAbove"
	AlertConditionTemperatureBelow              AlertConditionID = "TemperatureBelow"
	AlertConditionHumidityAbove                 AlertConditionID = "HumidityAbove"
	AlertConditionHumidityBelow                 AlertConditionID = "HumidityBelow"
	AlertConditionDoorOpen                      AlertConditionID = "DoorActivated"
	AlertConditionDoorClosed                    AlertConditionID = "DoorDeactivated"
	AlertConditionReeferTemperatureAbove        AlertConditionID = "ReeferTemperatureAboveSetPoint"
	AlertConditionReeferTemperatureBelow        AlertConditionID = "ReeferTemperatureBelowSetPoint"
)

// WebhookEvent - событие webhook от Samsara
type WebhookEvent struct {
	EventID   string           `json:"eventId"`
	EventMs   int64            `json:"eventMs"`
	EventType WebhookEventType `json:"eventType"`
	OrgID     int              `json:"orgId,omitempty"`
	WebhookID string           `json:"webhookId,omitempty"`
	Event     json.RawMessage  `json:"event"`
}

// AlertEvent - событие алерта
type AlertEvent struct {
	AlertEventURL             string           `json:"alertEventUrl"`
	AlertConditionDescription string           `json:"alertConditionDescription"`
	AlertConditionID          AlertConditionID `json:"alertConditionId"`
	Details                   string           `json:"details"`
	OrgID                     int              `json:"orgId"`
	Resolved                  bool             `json:"resolved"`
	StartMs                   int64            `json:"startMs"`
	Summary                   string           `json:"summary"`

	Device *struct {
		ID     int64  `json:"id"`
		Name   string `json:"name"`
		Serial string `json:"serial"`
		VIN    string `json:"vin,omitempty"`
	} `json:"device,omitempty"`

	Driver *struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"driver,omitempty"`
}

// Alert Trigger Type IDs
const (
	TriggerTypeInsideGeofence   = 1017 // Внутри геозоны
	TriggerTypeOutsideGeofence  = 1018 // Вне геозоны
	TriggerTypeMovement         = 1019 // Начало движения
	TriggerTypeSpeedAbove       = 1001 // Превышение скорости
	TriggerTypeEngineIdle       = 1020 // Простой двигателя
	TriggerTypeEngineOn         = 1021 // Двигатель включен
	TriggerTypeEngineOff        = 1022 // Двигатель выключен
	TriggerTypeHarshEvent       = 1023 // Резкое событие (торможение/ускорение)
	TriggerTypeFaultCode        = 1031 // Код неисправности
	TriggerTypeTemperatureAbove = 1033 // Температура выше порога
	TriggerTypeTemperatureBelow = 1034 // Температура ниже порога
	TriggerTypeHumidityAbove    = 1035 // Влажность выше порога
	TriggerTypeHumidityBelow    = 1036 // Влажность ниже порога
	TriggerTypeDoorOpen         = 1037 // Дверь открыта
)

// Alert Action Type IDs
const (
	ActionTypeWebhook = 4 // Отправить webhook
	ActionTypeEmail   = 1 // Отправить email
	ActionTypeSMS     = 2 // Отправить SMS
)

// AlertConfiguration - конфигурация алерта
type AlertConfiguration struct {
	ID          string         `json:"id,omitempty"`
	Name        string         `json:"name"`
	IsEnabled   bool           `json:"isEnabled"`
	Description string         `json:"description,omitempty"`
	Scope       AlertScope     `json:"scope"`
	Triggers    []AlertTrigger `json:"triggers"`
	Actions     []AlertAction  `json:"actions"`
	CreatedAt   string         `json:"createdAt,omitempty"`
	UpdatedAt   string         `json:"updatedAt,omitempty"`
}

// AlertScope - к каким транспортам применяется алерт
type AlertScope struct {
	All        bool     `json:"all,omitempty"`        // Все транспорты
	AssetIDs   []string `json:"assetIds,omitempty"`   // Конкретные ассеты
	VehicleIDs []string `json:"vehicleIds,omitempty"` // Конкретные транспорты
	TagIDs     []string `json:"tagIds,omitempty"`     // Транспорты с тегами
}

// AlertTrigger - условие срабатывания алерта
type AlertTrigger struct {
	TriggerTypeID int                    `json:"triggerTypeId"`
	TriggerParams map[string]interface{} `json:"triggerParams"`
}

// AlertAction - действие при срабатывании алерта
type AlertAction struct {
	ActionTypeID int                    `json:"actionTypeId"`
	ActionParams map[string]interface{} `json:"actionParams"`
}

type AlertConfigurationResponse struct {
	Data AlertConfiguration `json:"data"`
}

type AlertConfigurationListResponse struct {
	Data       []AlertConfiguration `json:"data"`
	Pagination Pagination           `json:"pagination,omitempty"`
}

// Client - основной клиент для работы с Samsara API
type Client struct {
	httpClient *http.Client
	host       string
	apiKey     string
}

// NewClient создаёт новый клиент Samsara с конфигурацией
func NewClient(cfg config.SamsaraConfig, apiKey string) (*Client, error) {
	return &Client{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		host:       cfg.Host,
		apiKey:     apiKey,
	}, nil
}

// NewClientWithToken создаёт клиент только с API ключом (использует дефолтный хост)
func NewClientWithToken(apiKey string) (*Client, error) {
	return &Client{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		host:       "https://api.samsara.com",
		apiKey:     apiKey,
	}, nil
}

// doRequest - вспомогательный метод для выполнения HTTP запросов
func (c *Client) doRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.host+path, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return resp, nil
}

// ListVehicles получает список всех транспортных средств
func (c *Client) ListVehicles(ctx context.Context) (vehicles []VehicleInfo, err error) {
	path := "/fleet/vehicles"
	resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close response body: %w", closeErr)
		}
	}()

	var listResponse VehicleListResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return listResponse.Data, nil
}

// GetVehiclesStats получает GPS статистику для указанных транспортов (по Samsara ID, НЕ по VIN!)
func (c *Client) GetVehiclesStats(ctx context.Context, vehicleIDs []int64) (locations []VehicleLocation, err error) {
	if len(vehicleIDs) == 0 {
		return []VehicleLocation{}, nil
	}

	var stringIDs []string
	for _, id := range vehicleIDs {
		stringIDs = append(stringIDs, strconv.FormatInt(id, 10))
	}
	idsParam := strings.Join(stringIDs, ",")

	path := fmt.Sprintf("/fleet/vehicles/stats?types=gps&vehicleIds=%s", idsParam)

	resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close response body: %w", closeErr)
		}
	}()

	var locationResponse VehicleLocationResponse
	if err := json.NewDecoder(resp.Body).Decode(&locationResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return locationResponse.Data, nil
}

// GetVehicleCoordinates получает текущие координаты транспорта (по Samsara ID, НЕ по VIN!)
func (c *Client) GetVehicleCoordinates(ctx context.Context, vehicleID int64) (locations *VehicleLocation, err error) {
	path := fmt.Sprintf("/fleet/vehicles/%d/locations", vehicleID)
	resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close response body: %w", closeErr)
		}
	}()

	var locationResponse VehicleLocation
	if err := json.NewDecoder(resp.Body).Decode(&locationResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &locationResponse, nil
}

// GetVehicleByVIN находит транспорт по VIN номеру (возвращает Samsara ID в поле ID)
func (c *Client) GetVehicleByVIN(ctx context.Context, vin string) (vehicle *VehicleInfo, err error) {
	if vin == "" {
		return nil, fmt.Errorf("VIN cannot be empty")
	}

	path := fmt.Sprintf("/fleet/vehicles?vin=%s", vin)

	resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close response body: %w", closeErr)
		}
	}()

	var vehicleInfo VehicleInfo
	if err := json.NewDecoder(resp.Body).Decode(&vehicleInfo); err != nil {
		return nil, fmt.Errorf("failed to decode response for VIN '%s': %w", vin, err)
	}

	return &vehicleInfo, nil
}

// GetAllVehiclesLocations получает текущее местоположение всех транспортов
func (c *Client) GetAllVehiclesLocations(ctx context.Context) ([]VehicleLocation, error) {
	path := "/fleet/vehicles/locations"

	resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get all vehicles locations: %w", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {

		}
	}(resp.Body)

	var response VehicleLocationResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode vehicles locations response: %w", err)
	}

	return response.Data, nil
}

// GetAllVehiclesLocationsWithTime получает местоположения транспортов за период времени
func (c *Client) GetAllVehiclesLocationsWithTime(ctx context.Context, startTime, endTime time.Time) ([]VehicleLocation, error) {
	startTimeStr := startTime.Format(time.RFC3339)
	endTimeStr := endTime.Format(time.RFC3339)

	path := fmt.Sprintf("/fleet/vehicles/locations?startTime=%s&endTime=%s", startTimeStr, endTimeStr)

	resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get vehicles locations with time filter: %w", err)
	}

	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {

		}
	}(resp.Body)

	var response VehicleLocationResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode vehicles locations response: %w", err)
	}

	return response.Data, nil
}

// ============================================================================
// МЕТОДЫ ДЛЯ РАБОТЫ С ТЕМПЕРАТУРНЫМИ ДАТЧИКАМИ
// ============================================================================

// GetSensors получает список всех датчиков
func (c *Client) GetSensors(ctx context.Context) ([]SensorInfo, error) {
	path := "/v1/sensors"
	resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get sensors: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close response body: %w", closeErr)
		}
	}()

	var response SensorListResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return response.Sensors, nil
}

// GetSensorTemperature получает текущую температуру для указанных датчиков
func (c *Client) GetSensorTemperature(ctx context.Context, sensorIDs []int64) ([]TemperatureData, error) {
	if len(sensorIDs) == 0 {
		return nil, fmt.Errorf("sensor IDs cannot be empty")
	}

	// Legacy API использует другой формат
	body := map[string]interface{}{
		"sensors": sensorIDs,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.host+"/v1/sensors/temperature", strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close response body: %w", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var response TemperatureResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return response.Sensors, nil
}

// GetSensorHumidity получает текущую влажность для указанных датчиков
func (c *Client) GetSensorHumidity(ctx context.Context, sensorIDs []int64) ([]HumidityData, error) {
	if len(sensorIDs) == 0 {
		return nil, fmt.Errorf("sensor IDs cannot be empty")
	}

	body := map[string]interface{}{
		"sensors": sensorIDs,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.host+"/v1/sensors/humidity", strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close response body: %w", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var response HumidityResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return response.Sensors, nil
}

// GetSensorHistory получает исторические данные датчиков за период
func (c *Client) GetSensorHistory(ctx context.Context, sensorID int64, startTime, endTime time.Time, field string) (*SensorHistoryData, error) {
	body := map[string]interface{}{
		"series": []map[string]interface{}{
			{
				"widgetId": sensorID,
				"field":    field, // "ambientTemperature", "probeTemperature", "humidity"
			},
		},
		"startMs":     startTime.UnixMilli(),
		"endMs":       endTime.UnixMilli(),
		"stepMs":      60000, // 1 minute resolution
		"fillMissing": "null",
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.host+"/v1/sensors/history", strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close response body: %w", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var response struct {
		Results []SensorHistoryData `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(response.Results) > 0 {
		return &response.Results[0], nil
	}

	return nil, fmt.Errorf("no history data found")
}

// ConvertMilliCelsiusToFahrenheit конвертирует милли-Цельсии в Фаренгейты
func ConvertMilliCelsiusToFahrenheit(milliC int) float64 {
	celsius := float64(milliC) / 1000.0
	return (celsius * 9.0 / 5.0) + 32.0
}

// ConvertMilliCelsiusToCelsius конвертирует милли-Цельсии в Цельсии
func ConvertMilliCelsiusToCelsius(milliC int) float64 {
	return float64(milliC) / 1000.0
}

// ============================================================================
// МЕТОДЫ ДЛЯ РАБОТЫ С ГЕОЗОНАМИ (ADDRESSES/GEOFENCES)
// ============================================================================

// CreateAddress создаёт новый адрес с геозоной
func (c *Client) CreateAddress(ctx context.Context, address Address) (*Address, error) {
	body, err := json.Marshal(address)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal address: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.host+"/addresses", strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close response body: %w", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var addressResponse AddressResponse
	if err := json.NewDecoder(resp.Body).Decode(&addressResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &addressResponse.Data, nil
}

// GetAddress получает адрес по ID или external ID
func (c *Client) GetAddress(ctx context.Context, addressID string) (*Address, error) {
	path := fmt.Sprintf("/addresses/%s", addressID)
	resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close response body: %w", closeErr)
		}
	}()

	var addressResponse AddressResponse
	if err := json.NewDecoder(resp.Body).Decode(&addressResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &addressResponse.Data, nil
}

// ListAddresses получает список всех адресов с пагинацией
func (c *Client) ListAddresses(ctx context.Context, cursor string) (*AddressListResponse, error) {
	path := "/addresses"
	if cursor != "" {
		path = fmt.Sprintf("%s?after=%s", path, cursor)
	}

	resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close response body: %w", closeErr)
		}
	}()

	var listResponse AddressListResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &listResponse, nil
}

// UpdateAddress обновляет существующий адрес
func (c *Client) UpdateAddress(ctx context.Context, addressID string, updates map[string]interface{}) (*Address, error) {
	bodyBytes, err := json.Marshal(updates)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal updates: %w", err)
	}

	path := fmt.Sprintf("/addresses/%s", addressID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, c.host+path, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close response body: %w", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var addressResponse AddressResponse
	if err := json.NewDecoder(resp.Body).Decode(&addressResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &addressResponse.Data, nil
}

// DeleteAddress удаляет адрес по ID
func (c *Client) DeleteAddress(ctx context.Context, addressID string) error {
	path := fmt.Sprintf("/addresses/%s", addressID)
	resp, err := c.doRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close response body: %w", closeErr)
		}
	}()

	return nil
}

// CreateCircularGeofence создаёт круговую геозону с указанным радиусом
func (c *Client) CreateCircularGeofence(ctx context.Context, name, formattedAddress string, radiusMeters int, lat, lon float64) (*Address, error) {
	address := Address{
		Name:             name,
		FormattedAddress: formattedAddress,
		Geofence: Geofence{
			Circle: &CircleGeofence{
				RadiusMeters: radiusMeters,
			},
		},
	}

	// Добавляем координаты если указаны (только для круговых геозон)
	if lat != 0 && lon != 0 {
		address.Latitude = lat
		address.Longitude = lon
	}

	return c.CreateAddress(ctx, address)
}

// CreatePolygonalGeofence создаёт полигональную геозону
func (c *Client) CreatePolygonalGeofence(ctx context.Context, name, formattedAddress string, vertices []PolygonVertex) (*Address, error) {
	if len(vertices) < 3 {
		return nil, fmt.Errorf("polygon must have at least 3 vertices")
	}

	address := Address{
		Name:             name,
		FormattedAddress: formattedAddress,
		Geofence: Geofence{
			Polygon: &PolygonGeofence{
				Vertices: vertices,
			},
		},
	}

	return c.CreateAddress(ctx, address)
}

// GetAllAddresses получает ВСЕ адреса без пагинации (удобный helper)
func (c *Client) GetAllAddresses(ctx context.Context) ([]Address, error) {
	var allAddresses []Address
	cursor := ""

	for {
		response, err := c.ListAddresses(ctx, cursor)
		if err != nil {
			return nil, err
		}

		allAddresses = append(allAddresses, response.Data...)

		if !response.Pagination.HasNextPage {
			break
		}

		cursor = response.Pagination.EndCursor
	}

	return allAddresses, nil
}

// CreateWebhook создаёт новую webhook подписку в Samsara
func (c *Client) CreateWebhook(ctx context.Context, webhook WebhookDefinition) (*WebhookDefinition, error) {
	if webhook.Version == "" {
		webhook.Version = "2018-01-01"
	}

	body, err := json.Marshal(webhook)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal webhook: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.host+"/webhooks", strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close response body: %w", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var webhookResponse WebhookResponse
	if err := json.NewDecoder(resp.Body).Decode(&webhookResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &webhookResponse.Data, nil
}

// ListWebhooks получает список всех webhook с пагинацией
func (c *Client) ListWebhooks(ctx context.Context, cursor string) (*WebhookListResponse, error) {
	path := "/webhooks"
	if cursor != "" {
		path = fmt.Sprintf("%s?after=%s", path, cursor)
	}

	resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close response body: %w", closeErr)
		}
	}()

	var listResponse WebhookListResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &listResponse, nil
}

// GetWebhook получает webhook по ID
func (c *Client) GetWebhook(ctx context.Context, webhookID string) (*WebhookDefinition, error) {
	path := fmt.Sprintf("/webhooks/%s", webhookID)
	resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close response body: %w", closeErr)
		}
	}()

	var webhookResponse WebhookResponse
	if err := json.NewDecoder(resp.Body).Decode(&webhookResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &webhookResponse.Data, nil
}

// UpdateWebhook обновляет существующий webhook
func (c *Client) UpdateWebhook(ctx context.Context, webhookID string, updates map[string]interface{}) (*WebhookDefinition, error) {
	bodyBytes, err := json.Marshal(updates)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal updates: %w", err)
	}

	path := fmt.Sprintf("/webhooks/%s", webhookID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, c.host+path, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close response body: %w", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var webhookResponse WebhookResponse
	if err := json.NewDecoder(resp.Body).Decode(&webhookResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &webhookResponse.Data, nil
}

// DeleteWebhook удаляет webhook по ID
func (c *Client) DeleteWebhook(ctx context.Context, webhookID string) error {
	path := fmt.Sprintf("/webhooks/%s", webhookID)
	resp, err := c.doRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close response body: %w", closeErr)
		}
	}()

	return nil
}

// CreateGeofenceWebhook создаёт webhook для событий геозон (helper)
func (c *Client) CreateGeofenceWebhook(ctx context.Context, name, url string, customHeaders []WebhookCustomHeader) (*WebhookDefinition, error) {
	webhook := WebhookDefinition{
		Name:    name,
		URL:     url,
		Version: "2018-01-01",
		EventTypes: []string{
			"Alert", // Alert события включают вход/выход из геозоны
		},
		CustomHeaders: customHeaders,
		Enabled:       true,
	}

	return c.CreateWebhook(ctx, webhook)
}

// CreateAlertConfiguration создаёт новую конфигурацию алерта
func (c *Client) CreateAlertConfiguration(ctx context.Context, config AlertConfiguration) (*AlertConfiguration, error) {
	body, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal alert configuration: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.host+"/alerts/configurations", strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close response body: %w", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var alertResponse AlertConfigurationResponse
	if err := json.NewDecoder(resp.Body).Decode(&alertResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &alertResponse.Data, nil
}

// GetAlertConfiguration получает конфигурацию алерта по ID
func (c *Client) GetAlertConfiguration(ctx context.Context, alertID string) (*AlertConfiguration, error) {
	path := fmt.Sprintf("/alerts/configurations/%s", alertID)
	resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close response body: %w", closeErr)
		}
	}()

	var alertResponse AlertConfigurationResponse
	if err := json.NewDecoder(resp.Body).Decode(&alertResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &alertResponse.Data, nil
}

// UpdateAlertConfiguration обновляет существующую конфигурацию алерта
func (c *Client) UpdateAlertConfiguration(ctx context.Context, alertID string, updates map[string]interface{}) (*AlertConfiguration, error) {
	bodyBytes, err := json.Marshal(updates)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal updates: %w", err)
	}

	path := fmt.Sprintf("/alerts/configurations/%s", alertID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, c.host+path, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close response body: %w", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var alertResponse AlertConfigurationResponse
	if err := json.NewDecoder(resp.Body).Decode(&alertResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &alertResponse.Data, nil
}

// DeleteAlertConfiguration удаляет конфигурацию алерта
func (c *Client) DeleteAlertConfiguration(ctx context.Context, alertID string) error {
	path := fmt.Sprintf("/alerts/configurations/%s", alertID)
	resp, err := c.doRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close response body: %w", closeErr)
		}
	}()

	return nil
}

// CreateGeofenceAlert создаёт алерт для входа/выхода из геозоны
func (c *Client) CreateGeofenceAlert(ctx context.Context, name string, addressID string, webhookID string, onEntry bool, applyToAll bool) (*AlertConfiguration, error) {
	triggerTypeID := TriggerTypeInsideGeofence
	if !onEntry {
		triggerTypeID = TriggerTypeOutsideGeofence
	}

	triggerParams := map[string]interface{}{
		"geofence": map[string]interface{}{
			"addressIds": []string{addressID},
		},
	}

	actionParams := map[string]interface{}{
		"webhooks": map[string]interface{}{
			"webhookIds":  []string{webhookID},
			"payloadType": "enriched",
		},
	}

	config := AlertConfiguration{
		Name:      name,
		IsEnabled: true,
		Scope: AlertScope{
			All: applyToAll,
		},
		Triggers: []AlertTrigger{
			{
				TriggerTypeID: triggerTypeID,
				TriggerParams: triggerParams,
			},
		},
		Actions: []AlertAction{
			{
				ActionTypeID: ActionTypeWebhook,
				ActionParams: actionParams,
			},
		},
	}

	return c.CreateAlertConfiguration(ctx, config)
}

// CreateGeofenceAlertWithDuration создаёт алерт с задержкой (например, в геозоне больше 10 минут)
func (c *Client) CreateGeofenceAlertWithDuration(ctx context.Context, name string, addressID string, webhookID string, onEntry bool, durationMs int64, applyToAll bool) (*AlertConfiguration, error) {
	triggerTypeID := TriggerTypeInsideGeofence
	if !onEntry {
		triggerTypeID = TriggerTypeOutsideGeofence
	}

	triggerParams := map[string]interface{}{
		"geofence": map[string]interface{}{
			"addressIds":              []string{addressID},
			"minDurationMilliseconds": durationMs,
		},
	}

	actionParams := map[string]interface{}{
		"webhooks": map[string]interface{}{
			"webhookIds":  []string{webhookID},
			"payloadType": "enriched",
		},
	}

	config := AlertConfiguration{
		Name:      name,
		IsEnabled: true,
		Scope: AlertScope{
			All: applyToAll,
		},
		Triggers: []AlertTrigger{
			{
				TriggerTypeID: triggerTypeID,
				TriggerParams: triggerParams,
			},
		},
		Actions: []AlertAction{
			{
				ActionTypeID: ActionTypeWebhook,
				ActionParams: actionParams,
			},
		},
	}

	return c.CreateAlertConfiguration(ctx, config)
}

// CreateTemperatureAlert создаёт алерт для температуры (выше или ниже порога)
func (c *Client) CreateTemperatureAlert(ctx context.Context, name string, sensorID int64, webhookID string, thresholdCelsius float64, isAbove bool, durationMs int64, applyToAll bool) (*AlertConfiguration, error) {
	// Конвертируем Цельсии в милли-Цельсии
	thresholdMilliC := int(thresholdCelsius * 1000)

	triggerTypeID := TriggerTypeTemperatureAbove
	if !isAbove {
		triggerTypeID = TriggerTypeTemperatureBelow
	}

	triggerParams := map[string]interface{}{
		"temperature": map[string]interface{}{
			"sensorIds":               []int64{sensorID},
			"thresholdMilliC":         thresholdMilliC,
			"minDurationMilliseconds": durationMs,
		},
	}

	actionParams := map[string]interface{}{
		"webhooks": map[string]interface{}{
			"webhookIds":  []string{webhookID},
			"payloadType": "enriched",
		},
	}

	config := AlertConfiguration{
		Name:      name,
		IsEnabled: true,
		Scope: AlertScope{
			All: applyToAll,
		},
		Triggers: []AlertTrigger{
			{
				TriggerTypeID: triggerTypeID,
				TriggerParams: triggerParams,
			},
		},
		Actions: []AlertAction{
			{
				ActionTypeID: ActionTypeWebhook,
				ActionParams: actionParams,
			},
		},
	}

	return c.CreateAlertConfiguration(ctx, config)
}

// CreateHumidityAlert создаёт алерт для влажности (выше или ниже порога)
func (c *Client) CreateHumidityAlert(ctx context.Context, name string, sensorID int64, webhookID string, thresholdPercent int, isAbove bool, durationMs int64, applyToAll bool) (*AlertConfiguration, error) {
	triggerTypeID := TriggerTypeHumidityAbove
	if !isAbove {
		triggerTypeID = TriggerTypeHumidityBelow
	}

	triggerParams := map[string]interface{}{
		"humidity": map[string]interface{}{
			"sensorIds":               []int64{sensorID},
			"thresholdPercent":        thresholdPercent,
			"minDurationMilliseconds": durationMs,
		},
	}

	actionParams := map[string]interface{}{
		"webhooks": map[string]interface{}{
			"webhookIds":  []string{webhookID},
			"payloadType": "enriched",
		},
	}

	config := AlertConfiguration{
		Name:      name,
		IsEnabled: true,
		Scope: AlertScope{
			All: applyToAll,
		},
		Triggers: []AlertTrigger{
			{
				TriggerTypeID: triggerTypeID,
				TriggerParams: triggerParams,
			},
		},
		Actions: []AlertAction{
			{
				ActionTypeID: ActionTypeWebhook,
				ActionParams: actionParams,
			},
		},
	}

	return c.CreateAlertConfiguration(ctx, config)
}

// CreateDoorOpenAlert создаёт алерт для открытия двери
func (c *Client) CreateDoorOpenAlert(ctx context.Context, name string, sensorID int64, webhookID string, durationMs int64, applyToAll bool) (*AlertConfiguration, error) {
	triggerParams := map[string]interface{}{
		"door": map[string]interface{}{
			"sensorIds":               []int64{sensorID},
			"minDurationMilliseconds": durationMs,
		},
	}

	actionParams := map[string]interface{}{
		"webhooks": map[string]interface{}{
			"webhookIds":  []string{webhookID},
			"payloadType": "enriched",
		},
	}

	config := AlertConfiguration{
		Name:      name,
		IsEnabled: true,
		Scope: AlertScope{
			All: applyToAll,
		},
		Triggers: []AlertTrigger{
			{
				TriggerTypeID: TriggerTypeDoorOpen,
				TriggerParams: triggerParams,
			},
		},
		Actions: []AlertAction{
			{
				ActionTypeID: ActionTypeWebhook,
				ActionParams: actionParams,
			},
		},
	}

	return c.CreateAlertConfiguration(ctx, config)
}

// ============================================================================
// WEBHOOK HANDLER ДЛЯ ОБРАБОТКИ ВХОДЯЩИХ СОБЫТИЙ
// ============================================================================

// WebhookHandler обрабатывает входящие webhook события от Samsara
type WebhookHandler struct {
	// Callbacks для транспортных событий
	OnGeofenceEntry func(event *AlertEvent) error
	OnGeofenceExit  func(event *AlertEvent) error
	OnMovement      func(event *AlertEvent) error
	OnSpeeding      func(event *AlertEvent) error
	OnEngineIdle    func(event *AlertEvent) error
	OnVehicleFault  func(event *AlertEvent) error
	OnHarshEvent    func(event *AlertEvent) error

	// Callbacks для температурных/сенсорных событий
	OnTemperatureAbove func(event *AlertEvent) error
	OnTemperatureBelow func(event *AlertEvent) error
	OnHumidityAbove    func(event *AlertEvent) error
	OnHumidityBelow    func(event *AlertEvent) error
	OnDoorOpen         func(event *AlertEvent) error
	OnDoorClosed       func(event *AlertEvent) error
	OnReeferTempAbove  func(event *AlertEvent) error
	OnReeferTempBelow  func(event *AlertEvent) error

	// Callbacks для адресных событий
	OnAddressCreated func(address *Address) error
	OnAddressUpdated func(address *Address) error
	OnAddressDeleted func(addressID string) error
}

// NewWebhookHandler создаёт новый обработчик webhook
func NewWebhookHandler() *WebhookHandler {
	return &WebhookHandler{}
}

// ParseWebhookRequest парсит входящий webhook запрос
func (h *WebhookHandler) ParseWebhookRequest(body []byte) (*WebhookEvent, error) {
	var event WebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return nil, fmt.Errorf("failed to parse webhook event: %w", err)
	}
	return &event, nil
}

// HandleWebhook обрабатывает входящее webhook событие
func (h *WebhookHandler) HandleWebhook(event *WebhookEvent) error {
	switch event.EventType {
	case EventTypeAlert:
		return h.handleAlertEvent(event)
	case EventTypeAddressCreated:
		if h.OnAddressCreated != nil {
			var data struct {
				Address Address `json:"address"`
			}
			if err := json.Unmarshal(event.Event, &data); err != nil {
				return fmt.Errorf("failed to parse address event: %w", err)
			}
			return h.OnAddressCreated(&data.Address)
		}
	case EventTypeAddressUpdated:
		if h.OnAddressUpdated != nil {
			var data struct {
				Address Address `json:"address"`
			}
			if err := json.Unmarshal(event.Event, &data); err != nil {
				return fmt.Errorf("failed to parse address event: %w", err)
			}
			return h.OnAddressUpdated(&data.Address)
		}
	case EventTypeAddressDeleted:
		if h.OnAddressDeleted != nil {
			var data struct {
				AddressID string `json:"addressId"`
			}
			if err := json.Unmarshal(event.Event, &data); err != nil {
				return fmt.Errorf("failed to parse address deletion: %w", err)
			}
			return h.OnAddressDeleted(data.AddressID)
		}
	}
	return nil
}

// handleAlertEvent обрабатывает Alert события
func (h *WebhookHandler) handleAlertEvent(event *WebhookEvent) error {
	var alertEvent AlertEvent
	if err := json.Unmarshal(event.Event, &alertEvent); err != nil {
		return fmt.Errorf("failed to parse alert event: %w", err)
	}

	switch alertEvent.AlertConditionID {
	case AlertConditionDeviceLocationInsideGeofence:
		if h.OnGeofenceEntry != nil {
			return h.OnGeofenceEntry(&alertEvent)
		}
	case AlertConditionDeviceLocationOutsideGeofence:
		if h.OnGeofenceExit != nil {
			return h.OnGeofenceExit(&alertEvent)
		}
	case AlertConditionDeviceMovement:
		if h.OnMovement != nil {
			return h.OnMovement(&alertEvent)
		}
	case AlertConditionDeviceSpeedAbove, AlertConditionDeviceSpeedAboveSpeedLimit:
		if h.OnSpeeding != nil {
			return h.OnSpeeding(&alertEvent)
		}
	case AlertConditionEngineIdle:
		if h.OnEngineIdle != nil {
			return h.OnEngineIdle(&alertEvent)
		}
	case AlertConditionDeviceHasVehicleFault:
		if h.OnVehicleFault != nil {
			return h.OnVehicleFault(&alertEvent)
		}
	case AlertConditionHarshEvent:
		if h.OnHarshEvent != nil {
			return h.OnHarshEvent(&alertEvent)
		}
	// Temperature-related alerts
	case AlertConditionTemperatureAbove:
		if h.OnTemperatureAbove != nil {
			return h.OnTemperatureAbove(&alertEvent)
		}
	case AlertConditionTemperatureBelow:
		if h.OnTemperatureBelow != nil {
			return h.OnTemperatureBelow(&alertEvent)
		}
	case AlertConditionHumidityAbove:
		if h.OnHumidityAbove != nil {
			return h.OnHumidityAbove(&alertEvent)
		}
	case AlertConditionHumidityBelow:
		if h.OnHumidityBelow != nil {
			return h.OnHumidityBelow(&alertEvent)
		}
	case AlertConditionReeferTemperatureAbove:
		if h.OnReeferTempAbove != nil {
			return h.OnReeferTempAbove(&alertEvent)
		}
	case AlertConditionReeferTemperatureBelow:
		if h.OnReeferTempBelow != nil {
			return h.OnReeferTempBelow(&alertEvent)
		}
	case AlertConditionDoorOpen:
		if h.OnDoorOpen != nil {
			return h.OnDoorOpen(&alertEvent)
		}
	case AlertConditionDoorClosed:
		if h.OnDoorClosed != nil {
			return h.OnDoorClosed(&alertEvent)
		}
	}

	return nil
}

// HTTPHandler создаёт HTTP handler для webhook endpoint
func (h *WebhookHandler) HTTPHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		event, err := h.ParseWebhookRequest(body)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to parse webhook: %v", err), http.StatusBadRequest)
			return
		}

		if err := h.HandleWebhook(event); err != nil {
			http.Error(w, fmt.Sprintf("Failed to handle webhook: %v", err), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}
}

// GeofenceMonitor предоставляет удобные функции для мониторинга геозон
type GeofenceMonitor struct {
	client  *Client
	handler *WebhookHandler
}

// NewGeofenceMonitor создаёт новый монитор геозон
func NewGeofenceMonitor(client *Client) *GeofenceMonitor {
	return &GeofenceMonitor{
		client:  client,
		handler: NewWebhookHandler(),
	}
}

// OnVehicleEntersGeofence устанавливает callback для входа в геозону
func (m *GeofenceMonitor) OnVehicleEntersGeofence(callback func(deviceID int64, deviceName, vin string, details string, timestamp time.Time) error) {
	m.handler.OnGeofenceEntry = func(event *AlertEvent) error {
		timestamp := time.Unix(event.StartMs/1000, (event.StartMs%1000)*1000000)
		if event.Device != nil {
			return callback(event.Device.ID, event.Device.Name, event.Device.VIN, event.Details, timestamp)
		}
		return fmt.Errorf("no device information in geofence entry event")
	}
}

// OnVehicleExitsGeofence устанавливает callback для выхода из геозоны
func (m *GeofenceMonitor) OnVehicleExitsGeofence(callback func(deviceID int64, deviceName, vin string, details string, timestamp time.Time) error) {
	m.handler.OnGeofenceExit = func(event *AlertEvent) error {
		timestamp := time.Unix(event.StartMs/1000, (event.StartMs%1000)*1000000)
		if event.Device != nil {
			return callback(event.Device.ID, event.Device.Name, event.Device.VIN, event.Details, timestamp)
		}
		return fmt.Errorf("no device information in geofence exit event")
	}
}

// GetWebhookHandler возвращает webhook handler
func (m *GeofenceMonitor) GetWebhookHandler() *WebhookHandler {
	return m.handler
}

// CreateFullGeofenceSetup создаёт полную настройку геозоны за один вызов
func CreateFullGeofenceSetup(client *Client, ctx context.Context, name, address string, radius int, webhookURL string) error {
	// 1. Создаём геозону
	geofence, err := client.CreateCircularGeofence(ctx, name, address, radius, 0, 0)
	if err != nil {
		return fmt.Errorf("failed to create geofence: %w", err)
	}

	// 2. Создаём webhook
	webhook, err := client.CreateGeofenceWebhook(ctx, name+" Webhook", webhookURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create webhook: %w", err)
	}

	// 3. Создаём алерт для входа
	_, err = client.CreateGeofenceAlert(ctx, name+" Entry", geofence.ID, webhook.ID, true, true)
	if err != nil {
		return fmt.Errorf("failed to create entry alert: %w", err)
	}

	// 4. Создаём алерт для выхода
	_, err = client.CreateGeofenceAlert(ctx, name+" Exit", geofence.ID, webhook.ID, false, true)
	if err != nil {
		return fmt.Errorf("failed to create exit alert: %w", err)
	}

	return nil
}

// TemperatureMonitor предоставляет удобные функции для мониторинга температуры
type TemperatureMonitor struct {
	client  *Client
	handler *WebhookHandler
}

// NewTemperatureMonitor создаёт новый монитор температуры
func NewTemperatureMonitor(client *Client) *TemperatureMonitor {
	return &TemperatureMonitor{
		client:  client,
		handler: NewWebhookHandler(),
	}
}

// OnTemperatureExceedsThreshold устанавливает callback для превышения температуры
func (m *TemperatureMonitor) OnTemperatureExceedsThreshold(callback func(sensorID int64, sensorName string, tempCelsius float64, details string, timestamp time.Time) error) {
	m.handler.OnTemperatureAbove = func(event *AlertEvent) error {
		timestamp := time.Unix(event.StartMs/1000, (event.StartMs%1000)*1000000)
		// Парсим температуру из details или других полей
		// В реальном случае Samsara может передавать дополнительные данные в event
		if event.Device != nil {
			// Преобразуем milliC в Celsius для удобства
			tempCelsius := 0.0 // В реальности нужно извлечь из event
			return callback(event.Device.ID, event.Device.Name, tempCelsius, event.Details, timestamp)
		}
		return fmt.Errorf("no sensor information in temperature event")
	}
}

// OnTemperatureBelowThreshold устанавливает callback для падения температуры ниже порога
func (m *TemperatureMonitor) OnTemperatureBelowThreshold(callback func(sensorID int64, sensorName string, tempCelsius float64, details string, timestamp time.Time) error) {
	m.handler.OnTemperatureBelow = func(event *AlertEvent) error {
		timestamp := time.Unix(event.StartMs/1000, (event.StartMs%1000)*1000000)
		if event.Device != nil {
			tempCelsius := 0.0 // В реальности нужно извлечь из event
			return callback(event.Device.ID, event.Device.Name, tempCelsius, event.Details, timestamp)
		}
		return fmt.Errorf("no sensor information in temperature event")
	}
}

// OnHumidityExceedsThreshold устанавливает callback для превышения влажности
func (m *TemperatureMonitor) OnHumidityExceedsThreshold(callback func(sensorID int64, sensorName string, humidityPercent float64, details string, timestamp time.Time) error) {
	m.handler.OnHumidityAbove = func(event *AlertEvent) error {
		timestamp := time.Unix(event.StartMs/1000, (event.StartMs%1000)*1000000)
		if event.Device != nil {
			humidityPercent := 0.0 // В реальности нужно извлечь из event
			return callback(event.Device.ID, event.Device.Name, humidityPercent, event.Details, timestamp)
		}
		return fmt.Errorf("no sensor information in humidity event")
	}
}

// OnDoorOpen устанавливает callback для открытия двери
func (m *TemperatureMonitor) OnDoorOpen(callback func(sensorID int64, sensorName string, details string, timestamp time.Time) error) {
	m.handler.OnDoorOpen = func(event *AlertEvent) error {
		timestamp := time.Unix(event.StartMs/1000, (event.StartMs%1000)*1000000)
		if event.Device != nil {
			return callback(event.Device.ID, event.Device.Name, event.Details, timestamp)
		}
		return fmt.Errorf("no sensor information in door event")
	}
}

// OnDoorClosed устанавливает callback для закрытия двери
func (m *TemperatureMonitor) OnDoorClosed(callback func(sensorID int64, sensorName string, details string, timestamp time.Time) error) {
	m.handler.OnDoorClosed = func(event *AlertEvent) error {
		timestamp := time.Unix(event.StartMs/1000, (event.StartMs%1000)*1000000)
		if event.Device != nil {
			return callback(event.Device.ID, event.Device.Name, event.Details, timestamp)
		}
		return fmt.Errorf("no sensor information in door event")
	}
}

// GetWebhookHandler возвращает webhook handler
func (m *TemperatureMonitor) GetWebhookHandler() *WebhookHandler {
	return m.handler
}

// CreateFullTemperatureMonitoringSetup создаёт полную настройку мониторинга температуры
func CreateFullTemperatureMonitoringSetup(client *Client, ctx context.Context, name string, sensorID int64, webhookURL string, tempThresholdCelsius float64) error {
	// 1. Создаём webhook для температурных событий
	webhook, err := client.CreateWebhook(ctx, WebhookDefinition{
		Name:       name + " Temperature Webhook",
		URL:        webhookURL,
		EventTypes: []string{string(EventTypeAlert)},
	})
	if err != nil {
		return fmt.Errorf("failed to create webhook: %w", err)
	}

	// 2. Создаём алерт для превышения температуры
	_, err = client.CreateTemperatureAlert(ctx, name+" High Temp", sensorID, webhook.ID, tempThresholdCelsius, true, 0, false)
	if err != nil {
		return fmt.Errorf("failed to create high temperature alert: %w", err)
	}

	// 3. Создаём алерт для падения температуры
	_, err = client.CreateTemperatureAlert(ctx, name+" Low Temp", sensorID, webhook.ID, tempThresholdCelsius-10, false, 0, false)
	if err != nil {
		return fmt.Errorf("failed to create low temperature alert: %w", err)
	}

	return nil
}
