package toll

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// maxRemoteFileSize caps a single download. A weekly toll file is a few
// hundred kilobytes; anything past this is a misconfigured folder pointing at
// something else, and streaming it into memory would take the service down.
const maxRemoteFileSize = 64 << 20 // 64 MiB

// sftpReader is the read half of an SFTP session — the mirror of factoring's
// upload-only client. Kept unexported: callers go through Provider.
type sftpReader struct {
	ssh  *ssh.Client
	sftp *sftp.Client
}

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

// dialSFTP opens an SSH connection and the SFTP subsystem on top of it.
func dialSFTP(ctx context.Context, d sftpDialer) (*sftpReader, error) {
	if d.Host == "" {
		return nil, errors.New("toll/sftp: host is empty")
	}
	if d.Username == "" {
		return nil, errors.New("toll/sftp: username is empty")
	}
	if d.Password == "" {
		return nil, errors.New("toll/sftp: password is empty")
	}
	port := d.Port
	if port == 0 {
		port = 22
	}
	timeout := d.DialTimeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	hkcb, err := hostKeyCallback(d.HostKey)
	if err != nil {
		return nil, err
	}

	cfg := &ssh.ClientConfig{
		User:            d.Username,
		Auth:            []ssh.AuthMethod{ssh.Password(d.Password)},
		HostKeyCallback: hkcb,
		Timeout:         timeout,
	}

	addr := net.JoinHostPort(d.Host, strconv.Itoa(port))
	dialer := &net.Dialer{Timeout: timeout}
	tcpConn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("toll/sftp: tcp dial %s: %w", addr, err)
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(tcpConn, addr, cfg)
	if err != nil {
		_ = tcpConn.Close()
		if isSSHAuthFailure(err) {
			return nil, &AuthError{ProviderType: d.ProviderType, Cause: err}
		}
		return nil, fmt.Errorf("toll/sftp: ssh handshake %s: %w", addr, err)
	}
	sshClient := ssh.NewClient(sshConn, chans, reqs)

	sftpConn, err := sftp.NewClient(sshClient)
	if err != nil {
		_ = sshClient.Close()
		return nil, fmt.Errorf("toll/sftp: open sftp subsystem: %w", err)
	}
	return &sftpReader{ssh: sshClient, sftp: sftpConn}, nil
}

// hostKeyCallback builds the verifier for the configured pin, or the
// accept-anything callback when no pin was supplied.
func hostKeyCallback(hostKey string) (ssh.HostKeyCallback, error) {
	hostKey = strings.TrimSpace(hostKey)
	if hostKey == "" {
		//nolint:gosec // documented opt-out; see sftpDialer.HostKey
		return ssh.InsecureIgnoreHostKey(), nil
	}
	pub, _, _, _, err := ssh.ParseAuthorizedKey([]byte(hostKey))
	if err != nil {
		return nil, fmt.Errorf("toll/sftp: parse host key: %w", err)
	}
	return ssh.FixedHostKey(pub), nil
}

func isSSHAuthFailure(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "unable to authenticate") ||
		strings.Contains(msg, "permission denied") ||
		strings.Contains(msg, "no supported methods remain")
}

// List enumerates regular files in remoteDir, newest first, keeping only the
// extensions the provider accepts. Directories and symlinks are skipped.
//
// An empty result is not an error: between weekly drops the folder is simply
// empty, and the caller treats that as "nothing to do".
func (c *sftpReader) List(remoteDir string, exts []string) ([]RemoteFile, error) {
	dir := remoteDir
	if dir == "" {
		dir = "."
	}
	entries, err := c.sftp.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("toll/sftp: list %s: %w", dir, err)
	}
	out := make([]RemoteFile, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !e.Mode().IsRegular() {
			continue
		}
		if !hasAcceptedExt(e.Name(), exts) {
			continue
		}
		out = append(out, RemoteFile{Name: e.Name(), Size: e.Size(), ModTime: e.ModTime()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ModTime.After(out[j].ModTime) })
	return out, nil
}

// Fetch downloads one file from remoteDir. The name is reduced to its base so
// a crafted listing entry cannot walk out of the configured directory.
func (c *sftpReader) Fetch(remoteDir, name string) ([]byte, error) {
	base := path.Base(strings.TrimSpace(name))
	if base == "" || base == "." || base == "/" || base == ".." {
		return nil, fmt.Errorf("toll/sftp: invalid file name %q", name)
	}
	remotePath := path.Join(remoteDir, base)

	f, err := c.sftp.Open(remotePath)
	if err != nil {
		return nil, fmt.Errorf("toll/sftp: open %s: %w", remotePath, err)
	}
	defer func() { _ = f.Close() }()

	// LimitReader is one byte over the cap so an oversized file is detected
	// rather than silently truncated into a half-parsed spreadsheet.
	data, err := io.ReadAll(io.LimitReader(f, maxRemoteFileSize+1))
	if err != nil {
		return nil, fmt.Errorf("toll/sftp: read %s: %w", remotePath, err)
	}
	if len(data) > maxRemoteFileSize {
		return nil, fmt.Errorf("toll/sftp: %s exceeds %d bytes", remotePath, maxRemoteFileSize)
	}
	return data, nil
}

// Close releases the SFTP subsystem and the underlying SSH connection.
// Nil-safe on a partially constructed client.
func (c *sftpReader) Close() error {
	var firstErr error
	if c.sftp != nil {
		if err := c.sftp.Close(); err != nil {
			firstErr = err
		}
	}
	if c.ssh != nil {
		if err := c.ssh.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func hasAcceptedExt(name string, exts []string) bool {
	if len(exts) == 0 {
		return true
	}
	lower := strings.ToLower(name)
	for _, e := range exts {
		if strings.HasSuffix(lower, e) {
			return true
		}
	}
	return false
}
