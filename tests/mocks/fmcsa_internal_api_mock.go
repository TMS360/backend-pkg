package mocks

import (
	"context"

	"github.com/TMS360/backend-pkg/client/fmcsa"
	"github.com/stretchr/testify/mock"
)

// MockFmcsaAPI is a mock implementation of the FmcsaAPI interface
type MockFmcsaAPI struct {
	mock.Mock
}

func (m *MockFmcsaAPI) CheckCompanyByMC(ctx context.Context, mcNumber, entityType string) (*fmcsa.Result, error) {
	args := m.Called(ctx, mcNumber, entityType)

	var r0 *fmcsa.Result
	if args.Get(0) != nil {
		r0 = args.Get(0).(*fmcsa.Result)
	}

	return r0, args.Error(1)
}

func (m *MockFmcsaAPI) CheckCompanyByDOT(ctx context.Context, dotNumber, entityType string) (*fmcsa.Result, error) {
	args := m.Called(ctx, dotNumber, entityType)

	var r0 *fmcsa.Result
	if args.Get(0) != nil {
		r0 = args.Get(0).(*fmcsa.Result)
	}

	return r0, args.Error(1)
}

func (m *MockFmcsaAPI) GetCompany(ctx context.Context, dotNumber string) (*fmcsa.Result, error) {
	args := m.Called(ctx, dotNumber)

	var r0 *fmcsa.Result
	if args.Get(0) != nil {
		r0 = args.Get(0).(*fmcsa.Result)
	}

	return r0, args.Error(1)
}

func (m *MockFmcsaAPI) VerifyCompany(ctx context.Context, dotNumber, entityType string) (*fmcsa.Result, error) {
	args := m.Called(ctx, dotNumber, entityType)

	var r0 *fmcsa.Result
	if args.Get(0) != nil {
		r0 = args.Get(0).(*fmcsa.Result)
	}

	return r0, args.Error(1)
}

func (m *MockFmcsaAPI) SearchByDOT(ctx context.Context, dot, entityType string) (*fmcsa.Result, error) {
	args := m.Called(ctx, dot, entityType)

	var r0 *fmcsa.Result
	if args.Get(0) != nil {
		r0 = args.Get(0).(*fmcsa.Result)
	}

	return r0, args.Error(1)
}

func (m *MockFmcsaAPI) SearchByMC(ctx context.Context, mc, entityType string) (*fmcsa.Result, error) {
	args := m.Called(ctx, mc, entityType)

	var r0 *fmcsa.Result
	if args.Get(0) != nil {
		r0 = args.Get(0).(*fmcsa.Result)
	}

	return r0, args.Error(1)
}

func (m *MockFmcsaAPI) FetchFMCSAResults(ctx context.Context, query string, entityType string) ([]*fmcsa.Result, error) {
	args := m.Called(ctx, query, entityType)

	var r0 []*fmcsa.Result
	if args.Get(0) != nil {
		r0 = args.Get(0).([]*fmcsa.Result)
	}

	return r0, args.Error(1)
}

func (m *MockFmcsaAPI) SearchBrokers(ctx context.Context, params fmcsa.SearchParams) (*fmcsa.SearchResponse, error) {
	args := m.Called(ctx, params)

	var r0 *fmcsa.SearchResponse
	if args.Get(0) != nil {
		r0 = args.Get(0).(*fmcsa.SearchResponse)
	}

	return r0, args.Error(1)
}

func (m *MockFmcsaAPI) SearchCarriers(ctx context.Context, params fmcsa.SearchParams) (*fmcsa.SearchResponse, error) {
	args := m.Called(ctx, params)

	var r0 *fmcsa.SearchResponse
	if args.Get(0) != nil {
		r0 = args.Get(0).(*fmcsa.SearchResponse)
	}

	return r0, args.Error(1)
}
