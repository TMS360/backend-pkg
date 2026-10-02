// Package sftpfile is the shared SFTP read client: dial with password auth
// and a host-key policy, list a folder, stat and download a file with a size
// cap. Used by the PrePass toll provider (client/toll) and by backend-fuel
// for the Pilot Flying J price file (DEV-2692).
package sftpfile

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// AuthError: the server refused the login (as opposed to being unreachable).
type AuthError struct{ Cause error }

func (e *AuthError) Error() string { return "sftp: authentication failed: " + e.Cause.Error() }
func (e *AuthError) Unwrap() error { return e.Cause }

// ErrNotFound: a requested file does not exist.
var ErrNotFound = errors.New("sftp: file not found")

// Dialer holds the connection parameters. One is built per call; there is no
// pooling.
type Dialer struct {
	Host        string
	Port        int // 22 when zero
	Username    string
	Password    string
	DialTimeout time.Duration // 30 s when zero

	// Host-key policy, first match wins:
	//   - HostKey: the server must present exactly this key;
	//   - AuthorizedKey: the same, given in authorized_keys form ("ssh-ed25519 AAAA...");
	//   - AllowAnyHostKey: accept any key (an explicit opt-out, e.g. toll
	//     aggregators that rotate keys without notice).
	// With none set, Dial refuses: a missing pin is never silent.
	HostKey         ssh.PublicKey
	AuthorizedKey   string
	AllowAnyHostKey bool
}

// File is one remote regular file.
type File struct {
	Name    string
	Size    int64
	ModTime time.Time
}

// Reader is an open read session.
type Reader struct {
	ssh  *ssh.Client
	sftp *sftp.Client
}

// Dial opens the SSH connection and the SFTP subsystem.
func Dial(ctx context.Context, d Dialer) (*Reader, error) {
	switch {
	case d.Host == "":
		return nil, errors.New("sftp: host is empty")
	case d.Username == "":
		return nil, errors.New("sftp: username is empty")
	case d.Password == "":
		return nil, errors.New("sftp: password is empty")
	}
	port := d.Port
	if port == 0 {
		port = 22
	}
	timeout := d.DialTimeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	hkcb, err := d.HostKeyCallback()
	if err != nil {
		return nil, err
	}
	addr := net.JoinHostPort(d.Host, strconv.Itoa(port))
	tcp, err := (&net.Dialer{Timeout: timeout}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("sftp: tcp dial %s: %w", addr, err)
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = tcp.SetDeadline(dl)
	}
	cfg := &ssh.ClientConfig{User: d.Username, Auth: []ssh.AuthMethod{ssh.Password(d.Password)}, HostKeyCallback: hkcb, Timeout: timeout}
	conn, chans, reqs, err := ssh.NewClientConn(tcp, addr, cfg)
	if err != nil {
		_ = tcp.Close()
		if isAuthFailure(err) {
			return nil, &AuthError{Cause: err}
		}
		return nil, fmt.Errorf("sftp: ssh handshake %s: %w", addr, err)
	}
	client := ssh.NewClient(conn, chans, reqs)
	sc, err := sftp.NewClient(client)
	if err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("sftp: open subsystem: %w", err)
	}
	return &Reader{ssh: client, sftp: sc}, nil
}

// HostKeyCallback is the verifier for the configured policy.
func (d Dialer) HostKeyCallback() (ssh.HostKeyCallback, error) {
	if d.HostKey != nil {
		return ssh.FixedHostKey(d.HostKey), nil
	}
	if k := strings.TrimSpace(d.AuthorizedKey); k != "" {
		pub, _, _, _, err := ssh.ParseAuthorizedKey([]byte(k))
		if err != nil {
			return nil, fmt.Errorf("sftp: parse host key: %w", err)
		}
		return ssh.FixedHostKey(pub), nil
	}
	if d.AllowAnyHostKey {
		//nolint:gosec // explicit opt-out by the caller (Dialer.AllowAnyHostKey)
		return ssh.InsecureIgnoreHostKey(), nil
	}
	return nil, errors.New("sftp: no host key policy (pin a key or set AllowAnyHostKey)")
}

func isAuthFailure(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "unable to authenticate") ||
		strings.Contains(msg, "permission denied") ||
		strings.Contains(msg, "no supported methods remain")
}

// List returns the regular files of dir with one of exts (all when empty),
// newest first. An empty folder is not an error.
func (r *Reader) List(dir string, exts []string) ([]File, error) {
	if dir == "" {
		dir = "."
	}
	entries, err := r.sftp.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("sftp: list %s: %w", dir, err)
	}
	out := make([]File, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !e.Mode().IsRegular() || !hasExt(e.Name(), exts) {
			continue
		}
		out = append(out, File{Name: e.Name(), Size: e.Size(), ModTime: e.ModTime()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ModTime.After(out[j].ModTime) })
	return out, nil
}

// Fetch downloads dir/name, refusing files over maxBytes (checked by stat
// before reading and again on the bounded read). The name is reduced to its
// base, so a crafted listing entry cannot walk out of dir.
func (r *Reader) Fetch(dir, name string, maxBytes int64) ([]byte, error) {
	base := path.Base(strings.TrimSpace(name))
	if base == "" || base == "." || base == "/" || base == ".." {
		return nil, fmt.Errorf("sftp: invalid file name %q", name)
	}
	p := path.Join(dir, base)
	st, err := r.sftp.Stat(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("sftp: stat %s: %w", p, err)
	}
	if st.Size() > maxBytes {
		return nil, fmt.Errorf("sftp: %s exceeds %d bytes", p, maxBytes)
	}
	f, err := r.sftp.Open(p)
	if err != nil {
		return nil, fmt.Errorf("sftp: open %s: %w", p, err)
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("sftp: read %s: %w", p, err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("sftp: %s exceeds %d bytes", p, maxBytes)
	}
	return data, nil
}

// Close releases the SFTP subsystem and the SSH connection; nil-safe.
func (r *Reader) Close() error {
	var first error
	if r.sftp != nil {
		first = r.sftp.Close()
	}
	if r.ssh != nil {
		if err := r.ssh.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func hasExt(name string, exts []string) bool {
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
