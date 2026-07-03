// Package rmsgate — канонический SDK доменного сервиса для policy-шлюза RMS.
//
// Паттерн интеграции (шпаргалка, см. также rms/docs/DEMO.md и backend-sandbox):
//
//	guard  → доменная стейт-машина сервиса (CanTransition) — механизм, не политика
//	Decide → rmsgate.Client.Decide(process, transition, tenant, facts) — per-tenant политика
//	enforce→ if !dec.Allow → вернуть dec.Reasons; иначе применить dec.RequiredSteps
//	         ИДЕМПОТЕНТНО в СВОЕЙ транзакции (RMS никогда не пишет)
//	produce→ commit + transactional outbox (событие видно RMS только после коммита)
//
// Гейт вызывается ДО транзакции (pre-TX): синхронный сетевой хоп внутри
// SELECT FOR UPDATE запрещён (лок на сетевом вызове = дедлоки, ср. DEV-703).
//
// # Decision rule «30 секунд»: это в RMS или в Go?
//
// Проверять сверху вниз, первый «да» — ответ:
//
//  1. Шаг пишет в БД, держит лок или обязан быть атомарным с другими записями? → Go
//     (RMS может ПОТРЕБОВАТЬ действие через requiredSteps — исполняет сервис).
//  2. Математика над деньгами / замороженными снапшотами? → Go (RMS гейтит вход, не считает).
//  3. Hot path (>~1000 rps) / security (JWT/RBAC/HMAC) / стриминг-ingest? → Go.
//  4. Число/флаг/порог, разный по тенантам, читается Go-кодом? → Settings-ключ
//     (<companyID>:setting:<key> в Redis, data plane) — не RMS-хоп.
//  5. Да/нет с причинами ПЕРЕД транзакцией? → GATE-флоу (этот пакет).
//  6. Реакция на свершившийся факт (Kafka)? → EVENT-флоу RMS.
//  7. Цепочка ≥2 самостоятельных идемпотентных мутаций без атомарности? → ORCHESTRATION-флоу.
//  8. Ничего не подошло → Go, и это сигнал на ревью.
//
// # FailMode
//
// Контракт деградации объявляется один раз per process (NewRegistry) и
// применяется SDK: RiskSafe-процессы — FailOpen (глюк политики не морозит
// операции), деньги/внешнее/неизвестное — FailClosed. Decide никогда не
// возвращает error: при сбое RMS вердикт = FailMode с reason.
//
// # Подключение
//
// Пока пакет не запушен в origin backend-pkg, локальные модули используют
//
//	require github.com/TMS360/backend-pkg v0.0.0
//	replace github.com/TMS360/backend-pkg => ../backend-pkg
//
// после пуша — обычный go get github.com/TMS360/backend-pkg@<version>.
package rmsgate
