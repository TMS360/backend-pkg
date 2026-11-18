package importeddetail

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/TMS360/backend-pkg/enums"
	"github.com/TMS360/backend-pkg/middleware"
	"github.com/TMS360/backend-pkg/observability"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// NoPermissionMessage is the refusal a reader without private_data_view gets.
const NoPermissionMessage = "You need the 'see private data' permission."

// ErrNotFound means the record has no such detail in the caller's company.
var ErrNotFound = errors.New("importeddetail: no such detail")

// Reveal sources, stored on each private_data_reveals row.
const (
	SourceImportedDetail = "imported_detail"
	SourceRecordField    = "record_field"
)

// View is one detail as a reader sees it.
type View struct {
	Key          string
	Label        string
	Value        string
	Private      bool
	SourceSystem string
	ImportedAt   time.Time
	ReadError    bool
}

// reveal is one row of private_data_reveals: who saw which full value when.
// It never holds the value.
type reveal struct {
	ID         uuid.UUID `gorm:"column:id"`
	CompanyID  string    `gorm:"column:company_id"`
	ActorID    string    `gorm:"column:actor_id"`
	RecordType string    `gorm:"column:record_type"`
	RecordID   string    `gorm:"column:record_id"`
	Key        string    `gorm:"column:key"`
	Source     string    `gorm:"column:source"`
	RevealedAt time.Time `gorm:"column:revealed_at"`
}

func (reveal) TableName() string { return "private_data_reveals" }

// CanSeePrivate reports whether the caller holds private_data_view (platform
// staff always do).
func CanSeePrivate(ctx context.Context) bool {
	if actor, err := middleware.GetActor(ctx); err == nil && actor.IsSuperAdmin() {
		return true
	}
	return middleware.HasPermission(middleware.GetUserPermsFromContext(ctx), string(enums.PermPrivateDataView))
}

// List returns the stored details of one record, in the caller's company only.
func List(ctx context.Context, db *gorm.DB, companyID, recordType, recordID string) ([]Detail, error) {
	var out []Detail
	err := db.WithContext(ctx).
		Where("company_id = ? AND record_type = ? AND record_id = ?", companyID, recordType, recordID).
		Order("label, key").
		Find(&out).Error
	return out, err
}

// Views turns stored details into what the reader may see: plain values as
// they are, private ones opened and masked (Mask with private_data_view,
// MaskNone without). A value that does not open is one degraded row
// (ReadError, empty value, reported without the value); the rest still load.
func Views(ctx context.Context, details []Detail, canSeePrivate bool) []View {
	out := make([]View, 0, len(details))
	for _, d := range details {
		v := View{Key: d.Key, Label: d.Label, Value: d.Value, Private: d.Private, SourceSystem: d.SourceSystem, ImportedAt: d.ImportedAt}
		if d.Private {
			plain, err := Open(d.CompanyID, d.RecordType, d.RecordID, d.Key, d.Value)
			switch {
			case err != nil:
				observability.CaptureWithCtx(ctx, fmt.Errorf("importeddetail: %s %s key %s does not open: %w", d.RecordType, d.RecordID, d.Key, err))
				v.Value, v.ReadError = "", true
			case canSeePrivate:
				v.Value = Mask(d.Key, plain)
			default:
				v.Value = MaskNone(d.Key, plain)
			}
		}
		out = append(out, v)
	}
	return out
}

// Reveal opens one private detail in full for actorID and writes the audit
// row in the same transaction: no audit row, no value. The caller checks
// CanSeePrivate first. A plain detail is returned as is, without an audit row.
func Reveal(ctx context.Context, db *gorm.DB, actorID, companyID, recordType, recordID, key string) (string, error) {
	var d Detail
	err := db.WithContext(ctx).
		Where("company_id = ? AND record_type = ? AND record_id = ? AND key = ?", companyID, recordType, recordID, key).
		Take(&d).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if !d.Private {
		return d.Value, nil
	}
	plain, err := Open(d.CompanyID, d.RecordType, d.RecordID, d.Key, d.Value)
	if err != nil {
		return "", err
	}
	if err := Audit(ctx, db, actorID, companyID, recordType, recordID, key, SourceImportedDetail); err != nil {
		return "", err
	}
	return plain, nil
}

// Audit writes one private_data_reveals row. Services call it directly for a
// private field that lives on the record itself (SourceRecordField).
func Audit(ctx context.Context, db *gorm.DB, actorID, companyID, recordType, recordID, key, source string) error {
	return db.WithContext(ctx).Create(&reveal{
		ID: uuid.New(), CompanyID: companyID, ActorID: actorID,
		RecordType: recordType, RecordID: recordID, Key: key, Source: source,
		RevealedAt: time.Now().UTC(),
	}).Error
}
