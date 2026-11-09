package ringcentral

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The browser gets RingCentral's own shape, trimmed to the first sipInfo, the
// flags and the device id — nothing else from the answer.
func TestProvisionSIP_ReturnsTheRegistrationUnderTheGivenToken(t *testing.T) {
	url, seen := numbersServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, sipProvisionPath, r.URL.Path)
		assert.Equal(t, "Bearer handed-out", r.Header.Get("Authorization"))
		raw, _ := io.ReadAll(r.Body)
		assert.JSONEq(t, `{"sipInfo":[{"transport":"WSS"}]}`, string(raw))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"sipInfo":[
				{"username":"1555","password":"p","authorizationId":"42","domain":"sip.ringcentral.com","outboundProxy":"sip10.ringcentral.com:8083","transport":"WSS"},
				{"username":"second","transport":"WSS"}
			],
			"sipFlags":{"voipFeatureEnabled":true,"outboundCallsEnabled":true},
			"device":{"id":805286004,"type":"WebPhone"},
			"pollingInterval":7200
		}`))
	})

	reg, err := NewClientWithCredMust(t, url).ProvisionSIP(context.Background(), "handed-out")
	require.NoError(t, err)

	var got map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(reg, &got))
	assert.ElementsMatch(t, []string{"sipInfo", "sipFlags", "device"}, keys(got))

	var info []map[string]any
	require.NoError(t, json.Unmarshal(got["sipInfo"], &info))
	require.Len(t, info, 1)
	assert.Equal(t, "1555", info[0]["username"])
	assert.JSONEq(t, `{"voipFeatureEnabled":true,"outboundCallsEnabled":true}`, string(got["sipFlags"]))
	assert.JSONEq(t, `{"id":"805286004"}`, string(got["device"]))

	assert.Len(t, *seen, 1, "a given token means no token exchange")
}

func TestProvisionSIP_MintsATokenWhenNoneIsGiven(t *testing.T) {
	url, seen := numbersServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer tok", r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"sipInfo":[{"username":"1555"}]}`))
	})

	_, err := NewClientWithCredMust(t, url).ProvisionSIP(context.Background(), "")
	require.NoError(t, err)
	require.Len(t, *seen, 2)
	assert.Equal(t, tokenPath, (*seen)[0])
}

func TestProvisionSIP_Refusals(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"no VoIP permission", http.StatusForbidden,
			`{"errorCode":"CMN-408","message":"In order to call this API endpoint, user needs to have [VoipCalling] permission for requested resource."}`,
			ErrSIPCallingNotAllowed},
		{"phone limit", http.StatusBadRequest,
			`{"errorCode":"SIP-603","message":"603 Too Many Contacts"}`,
			ErrSIPPhoneLimit},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			url, _ := numbersServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})

			_, err := NewClientWithCredMust(t, url).ProvisionSIP(context.Background(), "t")
			assert.ErrorIs(t, err, tc.want)
			api, ok := AsAPIError(err)
			require.True(t, ok, "RingCentral's own answer stays reachable for logs")
			assert.Equal(t, tc.status, api.StatusCode)
		})
	}
}

func TestProvisionSIP_OtherFailuresPassThrough(t *testing.T) {
	url, _ := numbersServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	_, err := NewClientWithCredMust(t, url).ProvisionSIP(context.Background(), "t")
	assert.ErrorIs(t, err, ErrInvalidCredentials)

	url, _ = numbersServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	_, err = NewClientWithCredMust(t, url).ProvisionSIP(context.Background(), "t")
	assert.NotErrorIs(t, err, ErrSIPPhoneLimit)
	assert.NotErrorIs(t, err, ErrSIPCallingNotAllowed)
}

func keys(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
