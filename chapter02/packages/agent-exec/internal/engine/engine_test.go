package engine

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func frame(stream byte, s string) []byte {
	h := make([]byte, 8)
	h[0] = stream
	binary.BigEndian.PutUint32(h[4:], uint32(len(s)))
	return append(h, s...)
}

func TestDemux(t *testing.T) {
	var in []byte
	in = append(in, frame(1, "out1 ")...)
	in = append(in, frame(2, "err1 ")...)
	in = append(in, frame(1, "")...)
	in = append(in, frame(1, "out2")...)
	in = append(in, frame(2, "err2")...)
	var so, se bytes.Buffer
	if err := Demux(bytes.NewReader(in), &so, &se); err != nil {
		t.Fatal(err)
	}
	if so.String() != "out1 out2" || se.String() != "err1 err2" {
		t.Fatalf("stdout %q stderr %q", so.String(), se.String())
	}
}

func TestDemuxErrors(t *testing.T) {
	var so, se bytes.Buffer
	if err := Demux(bytes.NewReader(nil), &so, &se); err != nil {
		t.Errorf("empty stream: %v", err)
	}
	if err := Demux(bytes.NewReader(frame(1, "abcdef")[:11]), &so, &se); err == nil {
		t.Error("truncated frame accepted")
	}
	if err := Demux(bytes.NewReader(frame(9, "x")), &so, &se); err == nil {
		t.Error("unknown stream type accepted")
	}
	if err := Demux(bytes.NewReader([]byte{1, 0, 0}), &so, &se); err == nil {
		t.Error("truncated header accepted")
	}
}

func TestIsDenied(t *testing.T) {
	if !IsDenied(&APIError{Status: 403, Message: "x"}) {
		t.Error("403")
	}
	if !IsDenied(&APIError{Status: 500, Message: "Authorization denied by plugin"}) {
		t.Error("message")
	}
	if IsDenied(&APIError{Status: 500, Message: "boom"}) || IsDenied(errors.New("x")) {
		t.Error("false positive")
	}
}

func startServer(t *testing.T, mux *http.ServeMux) *Client {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "d.sock")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Skipf("no unix sockets: %v", err)
	}
	srv := &http.Server{Handler: mux}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close() })
	c, err := NewClient("unix://" + sock)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestClient(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/_ping", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("OK")) })
	mux.HandleFunc("/containers/create", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		w.Write([]byte(`{"message":"authorization denied by plugin ephemeral-authz: user must be 1001:1001"}`))
	})
	mux.HandleFunc("/images/sha256:missing/json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte(`{"message":"No such image"}`))
	})
	mux.HandleFunc("/images/sha256:there/json", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"Id":"sha256:there","Config":{"Env":["A=1"]}}`))
	})
	mux.HandleFunc("/containers/json", func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Query().Get("filters"), "ephemeral.exec") || r.URL.Query().Get("all") != "1" {
			w.WriteHeader(400)
			return
		}
		w.Write([]byte(`[{"Id":"abc","Names":["/ephemeral-e1"],"Labels":{"ephemeral.exec":"e1"},"State":"running"}]`))
	})
	mux.HandleFunc("/containers/abc/wait", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("condition") != "next-exit" {
			w.WriteHeader(400)
			return
		}
		w.Write([]byte(`{"StatusCode":3}`))
	})
	mux.HandleFunc("/containers/gone", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	})
	c := startServer(t, mux)
	ctx := context.Background()

	if err := c.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	_, err := c.ContainerCreate(ctx, "ephemeral-e1", CreateBody{})
	if !IsDenied(err) {
		t.Fatalf("create: %v", err)
	}
	if _, err := c.ImageInspect(ctx, "sha256:missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing image: %v", err)
	}
	img, err := c.ImageInspect(ctx, "sha256:there")
	if err != nil || img.ID != "sha256:there" || len(img.Config.Env) != 1 {
		t.Fatalf("image: %+v %v", img, err)
	}
	list, err := c.ContainerList(ctx, []string{"ephemeral.exec"}, true)
	if err != nil || len(list) != 1 || list[0].Labels["ephemeral.exec"] != "e1" {
		t.Fatalf("list: %+v %v", list, err)
	}
	if code, err := c.ContainerWait(ctx, "abc"); err != nil || code != 3 {
		t.Fatalf("wait: %d %v", code, err)
	}
	if err := c.ContainerRemove(ctx, "gone"); err != nil {
		t.Fatalf("removing an absent container is not an error: %v", err)
	}
}

func TestClientAttach(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/containers/abc/attach", func(w http.ResponseWriter, r *http.Request) {
		conn, buf, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		buf.WriteString("HTTP/1.1 101 UPGRADED\r\nContent-Type: application/vnd.docker.raw-stream\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n")
		buf.Write(frame(1, "hello "))
		buf.Write(frame(2, "oops"))
		buf.Write(frame(1, "world"))
		buf.Flush()
	})
	mux.HandleFunc("/containers/nope/attach", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		w.Write([]byte(`{"message":"authorization denied"}`))
	})
	c := startServer(t, mux)
	var so, se bytes.Buffer
	done, err := c.ContainerAttach(context.Background(), "abc", &so, &se)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("attach did not end")
	}
	if so.String() != "hello world" || se.String() != "oops" {
		t.Fatalf("stdout %q stderr %q", so.String(), se.String())
	}
	if _, err := c.ContainerAttach(context.Background(), "nope", &so, &se); !IsDenied(err) {
		t.Fatalf("denied attach: %v", err)
	}
}

func TestNewClientHost(t *testing.T) {
	if _, err := NewClient("tcp://127.0.0.1:2375"); err == nil {
		t.Error("tcp host accepted")
	}
	if _, err := NewClient(""); err != nil {
		t.Errorf("default host: %v", err)
	}
	if _, err := NewClient("/tmp/x.sock"); err != nil {
		t.Errorf("plain path: %v", err)
	}
}
