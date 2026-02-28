package mocks

import (
	"context"

	"github.com/TMS360/backend-pkg/client/saferapi"
	"github.com/stretchr/testify/mock"
)

type SaferAPIMock struct {
	mock.Mock
}

func (m *SaferAPIMock) FetchByMCNumber(ctx context.Context, mcNumber string) (*saferapi.SaferCompanyDTO, error) {
	args := m.Called(ctx, mcNumber)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*saferapi.SaferCompanyDTO), args.Error(1)
}

func (m *SaferAPIMock) FetchByDOTNumber(ctx context.Context, usdot string) (*saferapi.SaferCompanyDTO, error) {
	args := m.Called(ctx, usdot)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*saferapi.SaferCompanyDTO), args.Error(1)
}

func (m *SaferAPIMock) IsValid(saferData *saferapi.SaferCompanyDTO) bool {
	args := m.Called(saferData)
	return args.Bool(0)
}
