package toll

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// BestPassAPIBaseURL is the BestPass Fleet API v1 production endpoint. A
// credential's BaseURL overrides it (sandbox, tests).
const BestPassAPIBaseURL = "https://api.bestpass.com/fleet-api/v1"

// BestPassMaxWindow is the widest post-date window one /transactions call
// accepts.
const BestPassMaxWindow = 31 * 24 * time.Hour

const (
	bestPassPageSize = 10000
	// bestPassMaxPages stops a server that never returns a null nextPage.
	bestPassMaxPages = 1000
	// bestPassMaxRetryWait caps one Retry-After; the limit is 100 requests a
	// minute, so a longer wait is a misbehaving server.
	bestPassMaxRetryWait = 2 * time.Minute
	bestPassMaxRetries   = 5
)

// BestPassAccount is one account a key can read.
type BestPassAccount struct {
	Number string `json:"accountNumber"`
	Name   string `json:"accountName"`
}

// BestPassStatement is one monthly statement of an account.
type BestPassStatement struct {
	Number      string    `json:"statementNumber"`
	DateCreated time.Time `json:"dateCreated"`
}

// BestPassAPI reads tolls from the BestPass Fleet API with one company's key.
// Read only: nothing is ever written to BestPass.
type BestPassAPI struct {
	baseURL string
	key     string
	http    *http.Client
	sleep   func(context.Context, time.Duration) error
}

// NewBestPassAPI builds a client from the company's BestPass credential.
func NewBestPassAPI(cred Credential) *BestPassAPI {
	base := strings.TrimRight(strings.TrimSpace(cred.BaseURL), "/")
	if base == "" {
		base = BestPassAPIBaseURL
	}
	return &BestPassAPI{
		baseURL: base,
		key:     cred.Secret,
		http:    &http.Client{Timeout: 60 * time.Second},
		sleep:   sleepCtx,
	}
}

// Accounts lists the accounts the key can read. It is also the key check:
// a refused key answers *AuthError.
func (c *BestPassAPI) Accounts(ctx context.Context) ([]BestPassAccount, error) {
	var out []BestPassAccount
	if err := c.getJSON(ctx, "/permissions/accounts", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Transactions reads every toll of one account posted in [from, to).
func (c *BestPassAPI) Transactions(ctx context.Context, account string, from, to time.Time) (ParseResult, error) {
	if !to.After(from) || to.Sub(from) > BestPassMaxWindow {
		return ParseResult{}, fmt.Errorf("toll: bestpass window %s..%s must be positive and at most 31 days", from.Format(time.RFC3339), to.Format(time.RFC3339))
	}
	q := url.Values{}
	q.Set("accountNumber", account)
	q.Set("startDate", from.UTC().Format(time.RFC3339))
	q.Set("endDate", to.UTC().Format(time.RFC3339))
	return c.transactions(ctx, account, q)
}

// StatementTransactions reads every toll on one statement: the final amounts
// once BestPass has closed the month.
func (c *BestPassAPI) StatementTransactions(ctx context.Context, account, statement string) (ParseResult, error) {
	q := url.Values{}
	q.Set("accountNumber", account)
	q.Set("statementNumber", statement)
	return c.transactions(ctx, account, q)
}

// Statements lists one account's statements.
func (c *BestPassAPI) Statements(ctx context.Context, account string) ([]BestPassStatement, error) {
	q := url.Values{}
	q.Set("accountNumber", account)
	return getAllPages[BestPassStatement](ctx, c, "/statements", q)
}

func (c *BestPassAPI) transactions(ctx context.Context, account string, q url.Values) (ParseResult, error) {
	raws, err := getAllPages[json.RawMessage](ctx, c, "/transactions", q)
	if err != nil {
		return ParseResult{}, err
	}
	return rowsFromTransactions(raws, account), nil
}

// bestPassPage is the paged envelope every list answer comes in.
type bestPassPage[T any] struct {
	PagingInfo struct {
		NextPage *string `json:"nextPage"`
	} `json:"pagingInfo"`
	Results []T `json:"results"`
}

// getAllPages walks pageNumber 1, 2, … until nextPage is null.
func getAllPages[T any](ctx context.Context, c *BestPassAPI, path string, q url.Values) ([]T, error) {
	var out []T
	for page := 1; page <= bestPassMaxPages; page++ {
		pq := cloneValues(q)
		pq.Set("pageSize", strconv.Itoa(bestPassPageSize))
		pq.Set("pageNumber", strconv.Itoa(page))
		var p bestPassPage[T]
		if err := c.getJSON(ctx, path, pq, &p); err != nil {
			return nil, fmt.Errorf("toll: bestpass %s page %d: %w", path, page, err)
		}
		out = append(out, p.Results...)
		if p.PagingInfo.NextPage == nil || *p.PagingInfo.NextPage == "" {
			return out, nil
		}
	}
	return nil, fmt.Errorf("toll: bestpass %s: more than %d pages", path, bestPassMaxPages)
}

// getJSON GETs one resource, waiting out 429s, and decodes the body.
func (c *BestPassAPI) getJSON(ctx context.Context, path string, q url.Values, dst any) error {
	for attempt := 0; ; attempt++ {
		resp, err := c.get(ctx, path, q)
		if err != nil {
			return err
		}
		wait, retry := retryAfter(resp, attempt)
		if !retry {
			return decodeBestPass(resp, dst)
		}
		_ = resp.Body.Close()
		if err := c.sleep(ctx, wait); err != nil {
			return err
		}
	}
}

func (c *BestPassAPI) get(ctx context.Context, path string, q url.Values) (*http.Response, error) {
	u := c.baseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", c.key)
	req.Header.Set("Accept", "application/json")
	return c.http.Do(req)
}

// retryAfter says whether resp is a 429 worth waiting out, and for how long.
func retryAfter(resp *http.Response, attempt int) (time.Duration, bool) {
	if resp.StatusCode != http.StatusTooManyRequests || attempt >= bestPassMaxRetries {
		return 0, false
	}
	secs, err := strconv.Atoi(strings.TrimSpace(resp.Header.Get("Retry-After")))
	if err != nil || secs < 0 {
		secs = 1
	}
	return min(time.Duration(secs)*time.Second, bestPassMaxRetryWait), true
}

// decodeBestPass maps the status to an error and decodes a 2xx body.
func decodeBestPass(resp *http.Response, dst any) error {
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return &AuthError{ProviderType: ProviderBestPass, Cause: fmt.Errorf("HTTP %d", resp.StatusCode)}
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("decode: %w", err)
	}
	return nil
}

func cloneValues(q url.Values) url.Values {
	out := make(url.Values, len(q)+2)
	for k, v := range q {
		out[k] = append([]string(nil), v...)
	}
	return out
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
