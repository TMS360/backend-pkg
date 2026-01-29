package mocks

import (
	"context"

	"github.com/TMS360/backend-pkg/client/fmcsa"
	"github.com/stretchr/testify/mock"
)

type FmcsaAPIMock struct {
	mock.Mock
}

func (m *FmcsaAPIMock) SearchCompaniesByName(ctx context.Context, name string) ([]fmcsa.Carrier, error) {
	args := m.Called(ctx, name)

	// Handle nil return values safely
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).([]fmcsa.Carrier), args.Error(1)
}
