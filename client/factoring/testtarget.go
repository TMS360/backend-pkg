package factoring

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
)

// TestTarget is the TMS360 test server (SFTPGo) that a factoring test send
// writes to (DEV-3288). It is never a factor: the host comes only from the
// caller, the TEST_* overrides and APP_ENV are not read, and an empty host is
// an error — there is no fallback to the Triumph or RTS address.
type TestTarget struct {
	Host     string
	Port     int
	Username string
	Password string
	Dir      string // remote folder the files land in, e.g. /<companyID>/<stamp>
}

// NewTestTargetProvider returns the Triumph or RTS provider — same CSV layout
// and upload order as a real send — aimed at t instead of the factor.
func NewTestTargetProvider(layout ProviderType, t TestTarget) (Provider, error) {
	if strings.TrimSpace(t.Host) == "" {
		return nil, errors.New("factoring: test target host is empty")
	}
	if t.Username == "" || t.Password == "" {
		return nil, errors.New("factoring: test target username/password is empty")
	}
	switch layout {
	case ProviderTriumphSFTP:
		return &TriumphSFTPProvider{
			username: t.Username, password: t.Password,
			host: t.Host, port: t.Port, inboundDir: t.Dir,
			providerType: ProviderTriumphSFTP, dialFn: defaultSFTPDial,
		}, nil
	case ProviderRTSSFTP:
		return &RTSSFTPProvider{
			username: t.Username, password: t.Password,
			host: t.Host, port: t.Port, inboundDir: t.Dir,
			providerType: ProviderRTSSFTP, dialFn: defaultSFTPDial,
		}, nil
	default:
		return nil, fmt.Errorf("factoring: no test layout for provider %q", layout)
	}
}

// StatTestTarget dials t once and returns the size of every remote path that
// exists. A missing path is simply absent from the map; any other error
// aborts.
func StatTestTarget(ctx context.Context, t TestTarget, remotePaths []string) (map[string]int64, error) {
	c, err := dialSFTP(ctx, sftpDialer{Host: t.Host, Port: t.Port, Username: t.Username, Password: t.Password})
	if err != nil {
		return nil, err
	}
	defer c.Close()
	sizes := make(map[string]int64, len(remotePaths))
	for _, p := range remotePaths {
		fi, err := c.sftp.Stat(p)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return sizes, fmt.Errorf("factoring/sftp: stat %s: %w", p, err)
		}
		sizes[p] = fi.Size()
	}
	return sizes, nil
}
