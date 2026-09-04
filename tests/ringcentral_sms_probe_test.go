package tests

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TMS360/backend-pkg/client/ringcentral"
	"github.com/TMS360/backend-pkg/client/ringcentral/smsprobe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DEV-1895 — can one company-level RingCentral login text FROM a number that
// belongs to a different extension?
//
// The live answer needs a RingCentral sandbox credential, so what is tested
// here is everything that turns a live run into the ticket's answer: the wire
// shape of the send, the verbatim capture of RingCentral's refusal, the
// classification into the ticket's three answers, and the report the comment is
// made of (AC1) including the high-volume cost block when the answer is "no"
// (AC2).

const tokenPath = "/restapi/oauth/token"

// probeServer mounts the token endpoint plus a handler for everything else, and
// records the paths and bodies the client actually sent.
func probeServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request, body []byte)) (*httptest.Server, *[]string, *[]string) {
	t.Helper()
	var paths, bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == tokenPath {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600,"owner_id":"777"}`))
			return
		}
		body, _ := io.ReadAll(r.Body)
		paths = append(paths, r.URL.Path)
		bodies = append(bodies, string(body))
		handler(w, r, body)
	}))
	t.Cleanup(srv.Close)
	return srv, &paths, &bodies
}

func probeCred(server string) ringcentral.Cred {
	return ringcentral.Cred{ClientID: "id", ClientSecret: "secret", JWT: "jwt", ServerURL: server}
}

func TestSendSMS_PostsToTheExtensionScopedPath(t *testing.T) {
	srv, paths, bodies := probeServer(t, func(w http.ResponseWriter, _ *http.Request, _ []byte) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1234,"direction":"Outbound","messageStatus":"Queued","from":{"phoneNumber":"+15551110000"},"to":[{"phoneNumber":"+15552220000"}]}`))
	})

	client, err := ringcentral.NewClientWithCred(probeCred(srv.URL))
	require.NoError(t, err)

	res, err := client.SendSMS(context.Background(), ringcentral.SMSRequest{
		From: "+15551110000", To: []string{"+15552220000"}, Text: "hi",
	})
	require.NoError(t, err)
	assert.Equal(t, "1234", res.ID)
	assert.Equal(t, "Queued", res.MessageStatus)

	// An empty ExtensionID must mean "the extension the credential belongs to".
	require.Len(t, *paths, 1)
	assert.Equal(t, "/restapi/v1.0/account/~/extension/~/sms", (*paths)[0])

	var sent map[string]any
	require.NoError(t, json.Unmarshal([]byte((*bodies)[0]), &sent))
	assert.Equal(t, map[string]any{"phoneNumber": "+15551110000"}, sent["from"])
	assert.Equal(t, "hi", sent["text"])
}

func TestSendSMS_AddressesAnotherExtensionWhenAsked(t *testing.T) {
	srv, paths, _ := probeServer(t, func(w http.ResponseWriter, _ *http.Request, _ []byte) {
		_, _ = w.Write([]byte(`{"id":1,"messageStatus":"Queued"}`))
	})
	client, err := ringcentral.NewClientWithCred(probeCred(srv.URL))
	require.NoError(t, err)

	_, err = client.SendSMS(context.Background(), ringcentral.SMSRequest{
		ExtensionID: "42", From: "+15551110000", To: []string{"+15552220000"}, Text: "hi",
	})
	require.NoError(t, err)
	assert.Equal(t, "/restapi/v1.0/account/~/extension/42/sms", (*paths)[0])
}

func TestSendSMS_KeepsRingCentralsRefusalVerbatim(t *testing.T) {
	const body = `{"errorCode":"InvalidParameter","message":"Parameter [from] value is invalid","errors":[{"errorCode":"CMN-101","message":"Phone number doesn't belong to extension","parameterName":"from"}]}`
	srv, _, _ := probeServer(t, func(w http.ResponseWriter, _ *http.Request, _ []byte) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(body))
	})
	client, err := ringcentral.NewClientWithCred(probeCred(srv.URL))
	require.NoError(t, err)

	_, err = client.SendSMS(context.Background(), ringcentral.SMSRequest{
		From: "+15559990000", To: []string{"+15552220000"}, Text: "hi",
	})
	require.Error(t, err)

	apiErr, ok := ringcentral.AsAPIError(err)
	require.True(t, ok, "a platform refusal must survive as *APIError, not as a flattened string")
	assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	assert.Equal(t, "InvalidParameter", apiErr.Code)
	assert.Equal(t, []string{"CMN-101"}, apiErr.SubCodes())
	assert.Equal(t, body, apiErr.Body, "the raw body is the evidence the ticket asks for")
}

func TestSendSMS_RevokedCredentialIsNotAPlatformRefusal(t *testing.T) {
	srv, _, _ := probeServer(t, func(w http.ResponseWriter, _ *http.Request, _ []byte) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	client, err := ringcentral.NewClientWithCred(probeCred(srv.URL))
	require.NoError(t, err)

	_, err = client.SendSMS(context.Background(), ringcentral.SMSRequest{
		From: "+1", To: []string{"+2"}, Text: "hi",
	})
	assert.ErrorIs(t, err, ringcentral.ErrInvalidCredentials)
}

func TestTokenOwnerExtensionID(t *testing.T) {
	srv, _, _ := probeServer(t, func(http.ResponseWriter, *http.Request, []byte) {})
	client, err := ringcentral.NewClientWithCred(probeCred(srv.URL))
	require.NoError(t, err)

	owner, err := client.TokenOwnerExtensionID(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "777", owner, "owner_id is the only thing that says which extension our credential is")
}

// --- number selection ------------------------------------------------------

func TestSelectNumbers_PrefersSMSCapableAndNeedsASecondExtension(t *testing.T) {
	numbers := []ringcentral.PhoneNumber{
		{PhoneNumber: "+1000", UsageType: "MainCompanyNumber"}, // no extension: skipped
		{PhoneNumber: "+1001", ExtensionID: "777", Features: []string{"CallerId"}},
		{PhoneNumber: "+1002", ExtensionID: "777", Features: []string{"SmsSender", "CallerId"}},
		{PhoneNumber: "+1003", ExtensionID: "888", Features: []string{"SmsSender"}},
	}
	pick, err := smsprobe.SelectNumbers(numbers, "777")
	require.NoError(t, err)
	assert.Equal(t, "+1002", pick.Own.PhoneNumber)
	assert.Equal(t, "+1003", pick.Shared.PhoneNumber)
	assert.Equal(t, "888", pick.Shared.ExtensionID)
}

func TestSelectNumbers_RefusesWhenNothingCanBeShared(t *testing.T) {
	_, err := smsprobe.SelectNumbers([]ringcentral.PhoneNumber{
		{PhoneNumber: "+1002", ExtensionID: "777", Features: []string{"SmsSender"}},
	}, "777")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "second extension")
}

// --- classification: the ticket's three answers ----------------------------

func okAttempt(label string) smsprobe.Attempt {
	return smsprobe.Attempt{Label: label, Sent: true, MessageID: "9", MessageStatus: "Queued"}
}

func TestClassify_SharedNumberAccepted(t *testing.T) {
	v := smsprobe.Classify(okAttempt("control"), []smsprobe.Attempt{okAttempt("shared")})
	assert.Equal(t, smsprobe.AnswerSharedAllowed, v.Answer)
	assert.Contains(t, v.Reason, "accepted")
}

func TestClassify_OwnNumberOnly(t *testing.T) {
	shared := []smsprobe.Attempt{{
		Label:      "shared through our extension",
		StatusCode: http.StatusBadRequest,
		ErrorCode:  "InvalidParameter",
		SubCodes:   []string{"CMN-101"},
		Message:    "Parameter [from] value is invalid",
		Raw:        `{"errorCode":"InvalidParameter"}`,
	}}
	v := smsprobe.Classify(okAttempt("control"), shared)
	assert.Equal(t, smsprobe.AnswerOwnNumberOnly, v.Answer)
}

func TestClassify_PermissionRefusalIsNotAnAnswerAboutSharing(t *testing.T) {
	shared := []smsprobe.Attempt{{
		Label:      "shared through its own extension",
		StatusCode: http.StatusForbidden,
		ErrorCode:  "CMN-408",
		Message:    "In order to call this API endpoint, user needs to have [SMS] permission",
	}}
	v := smsprobe.Classify(okAttempt("control"), shared)
	assert.Equal(t, smsprobe.AnswerInconclusive, v.Answer,
		"a plain user is stopped at the door whatever number it asks for — that says nothing about who owns the sender")
	assert.Contains(t, v.Reason, "super admin", "the reason must name the fix, not just the failure")
}

func TestClassify_BareForbiddenIsStillAnOwnershipWall(t *testing.T) {
	shared := []smsprobe.Attempt{{
		Label:      "shared through its own extension",
		StatusCode: http.StatusForbidden,
		ErrorCode:  "CMN-102",
		Message:    "Resource for parameter [extensionId] is not found",
	}}
	v := smsprobe.Classify(okAttempt("control"), shared)
	assert.Equal(t, smsprobe.AnswerOwnNumberOnly, v.Answer,
		"a 403 that does not blame our permissions is the same wall from the other side")
}

func TestClassify_OwnershipPlusPermissionIsOnlyHalfAnAnswer(t *testing.T) {
	shared := []smsprobe.Attempt{
		{
			Label:      "shared through our extension",
			StatusCode: http.StatusBadRequest,
			ErrorCode:  "InvalidParameter",
			SubCodes:   []string{"CMN-101"},
			Message:    "Parameter [from] value is invalid",
		},
		{
			Label:      "shared through its own extension",
			StatusCode: http.StatusForbidden,
			ErrorCode:  "CMN-408",
			Message:    "user needs to have [SMS] permission",
		},
	}
	v := smsprobe.Classify(okAttempt("control"), shared)
	assert.Equal(t, smsprobe.AnswerInconclusive, v.Answer,
		"the admin-acting leg never ran, so OWN_NUMBER_ONLY would overstate what was proved")
	assert.Contains(t, v.Reason, "never actually tested")
}

func TestClassify_PlannedButUnsentAttemptsProveNothing(t *testing.T) {
	own := smsprobe.Attempt{Label: "control", From: "+15551110000", To: "+15552220000"}
	shared := []smsprobe.Attempt{{Label: "shared", From: "+15559990000", To: "+15552220000"}}

	v := smsprobe.Classify(own, shared)
	assert.Equal(t, smsprobe.AnswerInconclusive, v.Answer)
	assert.Contains(t, v.Reason, "not performed")
}

func TestClassify_HighVolumeProductWins(t *testing.T) {
	shared := []smsprobe.Attempt{{
		Label:      "shared through our extension",
		StatusCode: http.StatusBadRequest,
		ErrorCode:  "MSG-320",
		Message:    "Use the High Volume SMS (A2P) API for this sender",
		Raw:        `{"errorCode":"MSG-320"}`,
	}}
	v := smsprobe.Classify(okAttempt("control"), shared)
	assert.Equal(t, smsprobe.AnswerHighVolumeOnly, v.Answer)
}

func TestClassify_ControlFailureProvesNothing(t *testing.T) {
	own := smsprobe.Attempt{
		Label:      "control",
		StatusCode: http.StatusBadRequest,
		ErrorCode:  "MSG-242",
		Message:    "The requested feature is not available",
	}
	shared := []smsprobe.Attempt{{Label: "shared", StatusCode: 400, ErrorCode: "CMN-101", Message: "Parameter [from] value is invalid"}}

	v := smsprobe.Classify(own, shared)
	assert.Equal(t, smsprobe.AnswerInconclusive, v.Answer,
		"a control send that fails makes the shared attempts meaningless — this is the wrong-for-two-weeks trap")
	assert.Contains(t, v.Reason, "not enabled")
}

func TestClassify_UnknownRefusalStaysInconclusive(t *testing.T) {
	shared := []smsprobe.Attempt{{
		Label:      "shared",
		StatusCode: http.StatusInternalServerError,
		ErrorCode:  "CMN-999",
		Message:    "Something nobody has seen",
	}}
	v := smsprobe.Classify(okAttempt("control"), shared)
	assert.Equal(t, smsprobe.AnswerInconclusive, v.Answer)
	assert.Contains(t, v.Reason, "not classified")
}

func TestClassify_TransportFailureIsNotAnAnswer(t *testing.T) {
	v := smsprobe.Classify(smsprobe.Attempt{Label: "control", Transport: "dial tcp: timeout"}, nil)
	assert.Equal(t, smsprobe.AnswerInconclusive, v.Answer)
}

// --- the report the ticket comment is made of ------------------------------

func refusedReport() smsprobe.Report {
	return smsprobe.Report{
		ServerURL:        ringcentral.SandboxServerURL,
		OwnerExtensionID: "777",
		Numbers: []ringcentral.PhoneNumber{
			{PhoneNumber: "+15551110000", UsageType: "DirectNumber", Type: "VoiceFax",
				Features: []string{"SmsSender"}, ExtensionID: "777", ExtensionNumber: "101", ExtensionName: "Alice"},
		},
		Own: okAttempt("control — our own number, our own extension"),
		Shared: []smsprobe.Attempt{{
			Label:       "shared number, sent through OUR extension (the question)",
			ExtensionID: "~",
			From:        "+15559990000",
			To:          "+15552220000",
			Text:        "probe",
			StatusCode:  http.StatusBadRequest,
			ErrorCode:   "InvalidParameter",
			SubCodes:    []string{"CMN-101"},
			Message:     "Parameter [from] value is invalid",
			Raw:         `{"errorCode":"InvalidParameter","errors":[{"errorCode":"CMN-101"}]}`,
		}},
	}
}

func TestRender_CarriesVerdictRequestAndVerbatimResponse(t *testing.T) {
	r := refusedReport()
	r.Verdict = smsprobe.Classify(r.Own, r.Shared)

	out := smsprobe.Render(r)

	assert.Contains(t, out, "## Answer: OWN_NUMBER_ONLY")
	assert.Contains(t, out, "POST /restapi/v1.0/account/~/extension/~/sms", "the request that was sent")
	assert.Contains(t, out, `"phoneNumber":"+15559990000"`, "the sender that was attempted")
	assert.Contains(t, out, r.Shared[0].Raw, "the answer that came back, verbatim")
	assert.Contains(t, out, "CMN-101")
	assert.Contains(t, out, "SmsSender", "the account's numbers and what each may do")
}

func TestRender_QuotesTheHighVolumeCostWhenTheAnswerIsNo(t *testing.T) {
	r := refusedReport()
	r.Verdict = smsprobe.Classify(r.Own, r.Shared)

	out := smsprobe.Render(r)

	require.Contains(t, out, "High Volume SMS", "AC2: when sharing is refused, the comment must price the fallback")
	assert.Contains(t, out, "per SMS", "per-message price")
	assert.Contains(t, out, "Per month", "recurring cost")
	assert.Contains(t, out, "a2p-sms/messages", "the other product's endpoint, so nobody confuses the two")
}

func TestRender_OmitsTheCostBlockWhenSharingWorks(t *testing.T) {
	r := refusedReport()
	r.Shared = []smsprobe.Attempt{okAttempt("shared number, sent through OUR extension (the question)")}
	r.Verdict = smsprobe.Classify(r.Own, r.Shared)

	out := smsprobe.Render(r)

	assert.Equal(t, smsprobe.AnswerSharedAllowed, r.Verdict.Answer)
	assert.NotContains(t, out, "High Volume SMS")
}

func TestRun_RefusesWithoutARecipient(t *testing.T) {
	_, err := smsprobe.Run(context.Background(), smsprobe.Config{Cred: probeCred("http://127.0.0.1:1")})
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "recipient"), "got %q", err.Error())
}

func TestRun_DryRunReadsTheAccountAndSendsNothing(t *testing.T) {
	srv, paths, _ := probeServer(t, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		if strings.HasSuffix(r.URL.Path, "/sms") {
			t.Fatalf("dry run must not send: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"records":[
			{"id":1,"phoneNumber":"+15551110000","usageType":"DirectNumber","type":"VoiceFax","features":["SmsSender"],"extension":{"id":777,"extensionNumber":"101","name":"Alice"}},
			{"id":2,"phoneNumber":"+15559990000","usageType":"DirectNumber","type":"VoiceFax","features":["SmsSender"],"extension":{"id":888,"extensionNumber":"102","name":"Bob"}}
		],"paging":{"page":1,"totalPages":1}}`))
	})

	report, err := smsprobe.Run(context.Background(), smsprobe.Config{
		Cred: probeCred(srv.URL), To: "+15552220000", DryRun: true,
	})
	require.NoError(t, err)

	assert.Equal(t, "777", report.OwnerExtensionID)
	assert.Equal(t, "+15551110000", report.Own.From, "the control sends from our own extension's number")
	require.Len(t, report.Shared, 2, "the shared number is tried through our extension and through its own")
	assert.Equal(t, "+15559990000", report.Shared[0].From)
	assert.Equal(t, "~", report.Shared[0].ExtensionID)
	assert.Equal(t, "888", report.Shared[1].ExtensionID)
	assert.Equal(t, smsprobe.AnswerInconclusive, report.Verdict.Answer)

	for _, p := range *paths {
		assert.NotContains(t, p, "/sms")
	}
}

func TestRender_UnsentAttemptIsNotPrintedAsARefusal(t *testing.T) {
	r := refusedReport()
	r.Shared = []smsprobe.Attempt{{
		Label:       "shared number, sent through OUR extension (the question)",
		ExtensionID: "~",
		From:        "+15559990000",
		To:          "+15552220000",
		Text:        "probe",
	}}
	r.Own = smsprobe.Attempt{Label: "control", ExtensionID: "~", From: "+15551110000", To: "+15552220000"}
	r.Verdict = smsprobe.Classify(r.Own, r.Shared)

	out := smsprobe.Render(r)

	assert.Contains(t, out, "not attempted")
	assert.NotContains(t, out, "**refused**",
		"a dry run that reads as a refusal would put an invented RingCentral answer into the ticket")
}

func TestRender_ExtensionColumnHasNoEmptyBrackets(t *testing.T) {
	r := refusedReport()
	r.Numbers = []ringcentral.PhoneNumber{
		{PhoneNumber: "+15551110000", UsageType: "DirectNumber", Type: "VoiceFax",
			ExtensionID: "775117052", ExtensionNumber: "143"},
	}
	r.Verdict = smsprobe.Classify(r.Own, r.Shared)

	out := smsprobe.Render(r)

	assert.Contains(t, out, "143 (id 775117052)")
	assert.NotContains(t, out, "(, id", "a nameless extension must not print a dangling comma")
}

func TestRender_SaysWhetherOurExtensionMaySendSMS(t *testing.T) {
	r := refusedReport()
	r.OwnerSMS = smsprobe.FeatureCheck{Known: true, Available: false, Reason: "AccountLimitation The feature is turned off for the current account"}
	r.Verdict = smsprobe.Classify(r.Own, r.Shared)

	out := smsprobe.Render(r)

	assert.Contains(t, out, "SMSSending")
	assert.Contains(t, out, "turned off for the current account")
}

// featureServer answers the extension-features endpoint with one SMSSending
// record and the phone-number listing with two numbers on two extensions.
func featureServer(t *testing.T, smsAvailable bool) (*httptest.Server, *[]string) {
	t.Helper()
	srv, paths, _ := probeServer(t, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/features") {
			reason := ""
			if !smsAvailable {
				reason = `,"reason":{"code":"AccountLimitation","message":"The feature is turned off for the current account"}`
			}
			_, _ = w.Write([]byte(`{"records":[{"id":"SMSSending","available":` +
				map[bool]string{true: "true", false: "false"}[smsAvailable] + reason + `}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"records":[
			{"id":1,"phoneNumber":"+15551110000","usageType":"DirectNumber","type":"VoiceFax","extension":{"id":777,"extensionNumber":"101","name":"Alice"}},
			{"id":2,"phoneNumber":"+15559990000","usageType":"DirectNumber","type":"VoiceFax","extension":{"id":888,"extensionNumber":"102","name":"Bob"}}
		],"paging":{"page":1,"totalPages":1}}`))
	})
	return srv, paths
}

func TestRun_StopsBeforeSendingWhenOurExtensionMayNotText(t *testing.T) {
	srv, paths := featureServer(t, false)

	report, err := smsprobe.Run(context.Background(), smsprobe.Config{
		Cred: probeCred(srv.URL), To: "+15552220000",
	})
	require.NoError(t, err)

	assert.Equal(t, smsprobe.AnswerInconclusive, report.Verdict.Answer)
	assert.Contains(t, report.Verdict.Reason, "may not send SMS")
	for _, p := range *paths {
		assert.NotContains(t, p, "/sms", "no message may be spent to discover a switched-off account")
	}
}

func TestSMSSendingAvailable_ReadsOurOwnExtensionOnly(t *testing.T) {
	srv, paths := featureServer(t, true)

	client, err := ringcentral.NewClientWithCred(probeCred(srv.URL))
	require.NoError(t, err)

	available, found, reason, err := client.SMSSendingAvailable(context.Background())
	require.NoError(t, err)
	assert.True(t, available)
	assert.True(t, found)
	assert.Empty(t, reason)
	assert.Contains(t, (*paths)[0], "/extension/~/features",
		"asking about another extension answers 403 for a plain user, which this client reads as a revoked credential")
}

func TestReuseAccessToken_ExchangesTheJWTOncePerRun(t *testing.T) {
	var exchanges int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == tokenPath {
			exchanges++
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600,"owner_id":"777"}`))
			return
		}
		_, _ = w.Write([]byte(`{"records":[]}`))
	}))
	t.Cleanup(srv.Close)

	client, err := ringcentral.NewClientWithCred(probeCred(srv.URL))
	require.NoError(t, err)

	for range 3 {
		_, _, err := client.AccessToken(context.Background())
		require.NoError(t, err)
	}
	assert.Equal(t, 3, exchanges, "a service must keep exchanging, so a revoked credential surfaces on the next call")

	client.ReuseAccessToken()
	for range 3 {
		_, _, err := client.AccessToken(context.Background())
		require.NoError(t, err)
	}
	assert.Equal(t, 4, exchanges, "a one-shot tool exchanges once, or RingCentral's auth rate limit ends the run early")
}
