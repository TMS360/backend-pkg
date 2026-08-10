package tests

import (
	"testing"

	"github.com/TMS360/backend-pkg/eventlog/events"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DEV — the sensitivity contract producers stamp and the audit read path
// enforces. These pin the producer-side classification rules: the class is
// derived from the event's structured shape (entity_type + changed field names),
// never from prose, and "stricter wins" on a multi-field write.

// --- baseline entity classification -----------------------------------------

func TestClassify_OperationalByDefault(t *testing.T) {
	// A dispatch/trip event with no money fields is everyday operations.
	got := events.Classify("tms-loads", "trips", "sent_to_driver", []string{"status", "driver_crew_id"})
	assert.Equal(t, events.SensitivityOperational, got)
}

func TestClassify_FinancialEntitiesAreFinancial(t *testing.T) {
	for _, et := range []string{
		"invoices", "invoice_lines", "pay_batches", "pay_statements",
		"adjustments", "payments", "driver_tariff_assignments",
	} {
		t.Run(et, func(t *testing.T) {
			assert.Equal(t, events.SensitivityFinancial,
				events.Classify("backend-accounting", et, "updated", nil))
		})
	}
}

func TestClassify_FilesAreCompliance(t *testing.T) {
	// tms-files owns CDL/medical/MVR qualification documents.
	assert.Equal(t, events.SensitivityCompliance,
		events.Classify("tms-files", "files", "rejected", nil))
}

func TestClassify_DriverEmploymentIsHR(t *testing.T) {
	assert.Equal(t, events.SensitivityHR,
		events.Classify("tms-teams", "drivers", "terminated", nil))
}

func TestClassify_PermissionChangeIsSecurity(t *testing.T) {
	assert.Equal(t, events.SensitivitySecurity,
		events.Classify("tms-auth", "user_events", "permission_granted", nil))
}

// --- the load-feed regression: money riding on an operational row -----------

// A shipment row is operational until a money field changes on it — then the
// WHOLE event is FINANCIAL, so a dispatcher sees a sealed line instead of the
// pay figure. This is the exact exposure the read-path ticket exists to close.
func TestClassify_MoneyFieldEscalatesOperationalEventToFinancial(t *testing.T) {
	assert.Equal(t, events.SensitivityFinancial,
		events.Classify("tms-loads", "shipments", "updated", []string{"status", "load_pay"}))
	// camelCase field name is caught too.
	assert.Equal(t, events.SensitivityFinancial,
		events.Classify("tms-loads", "shipments", "updated", []string{"loadPay"}))
	assert.Equal(t, events.SensitivityFinancial,
		events.Classify("tms-loads", "trips", "updated", []string{"rate_per_mile"}))
	assert.Equal(t, events.SensitivityFinancial,
		events.Classify("tms-loads", "shipments", "updated", []string{"margin"}))
}

// A pure everyday change stays operational — no false-positive escalation.
func TestClassify_NonMoneyChangeStaysOperational(t *testing.T) {
	assert.Equal(t, events.SensitivityOperational,
		events.Classify("tms-loads", "shipments", "updated", []string{"status", "notes", "pickup_at"}))
}

// --- stricter-wins ordering --------------------------------------------------

func TestStricter_MoneyBeatsEveryday(t *testing.T) {
	assert.Equal(t, events.SensitivityFinancial,
		events.Stricter(events.SensitivityOperational, events.SensitivityFinancial))
	assert.Equal(t, events.SensitivityFinancial,
		events.Stricter(events.SensitivityFinancial, events.SensitivityOperational))
}

func TestStricter_UnknownNeverDominates(t *testing.T) {
	// An empty/unknown class must not win over a real one during producer-side
	// composition; the read side fails closed on empty separately.
	assert.Equal(t, events.SensitivityHR,
		events.Stricter(events.SensitivityHR, events.Sensitivity("")))
}

func TestSensitivity_ValidRejectsEmpty(t *testing.T) {
	assert.False(t, events.Sensitivity("").Valid())
	assert.False(t, events.Sensitivity("bogus").Valid())
	assert.True(t, events.SensitivityFinancial.Valid())
}

// --- participants: actor stamping + id extraction ---------------------------

func TestWithActor_PrependsActorWhenAbsent(t *testing.T) {
	actor := uuid.New()
	driver := uuid.New()
	ps := events.WithActor([]events.Participant{events.Driver(driver, events.ParticipantSubject)}, &actor)

	require.Len(t, ps, 2)
	assert.Equal(t, events.ParticipantActor, ps[0].Role)
	assert.Equal(t, actor, ps[0].ID)
	assert.Equal(t, events.ParticipantSubject, ps[1].Role)
}

func TestWithActor_DoesNotDuplicateExplicitActor(t *testing.T) {
	actor := uuid.New()
	ps := events.WithActor([]events.Participant{
		{ID: actor, Kind: events.ParticipantKindUser, Role: events.ParticipantActor},
	}, &actor)
	assert.Len(t, ps, 1)
}

func TestWithActor_NilActorIsNoop(t *testing.T) {
	driver := uuid.New()
	in := []events.Participant{events.Driver(driver, events.ParticipantSubject)}
	assert.Equal(t, in, events.WithActor(in, nil))
}

// A time-off event for a driver who has since left the company still stamps that
// driver — participants carry a raw id and never depend on current employment.
func TestParticipantIDs_DedupesAndSkipsNil(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	ids := events.ParticipantIDs([]events.Participant{
		{ID: a, Role: events.ParticipantActor},
		{ID: b, Role: events.ParticipantSubject},
		{ID: a, Role: events.ParticipantAffected}, // dup id, different role
		{ID: uuid.Nil, Role: events.ParticipantAssigned},
	})
	assert.Equal(t, []uuid.UUID{a, b}, ids)
}

// DEV-1514 — the participant contract must carry all five roles the entity
// timeline distinguishes: who acted, who it is about, who was assigned, who was
// mentioned, and who was affected next in the chain. These are distinct wire
// values (they land verbatim in the audit row), so a rename or a missing role
// silently drops a class of participation off every timeline.
func TestParticipantRoles_AreTheFiveDistinctContractValues(t *testing.T) {
	roles := map[string]events.ParticipantRole{
		"ACTOR":     events.ParticipantActor,
		"SUBJECT":   events.ParticipantSubject,
		"ASSIGNED":  events.ParticipantAssigned,
		"MENTIONED": events.ParticipantMentioned,
		"AFFECTED":  events.ParticipantAffected,
	}
	seen := make(map[events.ParticipantRole]struct{}, len(roles))
	for wire, role := range roles {
		assert.Equal(t, wire, string(role), "role wire value must be stable")
		seen[role] = struct{}{}
	}
	assert.Len(t, seen, 5, "all five roles must be distinct")
}
