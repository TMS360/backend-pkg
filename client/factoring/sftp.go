package factoring

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// sftpClient is a thin abstraction over github.com/pkg/sftp scoped to what
// factoring providers need: dial, ensure a directory exists, write a file,
// close. Kept unexported because no caller outside this package needs raw
// SFTP — they go through Provider.
type sftpClient struct {
	ssh  *ssh.Client
	sftp *sftp.Client
}

// sftpDialer captures the connection parameters for dialSFTP. Provider
// implementations build one per SubmitBatch call — no connection pooling.
type sftpDialer struct {
	Host            string
	Port            int
	Username        string
	Password        string
	ProviderType    ProviderType        // reported on AuthError; defaults to ProviderTriumphSFTP if empty
	DialTimeout     time.Duration
	HostKeyCallback ssh.HostKeyCallback // nil → ssh.InsecureIgnoreHostKey()
}

func dialSFTP(ctx context.Context, d sftpDialer) (*sftpClient, error) {
	if d.Host == "" {
		return nil, errors.New("factoring/sftp: host is empty")
	}
	if d.Username == "" {
		return nil, errors.New("factoring/sftp: username is empty")
	}
	if d.Password == "" {
		return nil, errors.New("factoring/sftp: password is empty")
	}
	port := d.Port
	if port == 0 {
		port = 22
	}
	timeout := d.DialTimeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	hkcb := d.HostKeyCallback
	if hkcb == nil {
		// TODO: production deployments should pin the factor's host key. For
		// now we accept any key — most factoring SFTP endpoints rotate keys
		// quietly and there is no public fingerprint published.
		hkcb = ssh.InsecureIgnoreHostKey()
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
		return nil, fmt.Errorf("factoring/sftp: tcp dial %s: %w", addr, err)
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(tcpConn, addr, cfg)
	if err != nil {
		_ = tcpConn.Close()
		if isSSHAuthFailure(err) {
			pt := d.ProviderType
			if pt == "" {
				pt = ProviderTriumphSFTP
			}
			return nil, &AuthError{ProviderType: pt, Cause: err}
		}
		return nil, fmt.Errorf("factoring/sftp: ssh handshake %s: %w", addr, err)
	}
	sshClient := ssh.NewClient(sshConn, chans, reqs)

	sftpConn, err := sftp.NewClient(sshClient)
	if err != nil {
		_ = sshClient.Close()
		return nil, fmt.Errorf("factoring/sftp: open sftp subsystem: %w", err)
	}

	return &sftpClient{ssh: sshClient, sftp: sftpConn}, nil
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

// EnsureDir creates the remote directory and all parents if missing. No-op if
// it already exists. Factoring SFTP endpoints typically have a single inbound
// folder already provisioned per carrier, but we MkdirAll defensively.
func (c *sftpClient) EnsureDir(remoteDir string) error {
	if remoteDir == "" || remoteDir == "." || remoteDir == "/" {
		return nil
	}
	if err := c.sftp.MkdirAll(remoteDir); err != nil {
		return fmt.Errorf("factoring/sftp: mkdir %s: %w", remoteDir, err)
	}
	return nil
}

// Upload writes content to a file at remoteDir/filename. The remote file is
// created with truncation; any existing file at the same path is overwritten.
// Returns the full remote path that was written.
func (c *sftpClient) Upload(remoteDir, filename string, content []byte) (string, error) {
	remotePath := path.Join(remoteDir, filename)
	f, err := c.sftp.Create(remotePath)
	if err != nil {
		return "", fmt.Errorf("factoring/sftp: create %s: %w", remotePath, err)
	}
	if _, err := io.Copy(f, bytes.NewReader(content)); err != nil {
		_ = f.Close()
		return "", fmt.Errorf("factoring/sftp: write %s: %w", remotePath, err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("factoring/sftp: close %s: %w", remotePath, err)
	}
	return remotePath, nil
}

// Close releases the SFTP subsystem and the underlying SSH connection. Safe
// to call on a partially-constructed client (nil-safe).
func (c *sftpClient) Close() error {
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
