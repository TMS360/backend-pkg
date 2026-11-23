package rmsgate

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterContractsSendsPayloadAndToken(t *testing.T) {
	var gotToken string
	var gotDefs []ContractDef
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("X-Registry-Token")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotDefs))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	defs := []ContractDef{{
		Process: "invoice", Service: "accounting",
		Transitions: []TransitionDef{{From: "DRAFT", To: "READY"}},
		Facts:       []FactDef{{Name: "grand_total", Type: "number"}},
		Steps:       []ContractStepDef{{Kind: StepManualApproval}},
	}}
	require.NoError(t, RegisterContracts(context.Background(), srv.URL, "tok", defs))
	assert.Equal(t, "tok", gotToken)
	require.Len(t, gotDefs, 1)
	assert.Equal(t, "invoice", gotDefs[0].Process)
	assert.Equal(t, "grand_total", gotDefs[0].Facts[0].Name)
}

func TestRegisterContractsNoRetryOn4xx(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	err := RegisterContracts(context.Background(), srv.URL, "bad", nil)
	require.Error(t, err)
	assert.Equal(t, int32(1), calls.Load(), "4xx не ретраится (токен/формат)")
}

func TestRegisterContractsRetriesOn5xx(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) < 2 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	require.NoError(t, RegisterContracts(context.Background(), srv.URL, "tok", nil))
	assert.Equal(t, int32(2), calls.Load())
}
