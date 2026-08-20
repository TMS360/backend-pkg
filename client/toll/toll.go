// Package toll is the abstraction for pulling toll-road transactions from
// toll aggregators (PrePass today; EZPass, Bestpass and others later).
//
// A carrier bolts a transponder to each truck; the aggregator pays the plazas
// and bills the carrier weekly. For lease/owner-operator drivers those tolls
// are recovered from driver pay, so backend-accounting needs every charge
// attributed to a truck, a trip and a driver.
//
// This package is the mirror image of client/factoring: factoring PUSHES a
// batch to a vendor, toll PULLS a file from one. Two axes vary independently
// and are deliberately kept apart:
//
//   - transport — how the bytes arrive (SFTP today; FTPS/FTP/HTTP API later),
//   - format    — how the bytes are read (PrePass column layout; others later).
//
// They are separate because a human uploading a spreadsheet by hand uses the
// format with no transport at all. Parse is therefore part of Provider and
// callable without ever opening a connection — one parser serves both the
// scheduled folder poll and the manual upload, so the two cannot drift.
//
// Credentials are per-company and live in backend-accounting's own
// `toll_credentials` table — NOT in tms360-backend's `settings` table and NOT
// alongside factoring credentials. Unlike factoring, transport config
// (host/port/directory) IS part of Credential: every carrier gets its own
// folder on the aggregator, so it cannot be a package constant.
//
// Implementations: PrePassSFTP (prepass_sftp). Add more by writing a new file
// and registering it in registry.go.
package toll

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// ProviderType identifies which toll aggregator a credential and an ingested
// row belong to. Stored on the toll_credentials row and as the discriminator
// column on every pooled toll event. Add a new constant whenever a new
// implementation lands; keep AllProviderTypes / IsValid in sync.
type ProviderType string

const (
	// ProviderPrePassSFTP is PrePass delivering a weekly spreadsheet to an
	// SFTP folder. PrePass exposes no API — the file is the whole interface.
	ProviderPrePassSFTP ProviderType = "prepass_sftp"
)

// AllProviderTypes is the canonical list of supported provider types — used by
// credential validation and by gqlgen for the GraphQL enum.
var AllProviderTypes = []ProviderType{
	ProviderPrePassSFTP,
}

// IsValid reports whether p is a known ProviderType. Use it to validate
// incoming input before persisting a credential.
func (p ProviderType) IsValid() bool {
	for _, x := range AllProviderTypes {
		if p == x {
			return true
		}
	}
	return false
}

// String returns the wire form ("prepass_sftp"); satisfies fmt.Stringer.
func (p ProviderType) String() string { return string(p) }

// MarshalGQL renders the enum on the GraphQL wire. gqlgen calls this when
// resolving TollProviderType in the schema.
func (p ProviderType) MarshalGQL(w io.Writer) {
	_, _ = io.WriteString(w, strconv.Quote(string(p)))
}

// UnmarshalGQL parses the enum from the GraphQL wire and validates it against
// AllProviderTypes — rejecting unknown values at the schema boundary instead
// of failing later inside the registry.
func (p *ProviderType) UnmarshalGQL(v any) error {
	s, ok := v.(string)
	if !ok {
		return fmt.Errorf("toll: ProviderType must be a string, got %T", v)
	}
	pt := ProviderType(s)
	if !pt.IsValid() {
		return fmt.Errorf("toll: unknown ProviderType %q", s)
	}
	*p = pt
	return nil
}

// Transport names the wire protocol a provider speaks. Reported by Rules so
// callers can render the right settings form (an API provider has no
// directory; an SFTP one has no base URL) without switching on ProviderType.
type Transport string

const (
	TransportSFTP Transport = "sftp"
	TransportFTPS Transport = "ftps"
	TransportAPI  Transport = "api"
)

// TLSMode selects how an FTP connection is secured. Only meaningful for
// TransportFTPS providers.
type TLSMode string

const (
	// TLSNone is plain FTP with no encryption.
	TLSNone TLSMode = ""
	// TLSExplicit is FTPES: connect in the clear on the normal port, then
	// upgrade with AUTH TLS. This is what most aggregators mean by "FTPS".
	TLSExplicit TLSMode = "explicit"
	// TLSImplicit is FTPS on a dedicated TLS-from-byte-zero port (usually 990).
	TLSImplicit TLSMode = "implicit"
)

// Credential is the per-company connection config for one toll aggregator.
//
// Unlike factoring.Credential, transport config lives here rather than in
// package constants: factoring has one endpoint shared by every carrier, while
// each carrier is issued its own host and folder by the toll aggregator.
//
// The struct deliberately carries the union of every transport's settings
// rather than one shape per transport. Aggregators arrive over SFTP, FTP,
// FTPS and HTTP APIs, and a union keeps the whole thing storable as a single
// opaque blob: adding FTPS or an API provider later needs no database
// migration, no new column and no change to the persistence layer. Each field
// is documented with the transports that read it; everything else is ignored.
//
// Secret is stored plaintext at rest, matching the existing convention for
// vendor credentials in this project. It is masked on read via Redacted() and
// is never logged here.
type Credential struct {
	ProviderType ProviderType `json:"provider_type,omitempty"`

	// AccountName is the carrier's account label at the aggregator. Optional,
	// but when set it is checked against the account printed in the file so a
	// spreadsheet belonging to another carrier is rejected instead of ingested.
	AccountName string `json:"account_name,omitempty"`

	// Username is the principal: an SFTP/FTP login, an API key id, or an OAuth
	// client id. Empty for aggregators that authenticate with a bare key.
	Username string `json:"username,omitempty"`
	// Secret is the one confidential field: a password, an API key, or an
	// OAuth client secret. Whatever the transport calls it, it lives here so
	// that exactly one field needs masking, rotating and keeping out of logs.
	Secret string `json:"secret,omitempty"`

	// Host, Port and Directory are read by the file transports — SFTP, FTP
	// and FTPS. Port 0 means "the transport's default".
	Host      string `json:"host,omitempty"`
	Port      int    `json:"port,omitempty"`
	Directory string `json:"directory,omitempty"`

	// HostKey is read by SFTP only: the expected server key in
	// authorized_keys form. Empty means "accept any key" — see the warning on
	// sftpDialer.HostKey.
	HostKey string `json:"host_key,omitempty"`

	// TLSMode is read by FTPS only.
	TLSMode TLSMode `json:"tls_mode,omitempty"`
	// Passive is read by FTP and FTPS. Nil means the transport's default,
	// which is passive mode — active mode needs an inbound port and almost
	// never survives a container network.
	Passive *bool `json:"passive,omitempty"`

	// BaseURL is read by HTTP API transports. Usually left empty so the
	// provider's own production endpoint is used; set it to point a company at
	// a sandbox.
	BaseURL string `json:"base_url,omitempty"`
}

// Redacted returns a copy safe to log or return over the wire: the secret is
// replaced with a fixed marker rather than a length-preserving mask, so
// nothing about it leaks. Presence is still observable — an empty secret stays
// empty — because "is this configured?" is a legitimate question.
func (c Credential) Redacted() Credential {
	if c.Secret != "" {
		c.Secret = "[REDACTED]"
	}
	return c
}

// Validate checks the credential against its provider's declared rules.
func (c Credential) Validate() error {
	if !c.ProviderType.IsValid() {
		return fmt.Errorf("toll: unknown provider_type %q", c.ProviderType)
	}
	r := RulesFor(c.ProviderType)
	if r.RequiresUsername && strings.TrimSpace(c.Username) == "" {
		return fmt.Errorf("toll: %s credential missing username", c.ProviderType)
	}
	if r.RequiresSecret && c.Secret == "" {
		return fmt.Errorf("toll: %s credential missing secret", c.ProviderType)
	}
	if r.RequiresBaseURL && strings.TrimSpace(c.BaseURL) == "" {
		return fmt.Errorf("toll: %s credential missing base url", c.ProviderType)
	}
	// The host requirement is waived on non-production deployments, where the
	// TEST_* override supplies our catcher folder instead of the real one.
	if r.RequiresHost && strings.TrimSpace(c.Host) == "" && !isNonProdAppEnv() {
		return fmt.Errorf("toll: %s credential missing host", c.ProviderType)
	}
	return nil
}

// Rules declares what a provider needs and what it can do, as data rather than
// as a type switch at every call site — the same trick factoring/rules.go uses.
// Callers ask RulesFor(pt) instead of branching on the concrete type.
//
// Requirements are per-provider rather than global on purpose. factoring's
// registry demands a username AND a password from every provider, which would
// reject an API-key-only aggregator outright; here each type states its own
// needs, so adding one is a data change rather than surgery on shared code.
type Rules struct {
	Transport        Transport
	RequiresUsername bool
	RequiresSecret   bool
	RequiresHost     bool
	RequiresBaseURL  bool
	// FileExtensions the provider is willing to pick up from the folder,
	// lowercase and dot-prefixed. Empty means "take anything", and is also
	// what an API transport leaves it as.
	FileExtensions []string
}

// RulesFor returns the declared rules for pt. The zero value — no
// requirements, no transport — is returned for unknown types; callers should
// have validated with IsValid first.
//
// Adding an aggregator: declare its ProviderType, add a case here describing
// what it needs, write the impl, and register it in registry.go. Nothing else
// in the stack — storage included — has to change.
func RulesFor(pt ProviderType) Rules {
	switch pt {
	case ProviderPrePassSFTP:
		return Rules{
			Transport:        TransportSFTP,
			RequiresUsername: true,
			RequiresSecret:   true,
			RequiresHost:     true,
			FileExtensions:   []string{".xlsx", ".xls", ".csv"},
		}
	default:
		return Rules{}
	}
}

// RemoteFile is one candidate file seen in the provider's folder. Size and
// ModTime come from the remote listing and are advisory — used for logging and
// for picking the newest file, never for deduplication (that is the row hash's
// job, because an aggregator may re-issue a corrected file under a new name).
type RemoteFile struct {
	Name    string
	Size    int64
	ModTime time.Time
}

// ReadType records how the plaza identified the vehicle.
type ReadType string

const (
	// ReadTransponder — the plaza read the transponder. DeviceID is populated.
	ReadTransponder ReadType = "transponder"
	// ReadPlate — the plaza photographed the licence plate; no transponder was
	// read, so DeviceID is empty and Plate carries the identification.
	ReadPlate ReadType = "plate"
	// ReadUnknown — the file used a value we do not recognise. The row is kept
	// (the money is real) but matching must fall back to whatever ids exist.
	ReadUnknown ReadType = ""
)

// Row is one toll transaction, normalised across providers.
//
// TIME ZONES: PostDate, EntryAt and ExitAt are wall-clock timestamps as
// printed in the source file, stamped UTC because the file carries no zone.
// They are NOT instants. Callers must re-interpret them in the company's time
// zone before comparing against trips or bucketing into pay weeks; treating
// them as UTC instants shifts every plaza crossing by the company's offset.
type Row struct {
	// RowNumber is the 1-based row in the source sheet, so a rejected row can
	// be pointed at in the UI.
	RowNumber int

	PostDate    time.Time
	InvoiceDate *time.Time
	EntryAt     *time.Time
	ExitAt      *time.Time

	Source   string // aggregator's upstream network, e.g. "EZPass", "ELITE"
	Agency   string // tolling authority, e.g. "ILTOLL", "FTE", "MdTA"
	ReadType ReadType

	// DeviceID is the aggregator's own transponder id — the primary match key
	// onto a truck. Empty when ReadType is ReadPlate.
	DeviceID string
	// AgencyRef is whatever the file printed in the "device id or plate"
	// column: the tolling authority's own tag number when a transponder was
	// read, or the raw plate string when it was not. Kept verbatim for audit;
	// not a match key today, but other aggregators key off it.
	AgencyRef string
	// Plate is the normalised plate (state prefix and punctuation stripped),
	// set only when ReadType is ReadPlate. Compare against a truck's plate
	// normalised through NormalizePlate.
	Plate string
	// TruckRef is the carrier's own truck number as printed in the file. It is
	// a CHECK, never the match key — aggregators carry stale and foreign
	// numbers here, and the value may have leading zeros.
	TruckRef string

	EntryPlaza string
	ExitPlaza  string
	Class      string // axle class, "Cl" column; absent in some files
	Miles      string // "Mi" column; absent in some files

	// Amount is the toll charged, rounded to cents. Zero is a legitimate value
	// (some agencies post $0.00 reads) and must be stored, not dropped.
	Amount decimal.Decimal

	// Hash is the stable dedup key for this row — see HashRow. The aggregator
	// issues no transaction id of its own, so re-sending the same week must be
	// caught by content.
	Hash string

	// Raw is the whole source row keyed by its normalised column name, kept so
	// the pool can persist it as jsonb and so columns we do not model yet are
	// not lost.
	Raw map[string]string
}

// RowError is a single row that could not be read. Collected rather than
// returned, so one malformed row never costs the whole file: good rows are
// ingested and bad ones are shown to a human.
type RowError struct {
	RowNumber int
	Column    string
	Value     string
	Err       error
}

func (e RowError) Error() string {
	if e.Column == "" {
		return fmt.Sprintf("row %d: %v", e.RowNumber, e.Err)
	}
	return fmt.Sprintf("row %d, column %q (value %q): %v", e.RowNumber, e.Column, e.Value, e.Err)
}

func (e RowError) Unwrap() error { return e.Err }

// ParseResult is the outcome of reading one file.
type ParseResult struct {
	// Account is the carrier account printed in the file's header sheet, when
	// the provider's format carries one. Used to reject another carrier's file.
	Account string
	// Rows are the transactions that parsed cleanly.
	Rows []Row
	// Errors are rows that did not. len(Errors) > 0 is not a failure of the
	// file as a whole.
	Errors []RowError
	// Skipped counts rows ignored as structurally empty (trailing blank lines,
	// spacer rows) — neither ingested nor an error.
	Skipped int
}

// Provider reads toll transactions from one aggregator.
//
// List and Fetch are the transport half and open a connection. Parse is the
// format half and is pure — it must never touch the network, so the manual
// upload path can call it directly on bytes a human supplied.
type Provider interface {
	// List enumerates candidate files in the configured folder, newest first.
	// An empty slice with a nil error means "connected fine, nothing waiting",
	// which is the normal state between weekly drops — not an error.
	List(ctx context.Context) ([]RemoteFile, error)

	// Fetch downloads one file named by List. Implementations MUST NOT delete
	// or move the remote file: the aggregator's folder is the carrier's own
	// record, and re-reading is made harmless by row hashing instead.
	Fetch(ctx context.Context, name string) ([]byte, error)

	// Parse turns file bytes into rows. Pure: no network, no clock, no
	// provider state. name is used only to pick a format when a provider
	// accepts several (e.g. .csv vs .xlsx) and for error messages.
	Parse(name string, content []byte) (ParseResult, error)

	// TestConnection opens the transport and closes it. Nothing is downloaded.
	// Auth failures return *AuthError so callers can tell a bad login from a
	// host that is merely down.
	TestConnection(ctx context.Context) error
}

// AuthError is returned when a provider rejects credentials (SFTP login
// failure, 401/403 on HTTP, etc.). Callers check IsAuthError to distinguish
// "these credentials are wrong" from "the host is unreachable".
//
// Note the asymmetry with factoring: a failed toll pull must NOT deactivate
// the credential the way a failed factoring push does. Nothing was sent, and a
// transient auth blip on a read is not worth silently disabling a company's
// integration. Deactivation, if any, is the caller's explicit decision.
type AuthError struct {
	ProviderType ProviderType
	Cause        error
}

func (e *AuthError) Error() string {
	return fmt.Sprintf("toll: %s authentication failed: %v", e.ProviderType, e.Cause)
}

func (e *AuthError) Unwrap() error { return e.Cause }

// IsAuthError reports whether err (or any error it wraps) is a *AuthError.
func IsAuthError(err error) bool {
	var ae *AuthError
	return errors.As(err, &ae)
}
