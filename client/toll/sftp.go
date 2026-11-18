package toll

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/TMS360/backend-pkg/client/sftpfile"
	"golang.org/x/crypto/ssh"
)

// maxRemoteFileSize caps a single download. A weekly toll file is a few
// hundred kilobytes; anything past this is a misconfigured folder pointing at
// something else, and streaming it into memory would take the service down.
const maxRemoteFileSize = 64 << 20 // 64 MiB

// sftpDialer captures connection parameters. One is built per call; there is
// no connection pooling, matching the factoring client's approach.
type sftpDialer struct {
	Host         string
	Port         int
	Username     string
	Password     string
	ProviderType ProviderType
	DialTimeout  time.Duration
	// HostKey is the expected server key in authorized_keys form
	// ("ssh-ed25519 AAAA..."). When set, the handshake fails unless the server
	// presents exactly this key.
	//
	// When empty, any host key is accepted. That is a deliberate opt-out
	// rather than an oversight: toll aggregators do not publish fingerprints
	// and rotate keys without notice, so requiring a pin would make the
	// integration unusable. Unlike the factoring client, the pin is at least
	// available to operators who can obtain the key.
	HostKey string
}

// sftpReader is the toll view of the shared SFTP read client
// (client/sftpfile). Kept unexported: callers go through Provider.
type sftpReader struct{ r *sftpfile.Reader }

// dialSFTP opens the session; a refused login comes back as a toll AuthError.
func dialSFTP(ctx context.Context, d sftpDialer) (*sftpReader, error) {
	r, err := sftpfile.Dial(ctx, sftpfile.Dialer{
		Host: d.Host, Port: d.Port, Username: d.Username, Password: d.Password, DialTimeout: d.DialTimeout,
		AuthorizedKey: d.HostKey, AllowAnyHostKey: d.HostKey == "",
	})
	var auth *sftpfile.AuthError
	if errors.As(err, &auth) {
		return nil, &AuthError{ProviderType: d.ProviderType, Cause: auth.Cause}
	}
	if err != nil {
		return nil, fmt.Errorf("toll/%w", err)
	}
	return &sftpReader{r: r}, nil
}

// List enumerates regular files in remoteDir, newest first, keeping only the
// extensions the provider accepts. An empty result is not an error: between
// weekly drops the folder is simply empty.
func (c *sftpReader) List(remoteDir string, exts []string) ([]RemoteFile, error) {
	files, err := c.r.List(remoteDir, exts)
	if err != nil {
		return nil, fmt.Errorf("toll/%w", err)
	}
	out := make([]RemoteFile, 0, len(files))
	for _, f := range files {
		out = append(out, RemoteFile{Name: f.Name, Size: f.Size, ModTime: f.ModTime})
	}
	return out, nil
}

// Fetch downloads one file from remoteDir (base name only, size-capped).
func (c *sftpReader) Fetch(remoteDir, name string) ([]byte, error) {
	data, err := c.r.Fetch(remoteDir, name, maxRemoteFileSize)
	if err != nil {
		return nil, fmt.Errorf("toll/%w", err)
	}
	return data, nil
}

// Close releases the session. Nil-safe.
func (c *sftpReader) Close() error {
	if c == nil || c.r == nil {
		return nil
	}
	return c.r.Close()
}

// hostKeyCallback is the toll pin policy: the given authorized_keys line, or
// any key when empty (see sftpDialer.HostKey).
func hostKeyCallback(hostKey string) (ssh.HostKeyCallback, error) {
	return sftpfile.Dialer{AuthorizedKey: hostKey, AllowAnyHostKey: hostKey == ""}.HostKeyCallback()
}
