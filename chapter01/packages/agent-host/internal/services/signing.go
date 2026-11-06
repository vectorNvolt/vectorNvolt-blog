// SFR-HOST-003: pure-Go OpenPGP verification of SHA256SUMS against the pinned Ubuntu CD image signing key
// (no gpg/gpgv binary; create_agent_host.exe runs on Windows).
package services

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"strings"

	"github.com/ProtonMail/go-crypto/openpgp"

	"agent-host/internal/config"
)

//go:embed ubuntu-cdimage-signing-key.asc
var ubuntuCDSigningKey []byte

// LoadPinnedKeyring parses the embedded Ubuntu CD image signing key and fails closed
// unless it holds exactly one key whose primary fingerprint is the pinned one.
func LoadPinnedKeyring() (openpgp.EntityList, error) {
	kr, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(ubuntuCDSigningKey))
	if err != nil {
		return nil, fmt.Errorf("load embedded Ubuntu signing key: %w", err)
	}
	if len(kr) != 1 || entityFingerprint(kr[0]) != config.UbuntuCDSigningKeyFingerprint {
		return nil, errors.New("embedded Ubuntu signing key does not match the pinned fingerprint")
	}
	return kr, nil
}

func entityFingerprint(e *openpgp.Entity) string {
	return strings.ToUpper(fmt.Sprintf("%x", e.PrimaryKey.Fingerprint))
}

// VerifySHA256SUMS accepts sums only if sig is a good detached signature (armored or binary)
// by the pinned Ubuntu CD image signing key.
func VerifySHA256SUMS(sums, sig []byte) error {
	kr, err := LoadPinnedKeyring()
	if err != nil {
		return err
	}
	return VerifySHA256SUMSWith(kr, config.UbuntuCDSigningKeyFingerprint, sums, sig)
}

// VerifySHA256SUMSWith is VerifySHA256SUMS with an injectable keyring and expected signer fingerprint
// (uppercase hex); used by tests with a throwaway key.
func VerifySHA256SUMSWith(keyring openpgp.EntityList, fingerprint string, sums, sig []byte) error {
	var signer *openpgp.Entity
	var err error
	if bytes.HasPrefix(bytes.TrimSpace(sig), []byte("-----BEGIN PGP")) {
		signer, err = openpgp.CheckArmoredDetachedSignature(keyring, bytes.NewReader(sums), bytes.NewReader(sig), nil)
	} else {
		signer, err = openpgp.CheckDetachedSignature(keyring, bytes.NewReader(sums), bytes.NewReader(sig), nil)
	}
	if err != nil {
		return fmt.Errorf("SHA256SUMS signature check failed: %w", err)
	}
	if got := entityFingerprint(signer); got != strings.ToUpper(fingerprint) {
		return fmt.Errorf("SHA256SUMS signed by unexpected key %s", got)
	}
	return nil
}
