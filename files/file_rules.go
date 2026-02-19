package files

import (
	"errors"
	"strings"
)

type FilePurpose string

const (
	PurposeUserAvatar      FilePurpose = "USER_AVATAR"
	PurposeShipmentRateCon FilePurpose = "SHIPMENT_RATE_CON"
	PurposeShipmentPOD     FilePurpose = "SHIPMENT_POD"
	PurposeShipmentBOL     FilePurpose = "SHIPMENT_BOL"
)

type FileRule struct {
	S3Folder         string
	IsPublic         bool
	MaxSizeBytes     int64
	AllowedMimeTypes []string
}

var FileRules = map[FilePurpose]*FileRule{
	PurposeUserAvatar: {
		S3Folder:         "avatars",
		IsPublic:         true,
		MaxSizeBytes:     2 * 1024 * 1024, // 5MB
		AllowedMimeTypes: []string{"image/jpeg", "image/png", "image/webp"},
	},
	PurposeShipmentRateCon: {
		S3Folder:         "rate_cons",
		IsPublic:         false,
		MaxSizeBytes:     10 * 1024 * 1024, // 10MB
		AllowedMimeTypes: []string{"application/pdf"},
	},
	PurposeShipmentPOD: {
		S3Folder:         "pod",
		IsPublic:         false,
		MaxSizeBytes:     10 * 1024 * 1024, // 10MB
		AllowedMimeTypes: []string{"application/pdf", "image/jpeg", "image/png"},
	},
	PurposeShipmentBOL: {
		S3Folder:         "bol",
		IsPublic:         false,
		MaxSizeBytes:     10 * 1024 * 1024, // 10MB
		AllowedMimeTypes: []string{"application/pdf", "image/jpeg", "image/png"},
	},
}

// GetFileRule retrieves the FileRule associated with the given purpose string. Returns an error for invalid purposes.
func GetFileRule(purposeStr string) (*FileRule, error) {
	purpose := FilePurpose(purposeStr)
	rule, exists := FileRules[purpose]
	if !exists {
		return &FileRule{}, errors.New("invalid_upload_purpose")
	}
	return rule, nil
}

func (r FileRule) IsAllowedMime(mime string) bool {
	for _, allowed := range r.AllowedMimeTypes {
		if strings.HasPrefix(mime, allowed) {
			return true
		}
	}
	return false
}
