package ringcentral

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	callLogPath = "/restapi/v1.0/account/~/call-log"

	// callLogPerPage mirrors numbersPerPage: RingCentral's maximum page size, so
	// a busy account finishes in as few round trips as possible and a quiet one
	// still finishes in a single call.
	callLogPerPage = 1000
	// maxCallLogPages bounds the pagination loop so a broken paging response can
	// never spin forever. Hitting it is reported through Truncated rather than
	// silently dropping the tail — a caller that quietly loses calls after an
	// outage looks exactly like a caller that had no calls.
	maxCallLogPages = 20

	// recordingTimeout covers a whole audio download. The shared client's 30s
	// budget is measured across the entire request INCLUDING the body read,
	// which would cut a long recording off mid-stream.
	recordingTimeout = 5 * time.Minute

	// mediaHostSuffix is the only host family a recording may be fetched from.
	// contentUri is vendor-controlled input that has travelled through our
	// database before being turned back into an outbound request, so it is an
	// SSRF boundary and gets checked like one.
	mediaHostSuffix = ".ringcentral.com"
)

// CallLogQuery selects the window to read. Both bounds are required: the caller
// owns the watermark, because only it knows what it has already stored.
type CallLogQuery struct {
	From time.Time
	To   time.Time
}

// CallParty is one end of a call as RingCentral reports it. ExtensionID is the
// reliable half for attributing an office person: the phone number on an
// outbound leg is the extension's configured caller ID, which is very often the
// company's main number rather than the direct line the call was placed from.
type CallParty struct {
	PhoneNumber     string
	ExtensionID     string
	ExtensionNumber string
	Name            string
}

// Recording points at audio RingCentral is holding for us. It is a pointer on
// CallRecord and not a value, because a call shorter than about thirty seconds
// produces no recording object at all — that is RingCentral policy, not an
// error, and the difference has to survive into the caller.
type Recording struct {
	ID   string
	Type string
	// ContentURI is absolute and lives on RingCentral's media host, which is a
	// different host from the platform API. Use it verbatim; see FetchRecording.
	ContentURI string
}

// CallRecord is one row of the company call log.
type CallRecord struct {
	// ID is RingCentral's call-log record id — the natural idempotency key for
	// storing a call exactly once.
	ID string
	// SessionID is telephonySessionId, shared by every leg of a call that was
	// transferred. Empty when RingCentral does not report it.
	SessionID string
	// Direction is "Inbound" or "Outbound", Result is RingCentral's own word
	// ("Accepted", "Missed", "No Answer", "Voicemail", ...). Both are passed
	// through verbatim: a caller that invents its own vocabulary here ends up
	// claiming a call completed when nobody knows that it did.
	Direction string
	Result    string
	From      CallParty
	To        CallParty
	StartTime time.Time
	// DurationSeconds is RingCentral's own duration for the leg.
	DurationSeconds int
	Recording       *Recording
}

// CallLog is a window of the company call log plus whether it was complete.
type CallLog struct {
	Records []CallRecord
	// Truncated reports that maxCallLogPages was reached with pages still
	// unread. The window held more than we returned; narrow it and read again.
	Truncated bool
}

// flexID decodes an identifier that RingCentral may send as either a JSON
// string or a JSON number, and yields a string either way.
//
// This is not defensive padding: within one API, call-log record ids arrive as
// strings ("AbC123" — they are not numeric at all) while phone-number and
// extension ids arrive as bare numbers. Committing each field to one of the two
// shapes means a vendor change to any of them fails the whole page decode and
// silently stops the call log, so the decoder accepts both and the rest of the
// system keeps treating ids as opaque.
type flexID string

func (f *flexID) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "" || s == "null" {
		*f = ""
		return nil
	}
	if s[0] == '"' {
		var str string
		if err := json.Unmarshal(b, &str); err != nil {
			return err
		}
		*f = flexID(str)
		return nil
	}
	var num json.Number
	if err := json.Unmarshal(b, &num); err != nil {
		return err
	}
	*f = flexID(num.String())
	return nil
}

func (f flexID) String() string { return string(f) }

// callLogRecord mirrors the RingCentral payload. Every identifier goes through
// flexID, so the decode does not depend on which of them the vendor currently
// sends as a number.
type callLogRecord struct {
	ID        flexID `json:"id"`
	SessionID string `json:"sessionId"`
	// TelephonySessionID is present in both Simple and Detailed views on the
	// current API but is absent from older published specs, so it is decoded
	// optionally and falls back to SessionID. A vendor surprise then costs
	// "transferred legs are not grouped", not a failed poll.
	TelephonySessionID string        `json:"telephonySessionId"`
	Direction          string        `json:"direction"`
	Result             string        `json:"result"`
	Type               string        `json:"type"`
	StartTime          string        `json:"startTime"`
	Duration           int           `json:"duration"`
	From               callLogParty  `json:"from"`
	To                 callLogParty  `json:"to"`
	Recording          *callLogRecrd `json:"recording"`
}

type callLogParty struct {
	PhoneNumber     string `json:"phoneNumber"`
	ExtensionID     flexID `json:"extensionId"`
	ExtensionNumber string `json:"extensionNumber"`
	Name            string `json:"name"`
}

type callLogRecrd struct {
	ID         flexID `json:"id"`
	Type       string `json:"type"`
	ContentURI string `json:"contentUri"`
}

type callLogPage struct {
	Records []callLogRecord `json:"records"`
	Paging  struct {
		Page       int `json:"page"`
		TotalPages int `json:"totalPages"`
	} `json:"paging"`
}

// ListCallLog returns the company's call-log records in [q.From, q.To],
// following pagination.
//
// The detailed view is requested because the simple one omits the recording
// pointer and the extension ids, which are the two things attribution needs.
//
// ErrInvalidCredentials means the stored credential no longer authenticates —
// and, for now, ALSO means "the app is missing the ReadCallLog scope", because
// the shared get helper maps 401 and 403 to the same sentinel and widening that
// would regress how DEV-1757's Settings screen classifies a failure. So a
// tenant whose app lacks the scope is told to reconnect, which will not help.
// See ErrInsufficientPermissions; splitting the two is its own ticket.
func (c *Client) ListCallLog(ctx context.Context, q CallLogQuery) (CallLog, error) {
	if q.From.IsZero() || q.To.IsZero() {
		return CallLog{}, fmt.Errorf("ringcentral: call log window requires both From and To")
	}
	if q.To.Before(q.From) {
		return CallLog{}, fmt.Errorf("ringcentral: call log window ends before it starts")
	}

	token, _, err := c.AccessToken(ctx)
	if err != nil {
		if IsAuthError(err) {
			return CallLog{}, ErrInvalidCredentials
		}
		return CallLog{}, err
	}

	out := CallLog{}
	for page := 1; page <= maxCallLogPages; page++ {
		v := url.Values{}
		v.Set("view", "Detailed")
		v.Set("dateFrom", q.From.UTC().Format(time.RFC3339))
		v.Set("dateTo", q.To.UTC().Format(time.RFC3339))
		v.Set("perPage", strconv.Itoa(callLogPerPage))
		v.Set("page", strconv.Itoa(page))

		body, err := c.get(ctx, token, callLogPath+"?"+v.Encode())
		if err != nil {
			return CallLog{}, err
		}

		var decoded callLogPage
		if err := json.Unmarshal(body, &decoded); err != nil {
			return CallLog{}, fmt.Errorf("ringcentral: failed to decode call log: %w", err)
		}
		for _, r := range decoded.Records {
			out.Records = append(out.Records, r.toCallRecord())
		}

		if decoded.Paging.TotalPages <= page || len(decoded.Records) == 0 {
			return out, nil
		}
		if page == maxCallLogPages {
			// Say so rather than returning a short window that reads as "quiet".
			out.Truncated = true
		}
	}
	return out, nil
}

func (r callLogRecord) toCallRecord() CallRecord {
	out := CallRecord{
		ID:              r.ID.String(),
		SessionID:       strings.TrimSpace(r.TelephonySessionID),
		Direction:       r.Direction,
		Result:          r.Result,
		From:            r.From.toParty(),
		To:              r.To.toParty(),
		DurationSeconds: r.Duration,
	}
	if out.SessionID == "" {
		out.SessionID = strings.TrimSpace(r.SessionID)
	}
	// RingCentral timestamps are RFC3339 with an offset. An unparseable one
	// leaves the zero time for the caller to reject; it must not fail the whole
	// page, or one malformed record would hide every good call beside it.
	if t, err := time.Parse(time.RFC3339, r.StartTime); err == nil {
		out.StartTime = t.UTC()
	}
	if r.Recording != nil {
		out.Recording = &Recording{
			ID:         r.Recording.ID.String(),
			Type:       r.Recording.Type,
			ContentURI: strings.TrimSpace(r.Recording.ContentURI),
		}
	}
	return out
}

func (p callLogParty) toParty() CallParty {
	return CallParty{
		PhoneNumber:     strings.TrimSpace(p.PhoneNumber),
		ExtensionID:     p.ExtensionID.String(),
		ExtensionNumber: p.ExtensionNumber,
		Name:            p.Name,
	}
}

// FetchRecording opens the audio behind a Recording.ContentURI and hands the
// caller an unread body to stream. The caller owns closing it.
//
// This deliberately does NOT go through the shared get helper, for three
// independent reasons:
//
//   - get prepends c.serverURL, but contentUri is absolute and points at
//     RingCentral's media host. Reconstructing a path against the platform host
//     earns a cross-host redirect, and Go strips the Authorization header across
//     one, which surfaces as a silent 401.
//   - get does io.ReadAll. This is audio; buffering a long call in memory to
//     immediately write it out again is exactly what streaming avoids.
//   - get forces Accept: application/json, and the response is audio/mpeg or
//     audio/x-wav.
//
// The returned string is the Content-Type RingCentral served, so a caller can
// pass it through rather than guessing the codec.
func (c *Client) FetchRecording(ctx context.Context, contentURI string) (io.ReadCloser, string, error) {
	// Guard first, and separately: nothing below this line may run for a host we
	// have not vetted, because everything below it sends a bearer token.
	if err := checkMediaURI(contentURI); err != nil {
		return nil, "", err
	}
	return c.fetchRecordingAt(ctx, contentURI)
}

// fetchRecordingAt is the transport half of FetchRecording, split out so the
// streaming and status-mapping behaviour is testable against a local stub — a
// test server can never be given a .ringcentral.com host, and weakening the
// guard to make it testable would defeat the guard. Callers must go through
// FetchRecording; this one trusts the URI it is handed.
func (c *Client) fetchRecordingAt(ctx context.Context, contentURI string) (io.ReadCloser, string, error) {
	token, _, err := c.AccessToken(ctx)
	if err != nil {
		if IsAuthError(err) {
			return nil, "", ErrInvalidCredentials
		}
		return nil, "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, contentURI, nil)
	if err != nil {
		return nil, "", fmt.Errorf("ringcentral: failed to create recording request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{
		Timeout: recordingTimeout,
		// Do not follow redirects: a redirect to another host would either drop
		// the Authorization header or carry it somewhere we never vetted.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("ringcentral: network error fetching recording: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		_ = resp.Body.Close()
		return nil, "", ErrInvalidCredentials
	}
	if resp.StatusCode == http.StatusForbidden {
		_ = resp.Body.Close()
		return nil, "", ErrInsufficientPermissions
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Read a bounded prefix only: an unexpected HTML error page must not be
		// pulled into memory in full, and describe() truncates what we do read.
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		var er errorResponse
		_ = json.Unmarshal(body, &er)
		return nil, "", fmt.Errorf("ringcentral: recording request failed with status %d: %s",
			resp.StatusCode, describe(er, body))
	}

	return resp.Body, resp.Header.Get("Content-Type"), nil
}

// checkMediaURI refuses anything that is not an https URL on RingCentral's own
// media hosts. contentUri arrives from the vendor and is stored before it is
// ever dialled, so by the time it becomes an outbound request it is untrusted
// input crossing a trust boundary — the check belongs here, not at the caller.
//
// The suffix form covers production (media.ringcentral.com) and the developer
// sandbox alike, since Cred.ServerURL explicitly supports the sandbox host.
func checkMediaURI(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("ringcentral: recording URI is not a URL: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("ringcentral: recording URI must be https, got %q", u.Scheme)
	}
	// Lower-cased because host names are case-insensitive and the suffix test is
	// not: without this a perfectly good "MEDIA.RingCentral.com" would be
	// refused. Note url.Hostname() strips any userinfo, so
	// "https://media.ringcentral.com@evil.example.com/x" yields "evil.example.com"
	// and is refused, which is the point.
	host := strings.ToLower(u.Hostname())
	if host != "ringcentral.com" && !strings.HasSuffix(host, mediaHostSuffix) {
		// The host is deliberately not echoed further than this error, and this
		// error is not shown to an end user.
		return fmt.Errorf("ringcentral: refusing to fetch a recording from host %q", host)
	}
	return nil
}
