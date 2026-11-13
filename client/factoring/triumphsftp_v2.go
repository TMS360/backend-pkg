package factoring

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/TMS360/backend-pkg/deployenv"
	"golang.org/x/crypto/ssh"
)

// Triumph's new SFTP server (DEV-4015). Triumph is switching the old server
// (files.triumphbcap.com, provider triumph_sftp) off; the account moves to this
// one under the provider type triumph_sftp_v2. Same files as v1 — same CSV, same
// PDF names, PDFs first and the CSV last — only the transport differs: another
// host and folder, key sign-in instead of a password, and a pinned host key.
//
// Outside Railway production the real host is never dialled (DEV-3306): the
// provider dials only TEST_TRIUMPH_SFTP_V2_*, and with no test host set every
// dial fails with ErrRealServerBlocked.
const (
	triumphV2SFTPHost       = "prod.sftp.triumph.io"
	triumphV2SFTPPort       = 22
	triumphV2SFTPInboundDir = "/Factoring/TMS_INPUT/SUBMISSIONS"

	envTestTriumphV2SFTPHost       = "TEST_TRIUMPH_SFTP_V2_HOST"
	envTestTriumphV2SFTPPort       = "TEST_TRIUMPH_SFTP_V2_PORT"
	envTestTriumphV2SFTPInboundDir = "TEST_TRIUMPH_SFTP_V2_INBOUND_DIR"

	// triumphV2HostKey is the public RSA 4096 host key of prod.sftp.triumph.io,
	// SHA256:AoDQhntanGauYQKDxQ0Dnv3xTFrWhtv9PFlstUHdZu0. It was read from the
	// server on 2026-11-12 (ssh-keyscan); Triumph was asked to confirm it. If
	// Triumph rotates the key, every v2 connection fails with
	// *HostKeyMismatchError until this constant is updated.
	triumphV2HostKey = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAACAQDCTX682aHwYBHQSFm23QK42OojjOA+/GZauUNZoEAUmkQbrmpEjFeEzjBCmaTXSPZXUzwN5ifW9ZqyY7KfP8+t0YU8euwccl+KoAD0FeMHN/UbI42YUZYn8yZDNu0g9Aq9Y4xVCITOc9e5iMSLkkscZP7qMMWnOfIX+eeBr7+unSPVMCXmC8fq9TZyFBdMEEpHK91Vt7aT3dB3GeFMjGkxL/kz71VKj7qNU/4mVsJnZujcrNvhnYsRgKMzhu+YjM+ut95KXI0t/O58vm5WaCsWqO7zqh1ITIWNAkhqTEG7BJ2/6uj//XYPOAXUevBPDtuY9qG9WzU9kJZrh9wPTdn2U5+JBnFre6UfBLHMvKBpO8P1Qu+VpGYhdDHLaJlCgjNvCH2sy6jk/rRiz8oAxAHiwRsng2YeaL2u3Nje3iQkaNYW8xcBJRZleVBOCeXNaP9O6BfkBgu7WXXnqWyhsjIR0x9ZNBjk2Fxkc3JcHtnOR434J5P1Iq2shJxiy826Fih/SvUH2VkQBP29GBn2VGX+Q50oeaGrnx9i6PTQ8ip3o3LFfX0GXERKU6xAvjpuV/D3uaKe4xtVxOsa+5RWL4qFrsACrbbThVxV/JnUeAkLSU2oFKWKyS6VOsi7WE1p2ewHIB6OETjURneuL3bBfeWDPsURlEoBylHALdMpI+bvqQ=="
)

var (
	// ErrPrivateKeyMissing means a key-sign-in credential has no private key.
	ErrPrivateKeyMissing = errors.New("factoring: private key is missing")
	// ErrPrivateKeyInvalid means the stored private key cannot be used. The text
	// never carries any part of the key.
	ErrPrivateKeyInvalid = errors.New("factoring: private key is not a valid OpenSSH/PEM private key without a passphrase")
	// ErrHostKeyNotPinned guards the v2 provider: it never dials without a
	// host-key check. Reaching it is a programming error, not a server problem.
	ErrHostKeyNotPinned = errors.New("factoring: triumph_sftp_v2 has no host key check configured; refusing to dial")
)

// HostKeyMismatchError is returned when the server presents a host key other
// than the pinned one: either Triumph changed the key or the connection is
// being intercepted. Check with errors.As or IsHostKeyMismatch.
type HostKeyMismatchError struct {
	Host string
	Got  string // SHA256 fingerprint the server presented
	Want string // SHA256 fingerprint we pinned
}

func (e *HostKeyMismatchError) Error() string {
	return fmt.Sprintf("factoring/sftp: host key of %s does not match the pinned key (server %s, pinned %s); connection refused",
		e.Host, e.Got, e.Want)
}

// IsHostKeyMismatch reports whether err is (or wraps) a *HostKeyMismatchError.
func IsHostKeyMismatch(err error) bool {
	var hk *HostKeyMismatchError
	return errors.As(err, &hk)
}

// DialError is a failed TCP connect (network down, blocked, refused, timed
// out). Its text is identical to the plain wrapped error the dialer returned
// before, so existing messages do not change. Check with IsDialError.
type DialError struct {
	Addr  string
	Cause error
}

func (e *DialError) Error() string {
	return fmt.Sprintf("factoring/sftp: tcp dial %s: %v", e.Addr, e.Cause)
}
func (e *DialError) Unwrap() error { return e.Cause }

// IsDialError reports whether err is (or wraps) a *DialError.
func IsDialError(err error) bool {
	var de *DialError
	return errors.As(err, &de)
}

// NewTriumphSFTPV2 builds the provider for Triumph's new SFTP server. It reads
// Username and PrivateKey from the credential (the password is ignored).
// Reuse is safe; the connection is opened and closed inside each call.
func NewTriumphSFTPV2(cred Credential) *TriumphSFTPProvider {
	p := &TriumphSFTPProvider{
		username:     cred.Username,
		host:         triumphV2SFTPHost,
		port:         triumphV2SFTPPort,
		inboundDir:   triumphV2SFTPInboundDir,
		providerType: ProviderTriumphSFTPV2,
		dialFn:       defaultSFTPDial,
		privateKey:   []byte(cred.PrivateKey),
	}
	if deployenv.IsProduction() {
		// The only place the real host is dialled, and the only place the pin
		// is applied: production never reaches the test-target branch below.
		p.hostKey, p.hostKeyAlgorithms, p.blocked = pinnedTriumphV2HostKey()
		return p
	}

	p.host, p.blocked = testHostOrBlock(triumphV2SFTPHost, envTestTriumphV2SFTPHost)
	if raw := firstNonEmptyEnv(envTestTriumphV2SFTPPort); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			p.port = n
		}
	}
	if d := firstNonEmptyEnv(envTestTriumphV2SFTPInboundDir); d != "" {
		p.inboundDir = d
	}
	if p.blocked == nil {
		// TEST TARGET ONLY. The stand-in server (SFTPGo) has its own host key,
		// not Triumph's, so the pin cannot apply. This branch needs a test host
		// from TEST_TRIUMPH_SFTP_V2_HOST and is unreachable when
		// deployenv.IsProduction() is true (that case returned above).
		p.hostKey = ssh.InsecureIgnoreHostKey() // #nosec G106 -- non-production test target, see above
	}
	return p
}

// pinnedTriumphV2HostKey returns the host key check for the real server.
func pinnedTriumphV2HostKey() (ssh.HostKeyCallback, []string, error) {
	return pinnedHostKey(triumphV2HostKey)
}

// pinnedHostKey accepts only the RSA key in line (authorized_keys format), and
// asks the server for RSA host keys only, so a server that also offers other
// key types still presents the one we pin.
func pinnedHostKey(line string) (ssh.HostKeyCallback, []string, error) {
	want, _, _, _, err := ssh.ParseAuthorizedKey([]byte(line))
	if err != nil {
		return nil, nil, fmt.Errorf("factoring: pinned triumph host key is unreadable: %w", err)
	}
	wantBytes, wantPrint := want.Marshal(), ssh.FingerprintSHA256(want)
	cb := func(hostname string, _ net.Addr, got ssh.PublicKey) error {
		if bytes.Equal(got.Marshal(), wantBytes) {
			return nil
		}
		return &HostKeyMismatchError{Host: hostname, Got: ssh.FingerprintSHA256(got), Want: wantPrint}
	}
	return cb, []string{ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256, ssh.KeyAlgoRSA}, nil
}

// ValidatePrivateKey checks that pem is a usable private key (OpenSSH or PEM,
// no passphrase) without keeping it. The error never carries the key.
func ValidatePrivateKey(pem string) error {
	_, err := parsePrivateKey([]byte(pem))
	return err
}

func parsePrivateKey(pem []byte) (ssh.Signer, error) {
	if len(bytes.TrimSpace(pem)) == 0 {
		return nil, ErrPrivateKeyMissing
	}
	signer, err := ssh.ParsePrivateKey(pem)
	if err != nil {
		// err text is dropped on purpose: it must not reach a log or a message.
		return nil, ErrPrivateKeyInvalid
	}
	return signer, nil
}

// preflight runs the checks that must pass before any dial. v1 has only the
// DEV-3306 block; v2 also needs a private key and a host key check.
func (p *TriumphSFTPProvider) preflight() error {
	if p.blocked != nil {
		return p.blocked
	}
	if p.providerType == ProviderTriumphSFTPV2 {
		if strings.TrimSpace(string(p.privateKey)) == "" {
			return ErrPrivateKeyMissing
		}
		if p.hostKey == nil {
			return ErrHostKeyNotPinned
		}
	}
	return nil
}

// dialer is the connection description for this provider, shared by
// TestConnection and SubmitBatch.
func (p *TriumphSFTPProvider) dialer() sftpDialer {
	return sftpDialer{
		Host:              p.host,
		Port:              p.port,
		Username:          p.username,
		Password:          p.password,
		PrivateKey:        p.privateKey,
		ProviderType:      p.providerType,
		HostKeyCallback:   p.hostKey,
		HostKeyAlgorithms: p.hostKeyAlgorithms,
	}
}
