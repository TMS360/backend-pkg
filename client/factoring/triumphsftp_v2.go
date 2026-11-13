package factoring

import (
	"bytes"
	"crypto/rsa"
	"errors"
	"fmt"
	"net"
	"strconv"

	"github.com/TMS360/backend-pkg/deployenv"
	"golang.org/x/crypto/ssh"
)

// Triumph's new SFTP server, provider type triumph_sftp_v2. It sends the same
// files as triumph_sftp (same CSV, same PDF names, PDFs first and the CSV
// last); only the transport differs: another host and folder, key sign-in
// instead of a password, and a pinned host key.
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
	// SHA256:AoDQhntanGauYQKDxQ0Dnv3xTFrWhtv9PFlstUHdZu0, read from the server
	// on 2026-11-12. If the server's key changes, every v2 connection fails
	// with *HostKeyMismatchError until this constant is updated.
	triumphV2HostKey = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAACAQDCTX682aHwYBHQSFm23QK42OojjOA+/GZauUNZoEAUmkQbrmpEjFeEzjBCmaTXSPZXUzwN5ifW9ZqyY7KfP8+t0YU8euwccl+KoAD0FeMHN/UbI42YUZYn8yZDNu0g9Aq9Y4xVCITOc9e5iMSLkkscZP7qMMWnOfIX+eeBr7+unSPVMCXmC8fq9TZyFBdMEEpHK91Vt7aT3dB3GeFMjGkxL/kz71VKj7qNU/4mVsJnZujcrNvhnYsRgKMzhu+YjM+ut95KXI0t/O58vm5WaCsWqO7zqh1ITIWNAkhqTEG7BJ2/6uj//XYPOAXUevBPDtuY9qG9WzU9kJZrh9wPTdn2U5+JBnFre6UfBLHMvKBpO8P1Qu+VpGYhdDHLaJlCgjNvCH2sy6jk/rRiz8oAxAHiwRsng2YeaL2u3Nje3iQkaNYW8xcBJRZleVBOCeXNaP9O6BfkBgu7WXXnqWyhsjIR0x9ZNBjk2Fxkc3JcHtnOR434J5P1Iq2shJxiy826Fih/SvUH2VkQBP29GBn2VGX+Q50oeaGrnx9i6PTQ8ip3o3LFfX0GXERKU6xAvjpuV/D3uaKe4xtVxOsa+5RWL4qFrsACrbbThVxV/JnUeAkLSU2oFKWKyS6VOsi7WE1p2ewHIB6OETjURneuL3bBfeWDPsURlEoBylHALdMpI+bvqQ=="
)

var (
	// ErrPrivateKeyMissing means a key-sign-in credential has no private key.
	ErrPrivateKeyMissing = errors.New("factoring: private key is missing")
	// ErrPrivateKeyTooLarge means the pasted key is longer than maxPrivateKeyBytes
	// or its RSA modulus is longer than maxRSAKeyBits.
	ErrPrivateKeyTooLarge = errors.New("factoring: private key is too large")
	// ErrPrivateKeyInvalid means the stored private key cannot be used. The text
	// never carries any part of the key.
	ErrPrivateKeyInvalid = errors.New("factoring: private key is not a valid OpenSSH/PEM private key without a passphrase")
	// ErrHostKeyNotPinned guards the v2 provider: it never dials without a
	// host-key check, whatever a future constructor change does.
	ErrHostKeyNotPinned = errors.New("factoring: triumph_sftp_v2 has no host key check configured; refusing to dial")
)

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
		p.hostKey, p.hostKeyAlgorithms, p.blocked = pinnedHostKey(triumphV2HostKey)
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

// pinnedHostKey accepts only the RSA key in line (authorized_keys format) and
// asks the server for the SHA-2 RSA host key signatures only, so a server that
// also offers other key types still presents the one we pin.
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
	return cb, []string{ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256}, nil
}

// ValidatePrivateKey checks that pem is a usable private key (OpenSSH or PEM,
// no passphrase) without keeping it. The error never carries the key.
func ValidatePrivateKey(pem string) error {
	_, err := parsePrivateKey([]byte(pem))
	return err
}

// Size limits applied before a pasted key is parsed and after it is parsed.
const (
	maxPrivateKeyBytes = 8 * 1024
	maxRSAKeyBits      = 8192
)

func parsePrivateKey(pem []byte) (ssh.Signer, error) {
	if len(bytes.TrimSpace(pem)) == 0 {
		return nil, ErrPrivateKeyMissing
	}
	if len(pem) > maxPrivateKeyBytes {
		return nil, ErrPrivateKeyTooLarge
	}
	signer, err := ssh.ParsePrivateKey(pem)
	if err != nil {
		// err text is dropped on purpose: it must not reach a log or a message.
		return nil, ErrPrivateKeyInvalid
	}
	if ck, ok := signer.PublicKey().(ssh.CryptoPublicKey); ok {
		if rk, ok := ck.CryptoPublicKey().(*rsa.PublicKey); ok && rk.N.BitLen() > maxRSAKeyBits {
			return nil, ErrPrivateKeyTooLarge
		}
	}
	return signer, nil
}
