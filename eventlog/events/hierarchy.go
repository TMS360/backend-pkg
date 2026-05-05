package events

// RootEntity is the small set of aggregate-root entity types the platform
// exposes to consumers (UI, reporting). Producers may publish far more
// granular EntityType strings (e.g. "trip_events"), but every leaf rolls up
// to exactly one root for the purposes of cross-service aggregate queries.
type RootEntity string

const (
	RootShipment     RootEntity = "shipments"
	RootUser         RootEntity = "users"
	RootCompany      RootEntity = "companies"
	RootBroker       RootEntity = "brokers"
	RootDriverCrew   RootEntity = "driver_crews"
	RootShareLink    RootEntity = "share_links"
	RootNotification RootEntity = "notifications"
)

// LeafToRoot is consulted by the audit consumer when an inbound event lacks
// explicit RootEntityType/RootEntityID. For self-rooted leaves (e.g. a
// "shipments" event already names its shipment via EntityID) this is a
// faithful fallback. For nested leaves (e.g. "trip_events") this map cannot
// know the parent shipment ID — the producer must set the root explicitly via
// tm.Event(...).WithRoot(...).Publish(ctx); otherwise the row will root to
// itself and remain invisible to aggregate-root queries (this is the
// documented "legacy producer" behaviour, not a bug).
var LeafToRoot = map[string]RootEntity{
	"shipments":       RootShipment,
	"shipment_events": RootShipment,
	"trips":           RootShipment,
	"trip_events":     RootShipment,
	"users":           RootUser,
	"user_events":     RootUser,
	"companies":       RootCompany,
	"brokers":         RootBroker,
	"driver_crews":    RootDriverCrew,
	"share_links":     RootShareLink,
	"notifications":   RootNotification,
}
