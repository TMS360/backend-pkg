package toll

import (
	"context"
	"testing"

	"github.com/TMS360/backend-pkg/deployenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// inProduction marks the test process as the Railway production deployment,
// where the stored PrePass host is dialed.
func inProduction(t *testing.T) {
	t.Helper()
	t.Setenv(deployenv.RailwayEnvVar, "production")
}

const realPrePassHost = "sftp.prepass.com"

// realPrePass is a staging row copied out of production: real host, real secret.
func realPrePass(t *testing.T) (*PrePassSFTP, *[]sftpDialer) {
	t.Helper()
	p := NewPrePassSFTP(Credential{
		ProviderType: ProviderPrePassSFTP,
		Host:         realPrePassHost,
		Username:     "carrier",
		Secret:       "real-secret",
		Directory:    "/statements",
	})
	var dials []sftpDialer
	p.dialFn = func(_ context.Context, d sftpDialer) (sftpFetcher, error) {
		dials = append(dials, d)
		return &fakeFetcher{}, nil
	}
	return p, &dials
}

// AC3: on staging with the drifted APP_ENV=prod and no catcher host, the toll
// import stops with a clear error that names the refused host; nothing dials.
func TestDEV3306_AC3_StagingWithoutTestHostBlocksPrePass(t *testing.T) {
	t.Setenv(deployenv.RailwayEnvVar, "staging")
	t.Setenv("APP_ENV", "prod")
	t.Setenv(envTestPrePassHost, "")

	p, dials := realPrePass(t)
	_, err := p.List(context.Background())

	require.ErrorIs(t, err, ErrRealServerBlocked)
	assert.Contains(t, err.Error(), "real PrePass server is blocked outside production")
	assert.Contains(t, err.Error(), realPrePassHost, "the log line names the blocked host")
	assert.Contains(t, err.Error(), envTestPrePassHost)
	assert.ErrorIs(t, p.TestConnection(context.Background()), ErrRealServerBlocked)
	_, err = p.Fetch(context.Background(), "weekly.xlsx")
	assert.ErrorIs(t, err, ErrRealServerBlocked)
	assert.Empty(t, *dials)
}

// AC3: with the catcher host set, staging reads only the test folder.
func TestDEV3306_AC3_StagingReadsOnlyTestFolder(t *testing.T) {
	t.Setenv(deployenv.RailwayEnvVar, "staging")
	t.Setenv("APP_ENV", "prod")
	t.Setenv(envTestPrePassHost, "sftpgo.railway.internal")
	t.Setenv(envTestPrePassDir, "/prepass")

	p, dials := realPrePass(t)
	_, err := p.List(context.Background())

	require.NoError(t, err)
	require.Len(t, *dials, 1)
	assert.Equal(t, "sftpgo.railway.internal", (*dials)[0].Host)
	assert.Equal(t, "/prepass", p.directory)
}

// AC4: production reads the stored PrePass folder as before; TEST_* is ignored.
func TestDEV3306_AC4_ProductionReadsStoredHost(t *testing.T) {
	inProduction(t)
	t.Setenv(envTestPrePassHost, "sftpgo.railway.internal")
	t.Setenv(envTestPrePassDir, "/prepass")

	p, dials := realPrePass(t)
	_, err := p.List(context.Background())

	require.NoError(t, err)
	require.Len(t, *dials, 1)
	assert.Equal(t, realPrePassHost, (*dials)[0].Host)
	assert.Equal(t, "/statements", p.directory)
}

// Edge: a laptop / CI (no Railway name) and a renamed production never reach
// the real PrePass host.
func TestDEV3306_Edge_NoOrRenamedRailwayNameBlocksPrePass(t *testing.T) {
	for _, name := range []string{"", "Production", "prod"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(deployenv.RailwayEnvVar, name)
			t.Setenv(envTestPrePassHost, "")

			p, dials := realPrePass(t)
			_, err := p.List(context.Background())

			require.ErrorIs(t, err, ErrRealServerBlocked)
			assert.Empty(t, *dials)
		})
	}
}

// The host requirement on save is waived outside production (the catcher
// supplies it) and enforced in production — decided by Railway, not APP_ENV.
func TestDEV3306_ValidateHostRequirementFollowsRailway(t *testing.T) {
	noHost := Credential{ProviderType: ProviderPrePassSFTP, Username: "u", Secret: "p"}

	t.Setenv(deployenv.RailwayEnvVar, "staging")
	t.Setenv("APP_ENV", "prod")
	assert.NoError(t, noHost.Validate())

	inProduction(t)
	t.Setenv("APP_ENV", "")
	assert.ErrorContains(t, noHost.Validate(), "missing host")
}
