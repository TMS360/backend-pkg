package postgresql

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Тестам нужен настоящий PostgreSQL — хазард 0A000 живёт в wire-протоколе pgx,
// на sqlite/моке его не воспроизвести. CI ОБЯЗАН выставлять TEST_DATABASE_URL,
// иначе тесты скипаются и ничего не пиннят.
func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run the postgresql client tests")
	}
	return dsn
}

// runSchemaChangeProbe воспроизводит ровно прод-сценарий:
// SELECT * (план готовится и кэшируется на соединении) → ALTER TABLE (меняет
// рез-тип, инвалидирует план на стороне PG) → тот же SELECT * на ТОМ ЖЕ
// соединении. Под extended-протоколом pgx переиспользует протухший план и PG
// возвращает 0A000; под simple-протоколом кэшировать нечего.
// Возвращает ошибку второго SELECT (nil = нет 0A000).
func runSchemaChangeProbe(t *testing.T, db *gorm.DB) error {
	t.Helper()

	// КРИТИЧНО: зажимаем пул в одно соединение. pgx кэширует statements
	// per-connection, поэтому второй SELECT обязан попасть на тот же backend
	// conn, что держит протухший план. Без этого проба может уйти на свежее
	// соединение, 0A000 не выстрелит, и позитивный тест пройдёт вхолостую.
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB(): %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	table := fmt.Sprintf("plan_probe_%d", time.Now().UnixNano())
	t.Cleanup(func() { db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", table)) })

	if err := db.Exec(fmt.Sprintf("CREATE TABLE %s (id int)", table)).Error; err != nil {
		t.Fatalf("create table: %v", err)
	}
	if err := db.Exec(fmt.Sprintf("INSERT INTO %s (id) VALUES (1)", table)).Error; err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Одинаковая строка SQL оба раза — pgx кэширует план по тексту запроса.
	// Именно SELECT *, не SELECT id: ошибка дословно "must not change result
	// type", её триггерит ADD COLUMN, меняющий рез-тип у *, но не у списка колонок.
	selectSQL := fmt.Sprintf("SELECT * FROM %s", table)
	var rows []map[string]any

	if err := db.Raw(selectSQL).Scan(&rows).Error; err != nil {
		t.Fatalf("first select: %v", err)
	}
	if err := db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN x int NULL", table)).Error; err != nil {
		t.Fatalf("alter table: %v", err)
	}
	return db.Raw(selectSQL).Scan(&rows).Error
}

func isCachedPlanError(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "0A000"
	}
	return false
}

// Регрешн-страж. Идёт через настоящий openGorm — реверт openGorm обратно на
// postgres.Open(dsn) заставит этот тест падать.
func TestOpenGorm_SurvivesSchemaChangeMidConnection(t *testing.T) {
	dsn := testDSN(t)

	db, err := openGorm(dsn)
	if err != nil {
		t.Fatalf("openGorm: %v", err)
	}

	if err := runSchemaChangeProbe(t, db); err != nil {
		t.Fatalf("смена схемы посреди соединения вернула ошибку — "+
			"PreferSimpleProtocol должен быть включён в openGorm "+
			"(фикс 0A000 отрегрессил): %v", err)
	}
}

// Control. Доказывает, что проба реально воспроизводит хазард: под дефолтным
// (extended) протоколом та же последовательность ОБЯЗАНА дать 0A000. Если этот
// тест перестанет падать — проба больше не воспроизводит баг, и страж выше
// бессмыслен.
func TestDefaultProtocol_StillReproducesCachedPlanError(t *testing.T) {
	dsn := testDSN(t)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open (default protocol): %v", err)
	}

	if err := runSchemaChangeProbe(t, db); !isCachedPlanError(err) {
		t.Fatalf("ожидался 0A000 под дефолтным протоколом "+
			"(контроль, что проба воспроизводит хазард); получено: %v", err)
	}
}

// DEV-2586. Postgres text for a UTC timestamptz is "…+00". On a UTC host Go's
// parser labels that time.Local, so reflect.DeepEqual rejects the same clock
// built with time.UTC. registerTimeCodecs is what openGorm installs.
func TestRegisterTimeCodecs_TimestamptzTextIsUTC(t *testing.T) {
	src := []byte("2026-06-02 08:00:00+00")
	want := time.Date(2026, time.June, 2, 8, 0, 0, 0, time.UTC)

	var plain pgtype.Timestamptz
	if err := scanTimestamptzText(pgtype.NewMap(), src, &plain); err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(plain.Time, want) {
		t.Fatal("default codec already returned time.UTC; this test no longer guards the scan")
	}

	m := pgtype.NewMap()
	if err := registerTimeCodecs(m, "UTC"); err != nil {
		t.Fatal(err)
	}
	var got pgtype.Timestamptz
	if err := scanTimestamptzText(m, src, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Time, want) {
		t.Fatalf("got %#v want %#v", got.Time, want)
	}
}

func scanTimestamptzText(m *pgtype.Map, src []byte, dst *pgtype.Timestamptz) error {
	plan := m.PlanScan(pgtype.TimestamptzOID, pgtype.TextFormatCode, dst)
	if plan == nil {
		return errors.New("no timestamptz scan plan")
	}
	return plan.Scan(src, dst)
}
