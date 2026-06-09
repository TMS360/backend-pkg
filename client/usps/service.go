package usps

import (
	"context"
	"fmt"
)

// Service is the high-level USPS verification API, designed to be reused across
// microservices the same way here.Service is.
type Service interface {
	// VerifyUSAddress standardizes and verifies a single US address. It returns
	// Verified=true only on an exact deliverable match (DPVConfirmation == "Y").
	// A non-match is reported as Verified=false with a nil error — USPS is a
	// paper/billing signal and must never surface as a hard failure.
	VerifyUSAddress(ctx context.Context, req AddressRequest) (*AddressResult, error)
}

type service struct {
	client *Client
}

// NewService wraps a low-level Client in the Service interface.
func NewService(client *Client) Service {
	return &service{client: client}
}

func (s *service) VerifyUSAddress(ctx context.Context, req AddressRequest) (*AddressResult, error) {
	if s.client == nil {
		return nil, fmt.Errorf("usps client is not configured")
	}
	if req.StreetAddress == "" {
		return nil, fmt.Errorf("streetAddress cannot be empty")
	}

	result, err := s.client.VerifyAddress(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to verify US address: %w", err)
	}
	return result, nil
}
