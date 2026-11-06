// IP-ROOTFS-PROVIDER adapter: HTTPS download of the rootfs archive with SHA-256 verification.
package services

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"

	"agent-host/internal/config"
)

type RootfsProvider struct {
	Client    *http.Client
	UserAgent string
	Timeout   time.Duration // per attempt
	Attempts  int
	Backoff   time.Duration // base; attempt n waits Backoff * 2^(n-1)
	// Keyring overrides the pinned Ubuntu signing key; tests only. Empty means the embedded pinned key.
	Keyring openpgp.EntityList
}

// Fetch downloads archiveURL to a temp file, verifies it against sumsURL (whose
// detached signature sumsURL+".gpg" must be good) and returns the file path.
// The file is deleted on any failure.
func (p *RootfsProvider) Fetch(ctx context.Context, archiveURL, sumsURL string) (string, error) {
	sigURL := sumsURL + config.SHA256SumsSignatureSuffix
	for _, raw := range []string{archiveURL, sumsURL, sigURL} {
		u, err := url.Parse(raw)
		if err != nil {
			return "", fmt.Errorf("invalid URL %q: %w", raw, err)
		}
		if u.Scheme != "https" {
			return "", fmt.Errorf("refusing non-https URL %q", raw)
		}
	}
	au, _ := url.Parse(archiveURL)
	name := path.Base(au.Path)

	f, err := os.CreateTemp("", "agent-host-rootfs-*.tar.gz")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmp := f.Name()
	ok := false
	defer func() {
		f.Close()
		if !ok {
			os.Remove(tmp)
		}
	}()

	var got string
	err = p.get(ctx, archiveURL, func(r io.Reader) error {
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return err
		}
		if err := f.Truncate(0); err != nil {
			return err
		}
		h := sha256.New()
		if _, err := io.Copy(io.MultiWriter(f, h), r); err != nil {
			return err
		}
		got = hex.EncodeToString(h.Sum(nil))
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("download rootfs: %w", err)
	}

	var sums []byte
	err = p.get(ctx, sumsURL, func(r io.Reader) error {
		b, err := io.ReadAll(io.LimitReader(r, 1<<20))
		sums = b
		return err
	})
	if err != nil {
		return "", fmt.Errorf("download SHA256SUMS: %w", err)
	}
	var sig []byte
	err = p.get(ctx, sigURL, func(r io.Reader) error {
		b, err := io.ReadAll(io.LimitReader(r, 1<<20))
		sig = b
		return err
	})
	if err != nil {
		return "", fmt.Errorf("download SHA256SUMS.gpg: %w", err)
	}
	// Authenticate SHA256SUMS before trusting any of its content.
	if len(p.Keyring) > 0 {
		err = VerifySHA256SUMSWith(p.Keyring, entityFingerprint(p.Keyring[0]), sums, sig)
	} else {
		err = VerifySHA256SUMS(sums, sig)
	}
	if err != nil {
		return "", err
	}
	want, err := ParseSHA256Sums(sums, name)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(want, got) {
		return "", fmt.Errorf("SHA-256 mismatch for %s: expected %s, got %s", name, want, got)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("close temp file: %w", err)
	}
	ok = true
	return tmp, nil
}

func (p *RootfsProvider) get(ctx context.Context, rawURL string, handle func(io.Reader) error) error {
	attempts := p.Attempts
	if attempts < 1 {
		attempts = 1
	}
	var last error
	for i := 0; i < attempts; i++ {
		if i > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(p.Backoff << (i - 1)):
			}
		}
		retry, err := p.once(ctx, rawURL, handle)
		if err == nil {
			return nil
		}
		last = err
		if !retry {
			return err
		}
	}
	return fmt.Errorf("GET %s failed after %d attempts: %w", rawURL, attempts, last)
}

type insecureRedirectError struct{ target string }

func (e *insecureRedirectError) Error() string {
	return fmt.Sprintf("refusing redirect to non-https URL %q", e.target)
}

// httpsOnlyClient returns a shallow copy of the provided client whose redirect
// policy rejects any non-https hop (then defers to the client's own policy, or
// the default 10-hop limit).
func (p *RootfsProvider) httpsOnlyClient() *http.Client {
	c := &http.Client{}
	if p.Client != nil {
		cp := *p.Client
		c = &cp
	}
	prev := c.CheckRedirect
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" {
			return &insecureRedirectError{target: req.URL.String()}
		}
		if prev != nil {
			return prev(req, via)
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
	return c
}

// once performs one attempt; the bool tells whether a retry makes sense.
func (p *RootfsProvider) once(ctx context.Context, rawURL string, handle func(io.Reader) error) (bool, error) {
	actx := ctx
	if p.Timeout > 0 {
		var cancel context.CancelFunc
		actx, cancel = context.WithTimeout(ctx, p.Timeout)
		defer cancel()
	}
	req, err := http.NewRequestWithContext(actx, http.MethodGet, rawURL, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("User-Agent", p.UserAgent)
	resp, err := p.httpsOnlyClient().Do(req)
	if err != nil {
		var ir *insecureRedirectError
		if errors.As(err, &ir) {
			return false, err
		}
		return ctx.Err() == nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return true, fmt.Errorf("GET %s: HTTP %d", rawURL, resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("GET %s: HTTP %d", rawURL, resp.StatusCode)
	}
	if err := handle(resp.Body); err != nil {
		return ctx.Err() == nil, err
	}
	return false, nil
}

// ParseSHA256Sums returns the hash listed for name in a SHA256SUMS file
// (`hash  name` or `hash *name`).
func ParseSHA256Sums(data []byte, name string) (string, error) {
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		i := strings.IndexAny(line, " \t")
		if i != 64 {
			continue
		}
		rest := strings.TrimSpace(line[i:])
		rest = strings.TrimPrefix(strings.TrimPrefix(rest, "*"), "./")
		if rest == name {
			return line[:i], nil
		}
	}
	return "", fmt.Errorf("no SHA256SUMS entry for %s", name)
}
