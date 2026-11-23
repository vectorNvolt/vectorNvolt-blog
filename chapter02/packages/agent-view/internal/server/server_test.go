package server

import (
	"bufio"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"agent-view/internal/wsread"
)

func write(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func setup(t *testing.T, static fs.FS) (string, http.Handler) {
	t.Helper()
	ws := t.TempDir()
	r, err := wsread.New(ws)
	if err != nil {
		t.Fatal(err)
	}
	return ws, New(Config{WS: r, Static: static, PollInterval: 20 * time.Millisecond, PingInterval: time.Hour})
}

func get(h http.Handler, method, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	req.Host = "127.0.0.1:8787"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

var okStatic = fstest.MapFS{
	"index.html": {Data: []byte("<html>ui</html>")},
	"app.js":     {Data: []byte("console.log(1)")},
	".gitkeep":   {Data: nil},
}

func TestListOrderAndNull(t *testing.T) {
	ws, h := setup(t, okStatic)
	base := filepath.Join(ws, ".output/exec")
	write(t, filepath.Join(base, "old/result.json"), `{"exit":0}`)
	write(t, filepath.Join(base, "old/in/netlist"), "x")
	write(t, filepath.Join(base, "old/out/sim.raw"), "x")
	write(t, filepath.Join(base, "new/result.json"), `{broken`)
	t0 := time.Now().Add(-time.Hour)
	if err := os.Chtimes(filepath.Join(base, "old"), t0, t0); err != nil {
		t.Fatal(err)
	}

	rec := get(h, "GET", "/api/execs")
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("status %d ct %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	var body struct {
		Execs []struct {
			ID      string          `json:"id"`
			MTime   string          `json:"mtime"`
			Result  json.RawMessage `json:"result"`
			Meta    json.RawMessage `json:"meta"`
			Inputs  []string        `json:"inputs"`
			Outputs []string        `json:"outputs"`
		} `json:"execs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Execs) != 2 || body.Execs[0].ID != "new" || body.Execs[1].ID != "old" {
		t.Fatalf("execs: %s", rec.Body.String())
	}
	if string(body.Execs[0].Result) != "null" || string(body.Execs[0].Meta) != "null" {
		t.Errorf("malformed json should be null: %s", rec.Body.String())
	}
	if string(body.Execs[1].Result) != `{"exit":0}` {
		t.Errorf("result: %s", body.Execs[1].Result)
	}
	if strings.Join(body.Execs[1].Inputs, ",") != "netlist" || strings.Join(body.Execs[1].Outputs, ",") != "sim.raw" {
		t.Errorf("inputs/outputs: %v %v", body.Execs[1].Inputs, body.Execs[1].Outputs)
	}
}

func TestListMissingOutputDir(t *testing.T) {
	_, h := setup(t, okStatic)
	rec := get(h, "GET", "/api/execs")
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `{"execs":[]}` {
		t.Fatalf("status %d body %q", rec.Code, rec.Body.String())
	}
}

func TestFileContentTypes(t *testing.T) {
	ws, h := setup(t, okStatic)
	dir := filepath.Join(ws, ".output/exec/rcd1")
	for _, p := range []string{"out/sim.raw", "out/sim.log", "result.json", "out/a.cir", "out/n.txt", "in/netlist", "in/data.bin"} {
		write(t, filepath.Join(dir, p), "data-"+p)
	}
	cases := map[string]string{
		"out/sim.raw": "application/octet-stream",
		"out/sim.log": "text/plain; charset=utf-8",
		"result.json": "text/plain; charset=utf-8",
		"out/a.cir":   "text/plain; charset=utf-8",
		"out/n.txt":   "text/plain; charset=utf-8",
		"in/netlist":  "text/plain; charset=utf-8",
		"in/data.bin": "text/plain; charset=utf-8",
	}
	for p, want := range cases {
		rec := get(h, "GET", "/api/execs/rcd1/file?path="+p)
		if rec.Code != 200 {
			t.Errorf("%s: status %d", p, rec.Code)
			continue
		}
		if got := rec.Header().Get("Content-Type"); got != want {
			t.Errorf("%s: content-type %q; want %q", p, got, want)
		}
		if rec.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("%s: cache-control %q", p, rec.Header().Get("Cache-Control"))
		}
		if rec.Body.String() != "data-"+p {
			t.Errorf("%s: body %q", p, rec.Body.String())
		}
	}
	if rec := get(h, "HEAD", "/api/execs/rcd1/file?path=out/sim.raw"); rec.Code != 200 || rec.Body.Len() != 0 {
		t.Errorf("HEAD: status %d body %d", rec.Code, rec.Body.Len())
	}
}

func TestFileErrors(t *testing.T) {
	ws, h := setup(t, okStatic)
	write(t, filepath.Join(ws, ".output/exec/rcd1/result.json"), "{}")
	write(t, filepath.Join(ws, "secret"), "s")
	outside := t.TempDir()
	write(t, filepath.Join(outside, "x"), "x")
	if err := os.Symlink(outside, filepath.Join(ws, ".output/exec/rcd1/esc")); err != nil {
		t.Fatal(err)
	}
	cases := map[string]int{
		"/api/execs/rcd1/file?path=../../../secret": 400,
		"/api/execs/rcd1/file?path=/etc/passwd":     400,
		"/api/execs/rcd1/file?path=a%5Cb":           400,
		"/api/execs/rcd1/file?path=":                400,
		"/api/execs/rcd1/file":                      400,
		"/api/execs/.hidden/file?path=result.json":  400,
		"/api/execs/rcd1/file?path=esc/x":           404,
		"/api/execs/rcd1/file?path=nope":            404,
		"/api/execs/nope/file?path=result.json":     404,
		"/api/execs/rcd1/file?path=.":               400,
	}
	for target, want := range cases {
		if rec := get(h, "GET", target); rec.Code != want {
			t.Errorf("%s: status %d; want %d", target, rec.Code, want)
		}
	}
}

func TestFileTooBig(t *testing.T) {
	ws, h := setup(t, okStatic)
	p := filepath.Join(ws, ".output/exec/rcd1/out/big.raw")
	write(t, p, "")
	if err := os.Truncate(p, wsread.MaxFileSize+1); err != nil {
		t.Skipf("truncate: %v", err)
	}
	if rec := get(h, "GET", "/api/execs/rcd1/file?path=out/big.raw"); rec.Code != 413 {
		t.Errorf("status %d; want 413", rec.Code)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	_, h := setup(t, okStatic)
	for _, m := range []string{"POST", "PUT", "DELETE", "PATCH", "OPTIONS"} {
		for _, target := range []string{"/api/execs", "/api/execs/rcd1/file?path=x", "/api/events", "/"} {
			if rec := get(h, m, target); rec.Code != 405 {
				t.Errorf("%s %s: status %d; want 405", m, target, rec.Code)
			}
		}
	}
}

func TestHostGuard(t *testing.T) {
	_, h := setup(t, okStatic)
	good := []string{"127.0.0.1:8787", "localhost:8787", "[::1]:8787", "127.0.0.2:1", "localhost"}
	bad := []string{"evil.example:8787", "example.com", "10.0.0.1:8787", "127.0.0.1.evil.com:80", "", "0.0.0.0:8787"}
	for _, host := range good {
		req := httptest.NewRequest("GET", "/api/execs", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Errorf("host %q: status %d", host, rec.Code)
		}
	}
	for _, host := range bad {
		req := httptest.NewRequest("GET", "/api/execs", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 421 && rec.Code != 403 {
			t.Errorf("host %q: status %d; want 421/403", host, rec.Code)
		}
		if rec.Header().Get("Content-Security-Policy") == "" {
			t.Errorf("host %q: rejection lacks security headers", host)
		}
	}
}

func TestSecurityHeaders(t *testing.T) {
	ws, h := setup(t, okStatic)
	write(t, filepath.Join(ws, ".output/exec/rcd1/result.json"), "{}")
	for _, target := range []string{"/", "/app.js", "/api/execs", "/api/execs/rcd1/file?path=result.json", "/api/execs/rcd1/file?path=bad", "/nope", "/api/nope"} {
		rec := get(h, "GET", target)
		if got := rec.Header().Get("Content-Security-Policy"); got != CSP {
			t.Errorf("%s: CSP %q", target, got)
		}
		if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: nosniff missing", target)
		}
		if rec.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Errorf("%s: referrer-policy missing", target)
		}
	}
	want := "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; worker-src 'self' blob:"
	if CSP != want {
		t.Errorf("CSP constant = %q", CSP)
	}
}

func TestStaticAndMissingBundle(t *testing.T) {
	_, h := setup(t, okStatic)
	if rec := get(h, "GET", "/"); rec.Code != 200 || !strings.Contains(rec.Body.String(), "ui") {
		t.Errorf("/: %d %q", rec.Code, rec.Body.String())
	}
	if rec := get(h, "GET", "/app.js"); rec.Code != 200 || rec.Body.String() != "console.log(1)" {
		t.Errorf("/app.js: %d %q", rec.Code, rec.Body.String())
	}
	if rec := get(h, "GET", "/.gitkeep"); rec.Code != 404 {
		t.Errorf("/.gitkeep: %d", rec.Code)
	}
	if rec := get(h, "GET", "/missing.css"); rec.Code != 404 {
		t.Errorf("/missing.css: %d", rec.Code)
	}

	_, h2 := setup(t, fstest.MapFS{".gitkeep": {}})
	rec := get(h2, "GET", "/")
	if rec.Code != 503 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain") {
		t.Fatalf("missing bundle: %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(rec.Body.String(), "agent_view: UI bundle not built (run npm run build in packages/agent-view/web)") {
		t.Errorf("body %q", rec.Body.String())
	}
}

func TestEventsEmitsOnChange(t *testing.T) {
	ws, h := setup(t, okStatic)
	ts := httptest.NewServer(h)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("status %d ct %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}

	lines := make(chan string, 16)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
	wait := func(want string) {
		t.Helper()
		timeout := time.After(5 * time.Second)
		for {
			select {
			case l, ok := <-lines:
				if !ok {
					t.Fatalf("stream closed before %q", want)
				}
				if l == want {
					return
				}
			case <-timeout:
				t.Fatalf("timeout waiting for %q", want)
			}
		}
	}
	wait(": connected") // baseline fingerprint is taken before this line is sent
	write(t, filepath.Join(ws, ".output/exec/rcd1/result.json"), "{}")
	wait("event: execs")
	wait("data: {}")

	write(t, filepath.Join(ws, ".output/exec/rcd1/out/sim.raw"), "abc")
	wait("event: execs")
}

func TestEventsStopsOnDisconnect(t *testing.T) {
	_, h := setup(t, okStatic)
	done := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r)
		close(done)
	}))
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/api/events")
	if err != nil {
		t.Fatal(err)
	}
	br := bufio.NewReader(resp.Body)
	if _, err := br.ReadString('\n'); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not return after client disconnect")
	}
}

func TestEventsPing(t *testing.T) {
	ws := t.TempDir()
	r, err := wsread.New(ws)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(New(Config{WS: r, Static: okStatic, PollInterval: time.Hour, PingInterval: 20 * time.Millisecond}))
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/api/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	sc := bufio.NewScanner(resp.Body)
	deadline := time.Now().Add(5 * time.Second)
	for sc.Scan() {
		if sc.Text() == ": ping" {
			return
		}
		if time.Now().After(deadline) {
			break
		}
	}
	t.Fatal("no ping comment received")
}
