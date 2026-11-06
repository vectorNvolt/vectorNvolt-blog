package services

import (
	"bytes"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp"

	"agent-host/internal/config"
)

func newTestKey(t *testing.T) *openpgp.Entity {
	t.Helper()
	e, err := openpgp.NewEntity("t", "", "t@example", nil)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func signArmored(t *testing.T, e *openpgp.Entity, msg []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := openpgp.ArmoredDetachSign(&buf, e, bytes.NewReader(msg), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestVerifySHA256SUMSGoodSignature(t *testing.T) {
	key := newTestKey(t)
	sums := []byte(bytesOf64("a") + "  a.tar.gz\n")
	sig := signArmored(t, key, sums)
	if err := VerifySHA256SUMSWith(openpgp.EntityList{key}, entityFingerprint(key), sums, sig); err != nil {
		t.Fatalf("good signature rejected: %v", err)
	}
}

func TestVerifySHA256SUMSBinarySignature(t *testing.T) {
	key := newTestKey(t)
	sums := []byte("data\n")
	var buf bytes.Buffer
	if err := openpgp.DetachSign(&buf, key, bytes.NewReader(sums), nil); err != nil {
		t.Fatal(err)
	}
	if err := VerifySHA256SUMSWith(openpgp.EntityList{key}, entityFingerprint(key), sums, buf.Bytes()); err != nil {
		t.Fatalf("binary signature rejected: %v", err)
	}
}

func TestVerifySHA256SUMSTamperedFails(t *testing.T) {
	key := newTestKey(t)
	sums := []byte("original\n")
	sig := signArmored(t, key, sums)
	if err := VerifySHA256SUMSWith(openpgp.EntityList{key}, entityFingerprint(key), []byte("tampered\n"), sig); err == nil {
		t.Fatal("tampered SHA256SUMS accepted")
	}
}

func TestVerifySHA256SUMSWrongSignerFails(t *testing.T) {
	key, other := newTestKey(t), newTestKey(t)
	sums := []byte("data\n")
	sig := signArmored(t, other, sums)
	if err := VerifySHA256SUMSWith(openpgp.EntityList{key}, entityFingerprint(key), sums, sig); err == nil {
		t.Fatal("signature by a different key accepted")
	}
}

func TestVerifySHA256SUMSFingerprintMismatchFails(t *testing.T) {
	key := newTestKey(t)
	sums := []byte("data\n")
	sig := signArmored(t, key, sums)
	if err := VerifySHA256SUMSWith(openpgp.EntityList{key}, config.UbuntuCDSigningKeyFingerprint, sums, sig); err == nil {
		t.Fatal("signer with unpinned fingerprint accepted")
	}
}

func TestVerifySHA256SUMSGarbageSignatureFails(t *testing.T) {
	if err := VerifySHA256SUMS([]byte("data\n"), []byte("not a signature")); err == nil {
		t.Fatal("garbage signature accepted")
	}
}

func TestEmbeddedKeyMatchesPinnedFingerprint(t *testing.T) {
	kr, err := LoadPinnedKeyring()
	if err != nil {
		t.Fatal(err)
	}
	if got := entityFingerprint(kr[0]); got != config.UbuntuCDSigningKeyFingerprint {
		t.Fatalf("fingerprint %s, want %s", got, config.UbuntuCDSigningKeyFingerprint)
	}
}

func bytesOf64(s string) string { return string(bytes.Repeat([]byte(s), 64)) }
