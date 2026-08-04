package events

import "github.com/google/uuid"

// ParticipantRole is the part an entity played in an audited action. It is what
// lets the driver page answer "everything about this driver" across services:
// the driver may be the SUBJECT of a dispatch, the SUBJECT of a pay statement,
// and an AFFECTED party of a reported issue — three services, one person.
type ParticipantRole string

const (
	// ParticipantActor — who performed the action (the caller). Stamped
	// automatically from the request actor; producers need not add it.
	ParticipantActor ParticipantRole = "ACTOR"
	// ParticipantSubject — the primary entity the action is about.
	ParticipantSubject ParticipantRole = "SUBJECT"
	// ParticipantAffected — another entity materially affected by the action.
	ParticipantAffected ParticipantRole = "AFFECTED"
	// ParticipantAssigned — an assignment target, e.g. the crew a trip was
	// dispatched to (its drivers are the SUBJECT/AFFECTED participants).
	ParticipantAssigned ParticipantRole = "ASSIGNED"
)

// ParticipantKind names what an id points at, so an event can carry several
// distinct entities without collapsing them to a single guessed subject — a
// document event may name a driver, a truck AND a load at once.
type ParticipantKind string

const (
	ParticipantKindUser     ParticipantKind = "user"
	ParticipantKindDriver   ParticipantKind = "driver"
	ParticipantKindCrew     ParticipantKind = "driver_crew"
	ParticipantKindTruck    ParticipantKind = "truck"
	ParticipantKindTrailer  ParticipantKind = "trailer"
	ParticipantKindShipment ParticipantKind = "shipment"
	ParticipantKindCompany  ParticipantKind = "company"
)

// Participant records one entity that took part in an audited action and the
// role it played. Stamped by the producing service at emit time — a trip
// assignment, for example, expands the crew to its drivers (SUBJECT) plus the
// crew itself (ASSIGNED), with the caller carried as ACTOR.
type Participant struct {
	ID   uuid.UUID       `json:"id"`
	Kind ParticipantKind `json:"kind"`
	Role ParticipantRole `json:"role"`
}

// Driver is a small constructor for the common case of stamping a driver user
// as a SUBJECT participant.
func Driver(id uuid.UUID, role ParticipantRole) Participant {
	return Participant{ID: id, Kind: ParticipantKindDriver, Role: role}
}

// hasActor reports whether the list already carries an ACTOR participant, so the
// auto-actor injection in writeEvent does not duplicate one a producer set
// explicitly.
func hasActor(ps []Participant) bool {
	for _, p := range ps {
		if p.Role == ParticipantActor {
			return true
		}
	}
	return false
}

// WithActor returns ps with an ACTOR participant for actorID prepended, unless
// one is already present or actorID is nil. Centralised here so every emit path
// stamps the caller the same way.
func WithActor(ps []Participant, actorID *uuid.UUID) []Participant {
	if actorID == nil || *actorID == uuid.Nil || hasActor(ps) {
		return ps
	}
	return append([]Participant{{ID: *actorID, Kind: ParticipantKindUser, Role: ParticipantActor}}, ps...)
}

// ParticipantIDs returns the distinct entity ids in the list, in first-seen
// order. The audit consumer stores these in a queryable column so a page can ask
// for every event a given user took part in.
func ParticipantIDs(ps []Participant) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(ps))
	out := make([]uuid.UUID, 0, len(ps))
	for _, p := range ps {
		if p.ID == uuid.Nil {
			continue
		}
		if _, ok := seen[p.ID]; ok {
			continue
		}
		seen[p.ID] = struct{}{}
		out = append(out, p.ID)
	}
	return out
}
