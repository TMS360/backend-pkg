package rmsgate

import "time"

// FailMode — что делает клиент, когда RMS недоступен/таймаут/сбой транспорта.
// Контракт деградации объявляется ОДИН раз per process и применяется SDK, а не
// размазывается if-ами по вызывающему коду.
type FailMode uint8

const (
	// FailClosed — DENY при сбое. Default для неизвестных процессов: деньги и
	// внешние эффекты безопаснее остановить, чем пропустить без политики.
	FailClosed FailMode = iota
	// FailOpen — ALLOW при сбое (RiskSafe-процессы: глюк политики не должен
	// морозить операции; пример — доступность водителя в backend-load, DEV-972).
	FailOpen
)

func (m FailMode) String() string {
	if m == FailOpen {
		return "fail-open"
	}
	return "fail-closed"
}

// ProcessDef — регистрация процесса на клиентской стороне.
type ProcessDef struct {
	Process string        // напр. "shipment", "invoice", "billing"
	Mode    FailMode      // режим деградации
	Timeout time.Duration // 0 => дефолт клиента
}

// Registry — реестр процессов сервиса. Неизвестный процесс = FailClosed
// (безопасный дефолт) с таймаутом клиента.
type Registry struct {
	defs map[string]ProcessDef
}

func NewRegistry(defs ...ProcessDef) *Registry {
	m := make(map[string]ProcessDef, len(defs))
	for _, d := range defs {
		m[d.Process] = d
	}
	return &Registry{defs: m}
}

// Mode возвращает режим деградации процесса (unknown => FailClosed).
func (r *Registry) Mode(process string) FailMode {
	if r == nil {
		return FailClosed
	}
	if d, ok := r.defs[process]; ok {
		return d.Mode
	}
	return FailClosed
}

// Timeout возвращает per-process дедлайн (0 => дефолт клиента).
func (r *Registry) Timeout(process string) time.Duration {
	if r == nil {
		return 0
	}
	if d, ok := r.defs[process]; ok {
		return d.Timeout
	}
	return 0
}
