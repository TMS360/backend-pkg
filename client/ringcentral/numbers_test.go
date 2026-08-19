package ringcentral

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// numbersServer mounts the token endpoint plus the phone-number listing, so a
// test drives the same two-step the real client performs.
func numbersServer(t *testing.T, handler http.HandlerFunc) (string, *[]string) {
	t.Helper()
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.RequestURI())
		if r.URL.Path == tokenPath {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
			return
		}
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv.URL, &seen
}

// The listing an admin screen needs: number, usage type and the extension it
// rings, with RingCentral's numeric ids normalised to strings.
func TestListPhoneNumbers_MapsRecords(t *testing.T) {
	url, _ := numbersServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer tok", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"records":[
				{"id":111,"phoneNumber":"+15551230000","usageType":"DirectNumber","type":"VoiceFax","status":"Normal","label":"Alice line",
				 "extension":{"id":222,"extensionNumber":"101","name":"Alice"}},
				{"id":333,"phoneNumber":"+15559990000","usageType":"MainCompanyNumber","type":"VoiceFax","status":"Normal"}
			],
			"paging":{"page":1,"totalPages":1}
		}`))
	})

	nums, err := NewClientWithCredMust(t, url).ListPhoneNumbers(context.Background())
	require.NoError(t, err)
	require.Len(t, nums, 2)

	assert.Equal(t, "111", nums[0].ID)
	assert.Equal(t, "+15551230000", nums[0].PhoneNumber)
	assert.Equal(t, "DirectNumber", nums[0].UsageType)
	assert.Equal(t, "Alice line", nums[0].Label)
	assert.Equal(t, "222", nums[0].ExtensionID, "numeric ids are normalised to strings")
	assert.Equal(t, "101", nums[0].ExtensionNumber)
	assert.Equal(t, "Alice", nums[0].ExtensionName)

	// A number with no extension (main company number behind an IVR) is still
	// listed, just without extension fields.
	assert.Equal(t, "+15559990000", nums[1].PhoneNumber)
	assert.Empty(t, nums[1].ExtensionID)
}

// A big account spans several pages; every page must be followed.
func TestListPhoneNumbers_FollowsPaging(t *testing.T) {
	url, seen := numbersServer(t, func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fmt.Sprintf(
			`{"records":[{"id":%s,"phoneNumber":"+1555000000%s"}],"paging":{"page":%s,"totalPages":3}}`,
			page, page, page)))
	})

	nums, err := NewClientWithCredMust(t, url).ListPhoneNumbers(context.Background())
	require.NoError(t, err)
	require.Len(t, nums, 3, "all three pages must be collected")
	assert.Equal(t, "+15550000001", nums[0].PhoneNumber)
	assert.Equal(t, "+15550000003", nums[2].PhoneNumber)

	// One token call plus one call per page.
	assert.Len(t, *seen, 4)
}

// A revoked credential must surface as ErrInvalidCredentials so the caller can
// say "reconnect RingCentral" rather than "this company has no numbers".
func TestListPhoneNumbers_RejectedCredential(t *testing.T) {
	t.Run("token rejected", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"Token is expired"}`))
		}))
		t.Cleanup(srv.Close)

		_, err := NewClientWithCredMust(t, srv.URL).ListPhoneNumbers(context.Background())
		assert.ErrorIs(t, err, ErrInvalidCredentials)
	})

	t.Run("listing rejected", func(t *testing.T) {
		url, _ := numbersServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"Unauthorized"}`))
		})

		_, err := NewClientWithCredMust(t, url).ListPhoneNumbers(context.Background())
		assert.ErrorIs(t, err, ErrInvalidCredentials)
	})
}

// A platform failure is not a credential failure — the tenant must not be told
// to reconnect because RingCentral had a bad minute.
func TestListPhoneNumbers_PlatformFailure(t *testing.T) {
	url, _ := numbersServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"Internal error"}`))
	})

	_, err := NewClientWithCredMust(t, url).ListPhoneNumbers(context.Background())
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrInvalidCredentials)
	assert.Contains(t, err.Error(), "500")
}

// An account with no numbers is an empty list, not an error.
func TestListPhoneNumbers_Empty(t *testing.T) {
	url, _ := numbersServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"records":[],"paging":{"page":1,"totalPages":1}}`))
	})

	nums, err := NewClientWithCredMust(t, url).ListPhoneNumbers(context.Background())
	require.NoError(t, err)
	assert.Empty(t, nums)
}

// NewClientWithCredMust builds a client pointed at the stub platform.
func NewClientWithCredMust(t *testing.T, serverURL string) *Client {
	t.Helper()
	c, err := NewClientWithCred(Cred{
		ClientID:     "app-id",
		ClientSecret: "app-secret",
		JWT:          "jwt-credential",
		ServerURL:    serverURL,
	})
	require.NoError(t, err)
	return c
}
