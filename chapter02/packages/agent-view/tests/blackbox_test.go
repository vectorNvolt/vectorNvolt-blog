package tests

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"agent-view/internal/server"
	"agent-view/internal/ui"
	"agent-view/internal/wsread"
)

func start(t *testing.T) (string, *httptest.Server) {
	t.Helper()
	ws := t.TempDir()
	r, err := wsread.New(ws)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(server.New(server.Config{WS: r, Static: ui.FS()}))
	t.Cleanup(ts.Close)
	return ws, ts
}

func fetch(t *testing.T, url string) (*http.Response, []byte) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, b
}

func TestEndToEnd(t *testing.T) {
	ws, ts := start(t)
	dir := filepath.Join(ws, ".output", "exec", "rcd1")
	if err := os.MkdirAll(filepath.Join(dir, "out"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "result.json"), []byte(`{"exit_code":0}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "out", "sim.log"), []byte("done"), 0o644); err != nil {
		t.Fatal(err)
	}

	resp, body := fetch(t, ts.URL+"/api/execs")
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	var list struct {
		Execs []struct {
			ID      string   `json:"id"`
			Outputs []string `json:"outputs"`
		} `json:"execs"`
	}
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Execs) != 1 || list.Execs[0].ID != "rcd1" || len(list.Execs[0].Outputs) != 1 {
		t.Fatalf("list: %s", body)
	}

	resp, body = fetch(t, ts.URL+"/api/execs/rcd1/file?path=out/sim.log")
	if resp.StatusCode != 200 || string(body) != "done" {
		t.Fatalf("file: %d %q", resp.StatusCode, body)
	}
	if resp.Header.Get("Content-Security-Policy") == "" {
		t.Error("CSP missing")
	}
}

func TestTraversalOverTheWire(t *testing.T) {
	ws, ts := start(t)
	if err := os.WriteFile(filepath.Join(ws, "secret.txt"), []byte("s"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(ws, ".output", "exec", "rcd1"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{
		"/api/execs/rcd1/file?path=../../../../secret.txt",
		"/api/execs/rcd1/file?path=..%2F..%2F..%2F..%2Fsecret.txt",
		"/api/execs/rcd1/file?path=%2Fetc%2Fpasswd",
		"/api/execs/..%2F..%2F/file?path=secret.txt",
		"/api/execs/rcd1/file?path=a%00b",
	} {
		resp, body := fetch(t, ts.URL+p)
		if resp.StatusCode == 200 {
			t.Errorf("%s served: %q", p, body)
		}
	}
}

func TestReadOnly(t *testing.T) {
	_, ts := start(t)
	resp, err := http.Post(ts.URL+"/api/execs", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST status %d", resp.StatusCode)
	}
}
