package factoring

import (
	"context"
	"testing"
	"time"

	"github.com/TMS360/backend-pkg/deployenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// inProduction marks the test process as the Railway production deployment,
// where providers dial the real factor hosts.
func inProduction(t *testing.T) {
	t.Helper()
	t.Setenv(deployenv.RailwayEnvVar, "production")
}

// onRailway sets the Railway environment name and clears every TEST_* host,
// so each test states exactly which overrides exist.
func onRailway(t *testing.T, name string) {
	t.Helper()
	t.Setenv(deployenv.RailwayEnvVar, name)
	for _, v := range []string{envTestTriumphSFTPHost, envTestSFTPHostLegacy, envTestRTSSFTPHost} {
		t.Setenv(v, "")
	}
}

// recordDials swaps the network for a fake and reports every host dialed.
func recordDials(dialFn *func(context.Context, sftpDialer) (sftpUploader, error)) *[]string {
	var hosts []string
	*dialFn = func(_ context.Context, d sftpDialer) (sftpUploader, error) {
		hosts = append(hosts, d.Host)
		return &fakeUploader{}, nil
	}
	return &hosts
}

func oneInvoiceBatch() Batch {
	return Batch{
		SubmittedAt: time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC),
		Invoices:    []InvoiceLine{{DebtorName: "ABC", InvoiceNumber: "INV-1", InvoiceDate: time.Now(), PONumber: "p", AmountUSD: 100}},
		PDFs:        []InvoicePDF{{InvoiceNumber: "INV-1", Bytes: []byte("pdf")}},
	}
}

// A staging credential carries the carrier's real password (the DB is a
// production copy) — the guard must hold regardless.
var realCredential = Credential{Username: "carrier", Password: "real-password"}

func triumph(t *testing.T) (*TriumphSFTPProvider, *[]string) {
	t.Helper()
	cred := realCredential
	cred.ProviderType = ProviderTriumphSFTP
	p := NewTriumphSFTP(cred)
	return p, recordDials(&p.dialFn)
}

func rtsProviders(t *testing.T) map[ProviderType]*RTSSFTPProvider {
	t.Helper()
	cred := realCredential
	return map[ProviderType]*RTSSFTPProvider{
		ProviderRTSSFTP:     NewRTSSFTP(cred),
		ProviderRTSTestSFTP: NewRTSTestSFTP(cred),
	}
}

// AC1: staging with the drifted APP_ENV=prod still lands in staging SFTPGo.
func TestDEV3306_AC1_StagingWithAppEnvProdDialsTestHost(t *testing.T) {
	onRailway(t, "staging")
	t.Setenv("APP_ENV", "prod")
	t.Setenv(envTestTriumphSFTPHost, "sftpgo.railway.internal")

	p, dialed := triumph(t)
	_, err := p.SubmitBatch(context.Background(), oneInvoiceBatch(), nil)

	require.NoError(t, err)
	assert.Equal(t, []string{"sftpgo.railway.internal"}, *dialed)
}

// AC1 for RTS: both RTS types (the "test" one is RTS's real server too) go to
// the catcher.
func TestDEV3306_AC1_StagingRTSDialsTestHost(t *testing.T) {
	onRailway(t, "staging")
	t.Setenv("APP_ENV", "prod")
	t.Setenv(envTestRTSSFTPHost, "sftpgo.railway.internal")

	for pt, p := range rtsProviders(t) {
		dialed := recordDials(&p.dialFn)
		_, err := p.SubmitBatch(context.Background(), rtsBatch(), nil)
		require.NoError(t, err, pt)
		assert.Equal(t, []string{"sftpgo.railway.internal"}, *dialed, pt)
	}
}

// AC2: no test host outside production → the send stops with the clear error
// and nothing is dialed; the real host is never the fallback.
func TestDEV3306_AC2_MissingTestHostBlocksTriumph(t *testing.T) {
	onRailway(t, "staging")

	p, dialed := triumph(t)
	_, err := p.SubmitBatch(context.Background(), oneInvoiceBatch(), nil)

	require.ErrorIs(t, err, ErrRealServerBlocked)
	assert.Contains(t, err.Error(), "real factoring server is blocked outside production")
	assert.Contains(t, err.Error(), envTestTriumphSFTPHost, "the error names the variable to set")
	assert.ErrorIs(t, p.TestConnection(context.Background()), ErrRealServerBlocked)
	assert.Empty(t, *dialed)
	assert.NotEqual(t, triumphSFTPHost, p.host)
}

func TestDEV3306_AC2_MissingTestHostBlocksRTS(t *testing.T) {
	onRailway(t, "staging")

	for pt, p := range rtsProviders(t) {
		dialed := recordDials(&p.dialFn)
		_, err := p.SubmitBatch(context.Background(), rtsBatch(), nil)
		require.ErrorIs(t, err, ErrRealServerBlocked, pt)
		assert.ErrorIs(t, p.TestConnection(context.Background()), ErrRealServerBlocked, pt)
		assert.Empty(t, *dialed, pt)
	}
}

// AC2: once the address is back, the next provider built for the retry sends
// to the catcher — the block is not sticky.
func TestDEV3306_AC2_RetryWorksAfterTestHostRestored(t *testing.T) {
	onRailway(t, "staging")
	blocked, _ := triumph(t)
	_, err := blocked.SubmitBatch(context.Background(), oneInvoiceBatch(), nil)
	require.ErrorIs(t, err, ErrRealServerBlocked)

	t.Setenv(envTestTriumphSFTPHost, "sftpgo.railway.internal")
	retry, dialed := triumph(t)
	_, err = retry.SubmitBatch(context.Background(), oneInvoiceBatch(), nil)

	require.NoError(t, err)
	assert.Equal(t, []string{"sftpgo.railway.internal"}, *dialed)
}

// AC4: production dials the real factor hosts exactly as before; stray TEST_*
// variables are ignored there.
func TestDEV3306_AC4_ProductionDialsRealHosts(t *testing.T) {
	inProduction(t)
	t.Setenv(envTestTriumphSFTPHost, "sftpgo.railway.internal")
	t.Setenv(envTestRTSSFTPHost, "sftpgo.railway.internal")

	p, dialed := triumph(t)
	_, err := p.SubmitBatch(context.Background(), oneInvoiceBatch(), nil)
	require.NoError(t, err)
	assert.Equal(t, []string{triumphSFTPHost}, *dialed)
	assert.Equal(t, triumphSFTPInboundDir, p.inboundDir)

	for pt, rp := range rtsProviders(t) {
		rtsDialed := recordDials(&rp.dialFn)
		_, err := rp.SubmitBatch(context.Background(), rtsBatch(), nil)
		require.NoError(t, err, pt)
		assert.Equal(t, []string{"ftps.rtsfinancial.com"}, *rtsDialed, pt)
	}
}

// Edge cases: a laptop / CI (no Railway name) and a renamed production are
// not production — with no test host they are blocked, never the real host.
func TestDEV3306_Edge_NoOrRenamedRailwayNameIsBlocked(t *testing.T) {
	for _, name := range []string{"", "Production", "prod", "production-2"} {
		t.Run(name, func(t *testing.T) {
			onRailway(t, name)
			t.Setenv("APP_ENV", "prod")

			p, dialed := triumph(t)
			_, err := p.SubmitBatch(context.Background(), oneInvoiceBatch(), nil)

			require.ErrorIs(t, err, ErrRealServerBlocked)
			assert.Empty(t, *dialed)
		})
	}
}

// The legacy TEST_SFTP_HOST keeps working as the Triumph catcher address.
func TestDEV3306_LegacyTestHostStillHonoured(t *testing.T) {
	onRailway(t, "development")
	t.Setenv(envTestSFTPHostLegacy, "sftpgo-legacy.internal")

	p, dialed := triumph(t)
	_, err := p.SubmitBatch(context.Background(), oneInvoiceBatch(), nil)

	require.NoError(t, err)
	assert.Equal(t, []string{"sftpgo-legacy.internal"}, *dialed)
}
