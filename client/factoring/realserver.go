package factoring

import (
	"errors"
	"fmt"

	"github.com/TMS360/backend-pkg/deployenv"
)

// ErrRealServerBlocked is returned by TestConnection / SubmitBatch when a
// provider runs outside Railway production and no TEST_*_SFTP_HOST points it
// at our catcher. Staging databases are restored from production, so the
// stored credentials are real: falling back to the factor's address would
// sell real invoices from a test box (DEV-3306). Check with errors.Is.
var ErrRealServerBlocked = errors.New("real factoring server is blocked outside production")

// testHostOrBlock picks the host a provider dials outside production: the
// first non-empty of testEnvNames, or "" plus ErrRealServerBlocked naming
// the missing variable and the refused realHost. It never returns realHost.
func testHostOrBlock(realHost string, testEnvNames ...string) (string, error) {
	if h := firstNonEmptyEnv(testEnvNames...); h != "" {
		return h, nil
	}
	return "", fmt.Errorf("%w: %s is unset (%s=%q), refusing to dial %s",
		ErrRealServerBlocked, testEnvNames[0], deployenv.RailwayEnvVar, deployenv.Name(), realHost)
}
