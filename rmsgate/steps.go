package rmsgate

import "strings"

// Типизированный контракт «решение → исполнение» (intent-write).
//
// RMS — read-only до транзакции и НЕ пересекает write-границу. Когда per-tenant
// политика требует действие (создать в pending, записать override-маркер,
// запостить системное сообщение), gate.decision кладёт его в Decision.RequiredSteps
// как строку вида "kind" или "kind:arg". Вызывающий доменный сервис разбирает шаги
// и применяет их ИДЕМПОТЕНТНО внутри СВОЕЙ транзакции — так решение замыкается на
// исполнение, а транзакционная граница остаётся в Go.
//
// Это канонический словарь шагов платформы (перенесён из
// rms/internal/grpcserver/requiredsteps.go — серверная сторона хранит свой
// экземпляр, взаимного импорта нет; словарь менять синхронно).
const (
	// StepManualApproval — не auto-approve: создать сущность в pending и ждать ручного аппрува.
	StepManualApproval = "manual_approval"
	// StepOverrideMarker — записать override-маркер перехода; Arg = причина.
	StepOverrideMarker = "override"
	// StepPostMessage — запостить системное сообщение; Arg = шаблон/идентификатор.
	StepPostMessage = "post_message"
	// StepNotify — отправить уведомление; Arg = канал/шаблон.
	StepNotify = "notify"
)

// Step — разобранный required-step контракта.
type Step struct {
	Kind string
	Arg  string
}

// FormatStep кодирует шаг в строку контракта: "kind" либо "kind:arg".
func FormatStep(kind, arg string) string {
	if arg == "" {
		return kind
	}
	return kind + ":" + arg
}

// ParseStep разбирает строку контракта в Step. Arg — всё после первого ':'
// (значения могут содержать ':', напр. URL — поэтому делим только по первому).
func ParseStep(s string) Step {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, ':'); i >= 0 {
		return Step{Kind: strings.TrimSpace(s[:i]), Arg: s[i+1:]}
	}
	return Step{Kind: s}
}

// ParseSteps разбирает Decision.RequiredSteps в типизированные шаги (пустые
// отбрасываются). Вызывается доменным сервисом для применения требований.
func ParseSteps(steps []string) []Step {
	out := make([]Step, 0, len(steps))
	for _, s := range steps {
		if step := ParseStep(s); step.Kind != "" {
			out = append(out, step)
		}
	}
	return out
}
