package factoring

import (
	"testing"

	"github.com/TMS360/backend-pkg/deployenv"
	"github.com/stretchr/testify/require"
)

func TestNewTestTargetProvider_EmptyHostRefused(t *testing.T) {
	_, err := NewTestTargetProvider(ProviderTriumphSFTP, TestTarget{Username: "u", Password: "p"})
	require.Error(t, err)
}

func TestNewTestTargetProvider_IgnoresTestEnvOverrides(t *testing.T) {
	t.Setenv(deployenv.RailwayEnvVar, "staging")
	t.Setenv(envTestTriumphSFTPHost, "other-host")
	t.Setenv(envTestSFTPHostLegacy, "other-host")
	t.Setenv(envTestRTSSFTPHost, "other-host")
	target := TestTarget{Host: "sftpgo-test.railway.internal", Port: 2022, Username: "u", Password: "p", Dir: "/c/1"}

	p, err := NewTestTargetProvider(ProviderTriumphSFTP, target)
	require.NoError(t, err)
	tp := p.(*TriumphSFTPProvider)
	require.Equal(t, target.Host, tp.host)
	require.Equal(t, 2022, tp.port)
	require.Equal(t, "/c/1", tp.inboundDir)

	p, err = NewTestTargetProvider(ProviderRTSSFTP, target)
	require.NoError(t, err)
	rp := p.(*RTSSFTPProvider)
	require.Equal(t, target.Host, rp.host)
	require.Equal(t, "/c/1", rp.inboundDir)
}

func TestNewTestTargetProvider_UnknownLayout(t *testing.T) {
	_, err := NewTestTargetProvider(ProviderRTSTestSFTP, TestTarget{Host: "h", Username: "u", Password: "p"})
	require.Error(t, err)
}
