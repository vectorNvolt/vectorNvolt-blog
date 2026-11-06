package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"
)

func newProvider(c *http.Client) *RootfsProvider {
	return &RootfsProvider{Client: c, UserAgent: "test", Timeout: 5 * time.Second, Attempts: 3, Backoff: time.Millisecond}
}

func assertNoTempFiles(t *testing.T, dir string) {
	t.Helper()
	ents, _ := os.ReadDir(dir)
	if len(ents) != 0 {
		t.Fatalf("temp files left behind: %v", ents)
	}
}

func TestFetchRejectsHTTPSToHTTPRedirect(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	var plainHits atomic.Int32
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		plainHits.Add(1)
		w.Write([]byte("data"))
	}))
	defer plain.Close()
	var tlsHits atomic.Int32
	tlsSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tlsHits.Add(1)
		http.Redirect(w, r, plain.URL+"/rootfs.tar.gz", http.StatusFound)
	}))
	defer tlsSrv.Close()

	_, err := newProvider(tlsSrv.Client()).Fetch(context.Background(), tlsSrv.URL+"/rootfs.tar.gz", tlsSrv.URL+"/SHA256SUMS")
	if err == nil || !strings.Contains(err.Error(), "non-https") {
		t.Fatalf("expected non-https redirect error, got %v", err)
	}
	if plainHits.Load() != 0 {
		t.Fatal("plain http server must not be contacted")
	}
	if tlsHits.Load() != 1 {
		t.Fatalf("redirect rejection must not be retried, hits=%d", tlsHits.Load())
	}
	assertNoTempFiles(t, tmp)
}

func TestFetch4xxNotRetried(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	var hits atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.NotFound(w, r)
	}))
	defer srv.Close()
	_, err := newProvider(srv.Client()).Fetch(context.Background(), srv.URL+"/a.tar.gz", srv.URL+"/SHA256SUMS")
	if err == nil || hits.Load() != 1 {
		t.Fatalf("expected single failed attempt, hits=%d err=%v", hits.Load(), err)
	}
	assertNoTempFiles(t, tmp)
}

type failingTransport struct{ calls atomic.Int32 }

func (f *failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	f.calls.Add(1)
	return nil, errors.New("connection reset")
}

func TestFetchTransportErrorRetried(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	ft := &failingTransport{}
	_, err := newProvider(&http.Client{Transport: ft}).Fetch(context.Background(), "https://example.invalid/a.tar.gz", "https://example.invalid/SHA256SUMS")
	if err == nil || ft.calls.Load() != 3 {
		t.Fatalf("expected 3 attempts, calls=%d err=%v", ft.calls.Load(), err)
	}
	assertNoTempFiles(t, tmp)
}

func TestFetchChecksumMismatchRemovesTempFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	key := newTestKey(t)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "SHA256SUMS") {
			w.Write([]byte(strings.Repeat("0", 64) + "  a.tar.gz\n"))
			return
		}
		if strings.HasSuffix(r.URL.Path, "SHA256SUMS.gpg") {
			w.Write(signArmored(t, key, []byte(strings.Repeat("0", 64)+"  a.tar.gz\n")))
			return
		}
		w.Write([]byte("payload"))
	}))
	defer srv.Close()
	p := newProvider(srv.Client())
	p.Keyring = openpgp.EntityList{key}
	_, err := p.Fetch(context.Background(), srv.URL+"/a.tar.gz", srv.URL+"/SHA256SUMS")
	if err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("expected mismatch error, got %v", err)
	}
	assertNoTempFiles(t, tmp)
}

func TestFetchBadSignatureRejectedBeforeHashCheck(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	key, other := newTestKey(t), newTestKey(t)
	body := []byte("payload")
	h := sha256.Sum256(body)
	sums := []byte(hex.EncodeToString(h[:]) + "  a.tar.gz\n") // hash would match
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "SHA256SUMS.gpg"):
			w.Write(signArmored(t, other, sums)) // signed by a key outside the keyring
		case strings.HasSuffix(r.URL.Path, "SHA256SUMS"):
			w.Write(sums)
		default:
			w.Write(body)
		}
	}))
	defer srv.Close()
	p := newProvider(srv.Client())
	p.Keyring = openpgp.EntityList{key}
	_, err := p.Fetch(context.Background(), srv.URL+"/a.tar.gz", srv.URL+"/SHA256SUMS")
	if err == nil || !strings.Contains(err.Error(), "signature") {
		t.Fatalf("expected signature error, got %v", err)
	}
	assertNoTempFiles(t, tmp)
}

func TestFetchMissingSignatureFails(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".gpg") {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte("x"))
	}))
	defer srv.Close()
	_, err := newProvider(srv.Client()).Fetch(context.Background(), srv.URL+"/a.tar.gz", srv.URL+"/SHA256SUMS")
	if err == nil || !strings.Contains(err.Error(), "SHA256SUMS.gpg") {
		t.Fatalf("expected signature download error, got %v", err)
	}
	assertNoTempFiles(t, tmp)
}

func TestParseSHA256Sums(t *testing.T) {
	h1 := strings.Repeat("a", 64)
	h2 := strings.Repeat("B", 64)
	h3 := strings.Repeat("c", 64)
	data := []byte(h1 + "  other.tar.gz\n" + h2 + " *target.tar.gz\n" + h3 + "  ./third.tar.gz\n")

	if got, err := ParseSHA256Sums(data, "target.tar.gz"); err != nil || got != h2 {
		t.Fatalf("binary marker: got %q, %v", got, err)
	}
	if got, err := ParseSHA256Sums(data, "other.tar.gz"); err != nil || got != h1 {
		t.Fatalf("text marker: got %q, %v", got, err)
	}
	if got, err := ParseSHA256Sums(data, "third.tar.gz"); err != nil || got != h3 {
		t.Fatalf("dot-slash: got %q, %v", got, err)
	}
	if _, err := ParseSHA256Sums(data, "missing.tar.gz"); err == nil {
		t.Fatal("expected error for missing entry")
	}
}
