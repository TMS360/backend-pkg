package tests

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TMS360/backend-pkg/client/ringcentral"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DEV-1899 — fax a load document to a broker.
//
// The one thing that makes fax different from SMS, and the reason the ticket
// insists on a result per number: RingCentral keeps a status and a reason for
// EACH recipient inside to[]. A client that collapsed them would make a retry
// after a partial failure re-send the document to the brokers who already have
// it — which is a phone call from an annoyed broker, not a cosmetic bug.

// capturedFax is one request the client actually sent, already unpacked.
type capturedFax struct {
	path       string
	settings   map[string]any
	attachName string
	attachType string
	attachBody string
}

// faxServer mounts the token endpoint and unpacks the multipart body of
// everything else, so a test asserts on parts rather than on a blob.
func faxServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) (*httptest.Server, *[]capturedFax) {
	t.Helper()
	var sent []capturedFax
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == tokenPath {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600,"owner_id":"777"}`))
			return
		}

		got := capturedFax{path: r.URL.Path}
		if mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err == nil {
			if boundary, ok := params["boundary"]; ok {
				assert.Equal(t, "multipart/form-data", mediaType)
				mr := multipart.NewReader(r.Body, boundary)
				for {
					part, err := mr.NextPart()
					if err != nil {
						break
					}
					body, _ := io.ReadAll(part)
					switch part.FormName() {
					case "json":
						assert.Equal(t, "application/json", part.Header.Get("Content-Type"))
						_ = json.Unmarshal(body, &got.settings)
					case "attachment":
						got.attachName = part.FileName()
						got.attachType = part.Header.Get("Content-Type")
						got.attachBody = string(body)
					}
				}
			}
		}
		sent = append(sent, got)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, &sent
}

func faxClient(t *testing.T, server string) *ringcentral.Client {
	t.Helper()
	client, err := ringcentral.NewClientWithCred(probeCred(server))
	require.NoError(t, err)
	return client
}

func TestSendFax_PackagesSettingsAndDocumentAsSeparateParts(t *testing.T) {
	srv, sent := faxServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":9001,"messageStatus":"Queued","faxPageCount":3,
			"to":[{"phoneNumber":"+15551110000","messageStatus":"Queued"}]}`))
	})

	res, err := faxClient(t, srv.URL).SendFax(context.Background(), ringcentral.FaxRequest{
		To:          []string{"+15551110000"},
		Filename:    "rate-con.pdf",
		ContentType: "application/pdf",
		Content:     []byte("%PDF-1.4 fake"),
		Resolution:  "High",
	})
	require.NoError(t, err)

	assert.Equal(t, "9001", res.ID)
	assert.Equal(t, 3, res.PageCount, "the office person is promised a page count")

	require.Len(t, *sent, 1)
	got := (*sent)[0]

	// An empty ExtensionID means "the extension the credential belongs to" —
	// a fax leaves as the company, so it never needs a person's extension.
	assert.Equal(t, "/restapi/v1.0/account/~/extension/~/fax", got.path)

	assert.Equal(t, []any{map[string]any{"phoneNumber": "+15551110000"}}, got.settings["to"])
	assert.Equal(t, "High", got.settings["faxResolution"])

	assert.Equal(t, "rate-con.pdf", got.attachName)
	assert.Equal(t, "application/pdf", got.attachType,
		"RingCentral renders by this type; announcing the wrong one comes back as RenderingFailed")
	assert.Equal(t, "%PDF-1.4 fake", got.attachBody)
}

func TestSendFax_KeepsEachRecipientsOwnAnswer(t *testing.T) {
	// AC2: three numbers, one of them bad, is two deliveries and one failure
	// with its own reason — never one shared error.
	srv, _ := faxServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":42,"messageStatus":"Sent","faxPageCount":2,"to":[
			{"phoneNumber":"+15551110000","messageStatus":"Delivered"},
			{"phoneNumber":"+15552220000","messageStatus":"SendingFailed","faxErrorCode":"NoAnswer"},
			{"phoneNumber":"+15553330000","messageStatus":"Delivered"}]}`))
	})

	res, err := faxClient(t, srv.URL).SendFax(context.Background(), ringcentral.FaxRequest{
		To:          []string{"+15551110000", "+15552220000", "+15553330000"},
		Filename:    "bol.pdf",
		ContentType: "application/pdf",
		Content:     []byte("doc"),
	})
	require.NoError(t, err)

	require.Len(t, res.Recipients, 3)
	assert.Equal(t, ringcentral.FaxRecipientResult{
		PhoneNumber: "+15551110000", MessageStatus: "Delivered",
	}, res.Recipients[0])
	assert.Equal(t, ringcentral.FaxRecipientResult{
		PhoneNumber: "+15552220000", MessageStatus: "SendingFailed", FaxErrorCode: "NoAnswer",
	}, res.Recipients[1], "the reason belongs to the number that failed, not to the send")
	assert.Equal(t, "Delivered", res.Recipients[2].MessageStatus)
}

func TestSendFax_RecipientWithoutItsOwnStatusInheritsTheMessage(t *testing.T) {
	// RingCentral answers the POST before it has judged anybody. A caller must
	// not have to treat "" as a fourth state.
	srv, _ := faxServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":7,"messageStatus":"Queued","to":[{"phoneNumber":"+1555"}]}`))
	})

	res, err := faxClient(t, srv.URL).SendFax(context.Background(), ringcentral.FaxRequest{
		To: []string{"+1555"}, Filename: "a.pdf", ContentType: "application/pdf", Content: []byte("x"),
	})
	require.NoError(t, err)

	require.Len(t, res.Recipients, 1)
	assert.Equal(t, "Queued", res.Recipients[0].MessageStatus)
}

func TestSendFax_RefusesAnUnsendableFaxBeforeTheNetwork(t *testing.T) {
	// A fax costs money per recipient and prints paper on somebody's machine.
	// Nothing that cannot possibly succeed should reach RingCentral.
	cases := map[string]ringcentral.FaxRequest{
		"no recipients":   {Filename: "a.pdf", ContentType: "application/pdf", Content: []byte("x")},
		"empty document":  {To: []string{"+1555"}, Filename: "a.pdf", ContentType: "application/pdf"},
		"no filename":     {To: []string{"+1555"}, ContentType: "application/pdf", Content: []byte("x")},
		"no content type": {To: []string{"+1555"}, Filename: "a.pdf", Content: []byte("x")},
		"blank recipient": {To: []string{"  "}, Filename: "a.pdf", ContentType: "application/pdf",
			Content: []byte("x")},
	}

	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			srv, sent := faxServer(t, func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"id":1}`))
			})

			_, err := faxClient(t, srv.URL).SendFax(context.Background(), req)

			require.Error(t, err)
			assert.Empty(t, *sent, "the refusal must happen before RingCentral is asked")
		})
	}
}

func TestSendFax_KeepsRingCentralsRefusalVerbatim(t *testing.T) {
	const body = `{"errorCode":"InvalidParameter","message":"Parameter [to] value is invalid","errors":[{"errorCode":"CMN-101","message":"Phone number is not valid","parameterName":"to"}]}`
	srv, _ := faxServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(body))
	})

	_, err := faxClient(t, srv.URL).SendFax(context.Background(), ringcentral.FaxRequest{
		To: []string{"+1"}, Filename: "a.pdf", ContentType: "application/pdf", Content: []byte("x"),
	})
	require.Error(t, err)

	apiErr, ok := ringcentral.AsAPIError(err)
	require.True(t, ok, "a platform refusal must survive as *APIError, not as a flattened string")
	assert.Equal(t, "InvalidParameter", apiErr.Code)
	assert.Equal(t, []string{"CMN-101"}, apiErr.SubCodes())
	assert.Equal(t, body, apiErr.Body)
}

func TestSendFax_RevokedCredentialIsNotAPlatformRefusal(t *testing.T) {
	srv, _ := faxServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	_, err := faxClient(t, srv.URL).SendFax(context.Background(), ringcentral.FaxRequest{
		To: []string{"+1"}, Filename: "a.pdf", ContentType: "application/pdf", Content: []byte("x"),
	})
	assert.ErrorIs(t, err, ringcentral.ErrInvalidCredentials)
}

func TestSendFax_ErrorsDoNotLeakSecrets(t *testing.T) {
	srv, _ := faxServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"errorCode":"X","message":"nope"}`))
	})

	_, err := faxClient(t, srv.URL).SendFax(context.Background(), ringcentral.FaxRequest{
		To: []string{"+1"}, Filename: "a.pdf", ContentType: "application/pdf", Content: []byte("x"),
	})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "secret")
	assert.NotContains(t, err.Error(), "jwt")
}

func TestFaxMessage_ReadsTheCurrentStatusBack(t *testing.T) {
	// This is how a Queued fax becomes Delivered, or names why it failed.
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == tokenPath {
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600,"owner_id":"777"}`))
			return
		}
		path = r.URL.Path
		_, _ = w.Write([]byte(`{"id":9001,"messageStatus":"Delivered","faxPageCount":3,
			"to":[{"phoneNumber":"+15551110000","messageStatus":"Delivered"}]}`))
	}))
	t.Cleanup(srv.Close)

	res, err := faxClient(t, srv.URL).FaxMessage(context.Background(), "", "9001")
	require.NoError(t, err)

	assert.Equal(t, "/restapi/v1.0/account/~/extension/~/message-store/9001", path)
	assert.Equal(t, "Delivered", res.MessageStatus)
	assert.Equal(t, 3, res.PageCount)
	require.Len(t, res.Recipients, 1)
	assert.Equal(t, "Delivered", res.Recipients[0].MessageStatus)
}

func TestFaxMessage_RequiresAMessageID(t *testing.T) {
	srv, sent := faxServer(t, func(w http.ResponseWriter, _ *http.Request) {})

	_, err := faxClient(t, srv.URL).FaxMessage(context.Background(), "", "  ")

	require.Error(t, err)
	assert.Empty(t, *sent)
}

func TestFaxStatusIsTerminal(t *testing.T) {
	// AC4: RingCentral redials a busy line for minutes. A fax still being tried
	// is "waiting", and calling it a failure tells an office person a broker
	// never got a document that arrives two minutes later.
	assert.False(t, ringcentral.FaxStatusIsTerminal(ringcentral.FaxStatusQueued))
	assert.False(t, ringcentral.FaxStatusIsTerminal(ringcentral.FaxStatusSent))
	assert.False(t, ringcentral.FaxStatusIsTerminal(""), "an unknown word is not an ending")

	assert.True(t, ringcentral.FaxStatusIsTerminal(ringcentral.FaxStatusDelivered))
	assert.True(t, ringcentral.FaxStatusIsTerminal(ringcentral.FaxStatusDeliveryFailed))
	assert.True(t, ringcentral.FaxStatusIsTerminal(ringcentral.FaxStatusSendingFailed))
}

func TestPhoneNumber_CanFax(t *testing.T) {
	assert.True(t, ringcentral.PhoneNumber{Type: "VoiceFax"}.CanFax())
	assert.True(t, ringcentral.PhoneNumber{Type: "FaxOnly"}.CanFax())
	assert.False(t, ringcentral.PhoneNumber{Type: "VoiceOnly"}.CanFax())
}
