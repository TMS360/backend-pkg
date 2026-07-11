// Package rmsreg — саморегистрация capability-операций сервиса в RMS.
// Отдельный от rmsgate пакет: у гейта контракт «Decide никогда не возвращает
// error», здесь — обычный best-effort HTTP. Регистрация FIRE-AND-FORGET:
// ошибка ⇒ warn-лог вызывающего, сервис стартует как обычно (RMS дорисует
// операции при следующем старте или из курируемого файла).
//
// Пример (в main):
//
//	go func() {
//		if err := rmsreg.RegisterCapabilities(ctx, rmsURL, token, []rmsreg.CapabilityDef{{
//			OpID: "accounting.submitBatchToFactoring", Service: "backend-accounting",
//			Title: "Отправить батч в факторинг", Transport: "http",
//			HTTPMethod: "POST", Endpoint: selfURL + "/ops/factoring/submit/{{ args.batchId }}",
//			Inputs: []rmsreg.FieldDef{{Name: "batchId", Type: "string", Required: true}},
//			Risk:   "money",
//		}}); err != nil {
//			slog.Warn("rms capability registration", "err", err)
//		}
//	}()
package rmsreg

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// FieldDef — плоское описание поля входа/выхода операции (advisory-схема).
// JSON-теги зеркалят приёмник RMS (internal/feature/capability) — менять синхронно.
type FieldDef struct {
	Name        string `json:"name"`
	Type        string `json:"type"` // string|number|bool|object|list
	Required    bool   `json:"required,omitempty"`
	Description string `json:"description,omitempty"`
	Example     string `json:"example,omitempty"`
}

// CapabilityDef — декларация операции («кнопки») сервиса.
type CapabilityDef struct {
	OpID        string `json:"opId"` // "<service>.<operation>"
	Service     string `json:"service"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`

	Transport  string `json:"transport"` // http|intent|grpc
	HTTPMethod string `json:"httpMethod,omitempty"`
	Endpoint   string `json:"endpoint,omitempty"` // http: URL-шаблон, {{ args.* }}
	Topic      string `json:"topic,omitempty"`    // intent
	Action     string `json:"action,omitempty"`   // intent

	Inputs  []FieldDef `json:"inputs,omitempty"`
	Outputs []FieldDef `json:"outputs,omitempty"`

	TimeoutMs  int    `json:"timeoutMs,omitempty"`
	Idempotent bool   `json:"idempotent,omitempty"`
	Risk       string `json:"risk,omitempty"` // safe|money|external
	// Enabled: nil = true (операция рабочая); явный false — задекларирована,
	// но выключена (видна на карте системы с пометкой).
	Enabled *bool `json:"enabled,omitempty"`
}

const registerHTTPTimeout = 5 * time.Second

// RegisterCapabilities отправляет декларации в RMS
// (POST /capabilities/register, заголовок X-Registry-Token). Ретраи внутри.
func RegisterCapabilities(ctx context.Context, rmsURL, token string, defs []CapabilityDef) error {
	body, err := json.Marshal(defs)
	if err != nil {
		return fmt.Errorf("rmsreg: marshal: %w", err)
	}
	client := &http.Client{Timeout: registerHTTPTimeout}
	url := rmsURL + "/capabilities/register"
	const attempts = 3
	var lastErr error
	for i := 1; i <= attempts; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Registry-Token", token)
		resp, err := client.Do(req)
		if err == nil {
			raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
			resp.Body.Close()
			if resp.StatusCode < 300 {
				return nil
			}
			lastErr = fmt.Errorf("rmsreg: register: %s: %s", resp.Status, bytes.TrimSpace(raw))
			if resp.StatusCode < 500 { // токен/формат — ретраи бессмысленны
				return lastErr
			}
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(i) * 500 * time.Millisecond):
		}
	}
	return lastErr
}
