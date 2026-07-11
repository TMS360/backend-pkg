package rmsgate

// Контракт гейт-процесса (roadmap №1 RMS): сервис-владелец при старте
// декларирует процесс, его переходы, схему facts и каталог исполнимых
// requiredSteps. RMS использует декларацию для подсказок в билдере и
// advisory-проверок на publish (опечатка процесса/факта/шага видна автору
// правила, а не молчаливым fail-closed DENY в проде).
//
// Регистрация — HTTP и FIRE-AND-FORGET: она НИКАК не влияет на Decide-путь и
// FailMode. RMS лежит/недоступен ⇒ warn-лог и сервис стартует как обычно.
//
// Пример (в main после конфигурации):
//
//	go func() {
//		if err := rmsgate.RegisterContracts(ctx, rmsHTTPURL, registryToken, []rmsgate.ContractDef{{
//			Process: "invoice", Service: "backend-accounting",
//			Transitions: []rmsgate.TransitionDef{{From: "DRAFT", To: "READY"}},
//			Facts:       []rmsgate.FactDef{{Name: "grand_total", Type: "number"}},
//			Steps:       []rmsgate.ContractStepDef{{Kind: rmsgate.StepManualApproval}},
//		}}); err != nil {
//			slog.Warn("rms contract registration", "err", err)
//		}
//	}()

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// FactDef — декларация факта гейта. JSON-теги зеркалят приёмник RMS
// (internal/feature/contracts) — менять синхронно.
type FactDef struct {
	Name        string `json:"name"`
	Type        string `json:"type"` // string|number|bool|list
	Description string `json:"description,omitempty"`
	Example     string `json:"example,omitempty"`
}

// TransitionDef — переход процесса ("NEW"->"DISPATCHED").
type TransitionDef struct {
	From        string `json:"from"`
	To          string `json:"to"`
	Description string `json:"description,omitempty"`
}

// ContractStepDef — шаг requiredSteps, который сервис умеет исполнять.
// Kind — константы словаря steps.go (компилятор ловит опечатки).
type ContractStepDef struct {
	Kind        string `json:"kind"`
	ArgHint     string `json:"argHint,omitempty"`
	Description string `json:"description,omitempty"`
}

// ContractDef — контракт одного процесса.
type ContractDef struct {
	Process     string            `json:"process"`
	Service     string            `json:"service"`
	Description string            `json:"description,omitempty"`
	Transitions []TransitionDef   `json:"transitions,omitempty"`
	Facts       []FactDef         `json:"facts,omitempty"`
	Steps       []ContractStepDef `json:"steps,omitempty"`
}

// registerHTTPTimeout — потолок одной попытки регистрации.
const registerHTTPTimeout = 5 * time.Second

// RegisterContracts отправляет декларации в RMS (POST /contracts/register,
// заголовок X-Registry-Token). Ретраи с бэкоффом внутри; итоговая ошибка —
// для warn-лога вызывающего (не для остановки сервиса).
func RegisterContracts(ctx context.Context, rmsURL, token string, defs []ContractDef) error {
	return registerJSON(ctx, rmsURL+"/contracts/register", token, defs)
}

// registerJSON — общая механика саморегистрации (контракты; capability — в
// пакете rmsreg с той же семантикой).
func registerJSON(ctx context.Context, url, token string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("rmsgate: marshal: %w", err)
	}
	client := &http.Client{Timeout: registerHTTPTimeout}
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
			// 4xx — ретраи бессмысленны (токен/формат), отдаём сразу.
			lastErr = fmt.Errorf("rmsgate: register %s: %s: %s", url, resp.Status, bytes.TrimSpace(raw))
			if resp.StatusCode < 500 {
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
