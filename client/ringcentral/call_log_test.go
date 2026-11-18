package ringcentral

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testWindow = CallLogQuery{
	From: time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC),
	To:   time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC),
}

// The shape attribution depends on: both extension ids, the direction and
// result verbatim, and a recording pointer when RingCentral produced one.
func TestListCallLog_MapsRecords(t *testing.T) {
	url, seen := numbersServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer tok", r.Header.Get("Authorization"))
		assert.Equal(t, "Detailed", r.URL.Query().Get("view"),
			"the simple view omits the recording pointer and the extension ids")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"records":[
				{"id":"AbC123","telephonySessionId":"s-aaa","direction":"Outbound","result":"Accepted",
				 "type":"Voice","startTime":"2026-08-19T09:15:00.000Z","duration":95,
				 "from":{"phoneNumber":"+15551230000","extensionId":222,"extensionNumber":"101","name":"Alice"},
				 "to":{"phoneNumber":"+15557770000","name":"Driver Dan"},
				 "recording":{"id":9001,"type":"Automatic","contentUri":"https://media.ringcentral.com:443/restapi/v1.0/account/1/recording/9001/content"}}
			],
			"paging":{"page":1,"totalPages":1}
		}`))
	})

	log, err := NewClientWithCredMust(t, url).ListCallLog(context.Background(), testWindow)
	require.NoError(t, err)
	require.Len(t, log.Records, 1)
	assert.False(t, log.Truncated)

	rec := log.Records[0]
	assert.Equal(t, "AbC123", rec.ID)
	assert.Equal(t, "s-aaa", rec.SessionID)
	assert.Equal(t, "Outbound", rec.Direction)
	assert.Equal(t, "Accepted", rec.Result, "RingCentral's own word, passed through")
	assert.Equal(t, 95, rec.DurationSeconds)
	assert.Equal(t, time.Date(2026, 8, 19, 9, 15, 0, 0, time.UTC), rec.StartTime)

	assert.Equal(t, "+15551230000", rec.From.PhoneNumber)
	assert.Equal(t, "222", rec.From.ExtensionID, "numeric ids are normalised to strings")
	assert.Equal(t, "101", rec.From.ExtensionNumber)
	assert.Equal(t, "+15557770000", rec.To.PhoneNumber)

	require.NotNil(t, rec.Recording)
	assert.Equal(t, "9001", rec.Recording.ID)
	assert.Equal(t, "Automatic", rec.Recording.Type)

	// One token exchange, then one page.
	require.Len(t, *seen, 2)
	assert.Contains(t, (*seen)[1], "dateFrom=2026-08-19T00%3A00%3A00Z")
}

// A call under about thirty seconds has no recording object at all. That has to
// survive as a nil pointer, or the caller cannot tell "nothing to play" from
// "we lost the pointer".
func TestListCallLog_NoRecordingIsNilNotEmpty(t *testing.T) {
	url, _ := numbersServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"records":[
				{"id":"short-1","telephonySessionId":"s-bbb","direction":"Inbound","result":"Missed",
				 "startTime":"2026-08-19T09:20:00.000Z","duration":8,
				 "from":{"phoneNumber":"+15557770000"},
				 "to":{"phoneNumber":"+15551230000","extensionId":222}}
			],
			"paging":{"page":1,"totalPages":1}
		}`))
	})

	log, err := NewClientWithCredMust(t, url).ListCallLog(context.Background(), testWindow)
	require.NoError(t, err)
	require.Len(t, log.Records, 1)
	assert.Nil(t, log.Records[0].Recording)
	assert.Equal(t, "Missed", log.Records[0].Result)
}

// telephonySessionId is absent from older published specs. When it is missing we
// fall back to sessionId so a vendor surprise costs grouping, not the poll.
func TestListCallLog_FallsBackToSessionID(t *testing.T) {
	url, _ := numbersServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"records":[{"id":"x","sessionId":"legacy-session","direction":"Inbound",
			            "startTime":"2026-08-19T09:20:00.000Z","from":{},"to":{}}],
			"paging":{"page":1,"totalPages":1}
		}`))
	})

	log, err := NewClientWithCredMust(t, url).ListCallLog(context.Background(), testWindow)
	require.NoError(t, err)
	assert.Equal(t, "legacy-session", log.Records[0].SessionID)
}

// Pagination stops on totalPages, and every page is actually read.
func TestListCallLog_FollowsPaging(t *testing.T) {
	url, seen := numbersServer(t, func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{
			"records":[{"id":"p%s","direction":"Inbound","startTime":"2026-08-19T09:20:00.000Z","from":{},"to":{}}],
			"paging":{"page":%s,"totalPages":3}
		}`, page, page)
	})

	log, err := NewClientWithCredMust(t, url).ListCallLog(context.Background(), testWindow)
	require.NoError(t, err)
	require.Len(t, log.Records, 3)
	assert.Equal(t, []string{"p1", "p2", "p3"}, []string{log.Records[0].ID, log.Records[1].ID, log.Records[2].ID})
	assert.Len(t, *seen, 4, "one token exchange plus three pages")
	assert.False(t, log.Truncated)
}

// A window holding more than the page cap says so, instead of returning a short
// list that reads as a quiet day.
func TestListCallLog_ReportsTruncation(t *testing.T) {
	url, _ := numbersServer(t, func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{
			"records":[{"id":"p%s","direction":"Inbound","startTime":"2026-08-19T09:20:00.000Z","from":{},"to":{}}],
			"paging":{"page":%s,"totalPages":9999}
		}`, page, page)
	})

	log, err := NewClientWithCredMust(t, url).ListCallLog(context.Background(), testWindow)
	require.NoError(t, err)
	assert.True(t, log.Truncated, "the caller must be able to tell it did not see everything")
	assert.Len(t, log.Records, maxCallLogPages)
}

func TestListCallLog_RejectsEmptyWindow(t *testing.T) {
	url, seen := numbersServer(t, func(w http.ResponseWriter, _ *http.Request) {
		t.Error("no request should be made for an invalid window")
		w.WriteHeader(http.StatusOK)
	})

	_, err := NewClientWithCredMust(t, url).ListCallLog(context.Background(), CallLogQuery{})
	require.Error(t, err)
	assert.Empty(t, *seen)

	_, err = NewClientWithCredMust(t, url).ListCallLog(context.Background(),
		CallLogQuery{From: testWindow.To, To: testWindow.From})
	require.Error(t, err)
	assert.Empty(t, *seen)
}

// A revoked credential is reported as such rather than as an empty call log, so
// the office is told to reconnect instead of being shown a quiet day.
func TestListCallLog_RejectedCredential(t *testing.T) {
	url, _ := numbersServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	_, err := NewClientWithCredMust(t, url).ListCallLog(context.Background(), testWindow)
	assert.ErrorIs(t, err, ErrInvalidCredentials)
}

// ─── FetchRecording ───────────────────────────────────────────────────────────

// recordingServer stands in for RingCentral's media host. The client refuses any
// host outside .ringcentral.com, so these tests exercise checkMediaURI
// separately and drive the happy path through the unexported helper's sibling:
// a stub whose URL is rewritten to look like a media host is not possible over
// plain HTTP, so the transport-level assertions live on the failure paths and
// the streaming behaviour is proven with checkMediaURI bypassed by construction.
func recordingServer(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == tokenPath {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
			return
		}
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// The SSRF guard is the whole reason contentUri is not simply dialled: it is
// vendor input that has been through our database before becoming a request.
func TestCheckMediaURI(t *testing.T) {
	valid := []string{
		"https://media.ringcentral.com:443/restapi/v1.0/account/1/recording/2/content",
		"https://media.devtest.ringcentral.com/restapi/v1.0/account/1/recording/2/content",
		"https://ringcentral.com/x",
	}
	for _, u := range valid {
		assert.NoError(t, checkMediaURI(u), u)
	}

	invalid := []string{
		"https://evil.example.com/restapi/v1.0/account/1/recording/2/content",
		"https://ringcentral.com.evil.example.com/x",
		"http://media.ringcentral.com/x",              // downgraded scheme
		"https://127.0.0.1/x",                         // link-local / loopback
		"file:///etc/passwd",                          // not even http
		"https://media.ringcentral.com.attacker.io/x", // suffix look-alike
	}
	for _, u := range invalid {
		assert.Error(t, checkMediaURI(u), u)
	}
}

// A foreign host is refused before any network call is made — not after a
// request that already leaked the bearer token to it.
func TestFetchRecording_ForeignHostMakesNoRequest(t *testing.T) {
	var hits int
	srv := recordingServer(t, func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	})

	body, _, err := NewClientWithCredMust(t, srv.URL).
		FetchRecording(context.Background(), srv.URL+"/recording/1/content")
	require.Error(t, err)
	assert.Nil(t, body)
	assert.Zero(t, hits, "the token must never be sent to an unvetted host")
	assert.NotContains(t, err.Error(), "tok", "no token in the error text")
}

// Streaming, status mapping and header pass-through, driven through the same
// helper the production path uses, with the host check satisfied by pointing at
// a stub that the guard is asked about directly.
func TestFetchRecording_StreamsAndMapsStatuses(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		wantErr error
	}{
		{"revoked credential", http.StatusUnauthorized, ErrInvalidCredentials},
		{"missing app scope", http.StatusForbidden, ErrInsufficientPermissions},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := recordingServer(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
			})
			c := NewClientWithCredMust(t, srv.URL)
			// Exercise the transport half directly: checkMediaURI is covered above
			// and would reject a httptest host by design.
			body, ctype, err := c.fetchRecordingAt(context.Background(), srv.URL+"/rec")
			assert.ErrorIs(t, err, tc.wantErr)
			assert.Nil(t, body)
			assert.Empty(t, ctype)
		})
	}

	t.Run("streams the body", func(t *testing.T) {
		srv := recordingServer(t, func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "Bearer tok", r.Header.Get("Authorization"))
			w.Header().Set("Content-Type", "audio/mpeg")
			_, _ = w.Write([]byte("ID3-audio-bytes"))
		})
		body, ctype, err := NewClientWithCredMust(t, srv.URL).
			fetchRecordingAt(context.Background(), srv.URL+"/rec")
		require.NoError(t, err)
		defer func() { _ = body.Close() }()

		assert.Equal(t, "audio/mpeg", ctype)
		got, err := io.ReadAll(body)
		require.NoError(t, err)
		assert.Equal(t, "ID3-audio-bytes", string(got))
	})
}

// Secrets must not reach an error string on any of the new paths either.
func TestCallLog_ErrorsDoNotLeakSecrets(t *testing.T) {
	url, _ := numbersServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"boom"}`))
	})

	_, err := NewClientWithCredMust(t, url).ListCallLog(context.Background(), testWindow)
	require.Error(t, err)
	for _, secret := range []string{"app-secret", "jwt-credential"} {
		assert.NotContains(t, err.Error(), secret)
	}
	var authErr *AuthError
	assert.False(t, errors.As(err, &authErr), "a 500 says nothing about the credential")
	assert.True(t, strings.Contains(err.Error(), "500"))
}
