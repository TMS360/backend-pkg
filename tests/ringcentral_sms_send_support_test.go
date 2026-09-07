package tests

// DEV-1897 — the package-level plumbing a driver text ride on: the
// extension-scoped number listing (the only place FeatureSMSSender exists), the
// message-store read that turns "Queued" into the truth, and the flat sms_view /
// sms_send permissions.

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/TMS360/backend-pkg/client/ringcentral"
	"github.com/TMS360/backend-pkg/enums"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListExtensionPhoneNumbers_HitsTheOwningExtensionAndKeepsFeatures(t *testing.T) {
	srv, paths, _ := probeServer(t, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"records":[
			{"id":1,"phoneNumber":"+15551110000","usageType":"DirectNumber","type":"VoiceFax","features":["CallerId","SmsSender","MmsSender"],"extension":{"id":777,"extensionNumber":"101"}}
		],"paging":{"page":1,"totalPages":1}}`))
	})

	client, err := ringcentral.NewClientWithCred(probeCred(srv.URL))
	require.NoError(t, err)

	nums, err := client.ListExtensionPhoneNumbers(context.Background(), "")
	require.NoError(t, err)
	require.Len(t, nums, 1)
	assert.Contains(t, nums[0].Features, ringcentral.FeatureSMSSender,
		"the extension-scoped listing is the ONLY one that carries features — the account listing omits the field on a real account")
	assert.Contains(t, (*paths)[0], "/extension/~/phone-number",
		"an empty id must mean the credential's own extension")
}

func TestSMSMessage_ReadsTheStoredStatusVerbatim(t *testing.T) {
	srv, paths, _ := probeServer(t, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":3690522853053,"messageStatus":"DeliveryFailed",
			"creationTime":"2026-09-05T06:48:26.000Z","lastModifiedTime":"2026-09-05T06:48:28.662Z",
			"to":[{"messageStatus":"DeliveryFailed","errorCode":"SMS-CAR-105"}]}`))
	})

	client, err := ringcentral.NewClientWithCred(probeCred(srv.URL))
	require.NoError(t, err)

	st, err := client.SMSMessage(context.Background(), "", "3690522853053")
	require.NoError(t, err)
	assert.Equal(t, ringcentral.SMSStatusDeliveryFailed, st.MessageStatus)
	assert.Equal(t, "SMS-CAR-105", st.ErrorCode,
		"the carrier's own reason must survive; a bare 'failed' is the unhelpful answer the ticket calls out")
	assert.True(t, strings.Contains((*paths)[0], "/message-store/3690522853053"), "got %s", (*paths)[0])
}

func TestSMSMessage_RequiresAMessageID(t *testing.T) {
	client, err := ringcentral.NewClientWithCred(probeCred("http://127.0.0.1:1"))
	require.NoError(t, err)
	_, err = client.SMSMessage(context.Background(), "", " ")
	require.Error(t, err)
}

func TestSmsPermissions_AreFlatAndDefaultToTheDispatchDesk(t *testing.T) {
	for _, code := range []enums.UserPermissionEnum{enums.PermSmsView, enums.PermSmsSend} {
		assert.NotContains(t, string(code), ".",
			"a dotted code would be satisfied by a module prefix — the calls_view trap")
	}

	defaults := enums.DefaultRolePermissions()
	for _, role := range []enums.UserRoleEnum{enums.UserRoleAdmin, enums.UserRoleManager, enums.UserRoleDispatcher} {
		assert.Contains(t, defaults[role], string(enums.PermSmsView), "%s should view threads by default", role)
		assert.Contains(t, defaults[role], string(enums.PermSmsSend), "%s should text drivers by default", role)
	}
	assert.NotContains(t, defaults[enums.UserRoleDriver], string(enums.PermSmsView),
		"the thread is the office's record of texting drivers, not the drivers'")
}
