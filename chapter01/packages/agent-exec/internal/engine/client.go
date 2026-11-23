package engine

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// DefaultHost is the in-container engine socket (SSR-AGC-011: group docker).
const DefaultHost = "unix:///var/run/docker.sock"

// Client talks the Engine API over a Unix socket with net/http (no docker CLI, no SDK).
type Client struct {
	sock string
	hc   *http.Client
}

// NewClient accepts unix:///path or a plain socket path.
func NewClient(host string) (*Client, error) {
	if host == "" {
		host = DefaultHost
	}
	sock := host
	if strings.HasPrefix(host, "unix://") {
		sock = strings.TrimPrefix(host, "unix://")
	} else if strings.Contains(host, "://") {
		return nil, fmt.Errorf("unsupported engine host %q (unix socket only)", host)
	}
	if sock == "" {
		return nil, fmt.Errorf("empty engine socket path")
	}
	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", sock)
		},
		DisableCompression: true,
	}
	return &Client{sock: sock, hc: &http.Client{Transport: tr}}, nil
}

func (c *Client) do(ctx context.Context, method, path string, q url.Values, body io.Reader, ctype string) (*http.Response, error) {
	u := "http://docker" + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, err
	}
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func apiError(status int, body io.Reader) error {
	data, _ := io.ReadAll(io.LimitReader(body, 4096))
	msg := strings.TrimSpace(string(data))
	var m struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(data, &m) == nil && m.Message != "" {
		msg = m.Message
	}
	return &APIError{Status: status, Message: msg}
}

// call performs a request and returns the body of a response with one of the ok statuses.
func (c *Client) call(ctx context.Context, method, path string, q url.Values, body io.Reader, ctype string, ok ...int) ([]byte, error) {
	resp, err := c.do(ctx, method, path, q, body, ctype)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	for _, s := range ok {
		if resp.StatusCode == s {
			return io.ReadAll(resp.Body)
		}
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%w: %v", ErrNotFound, apiError(resp.StatusCode, resp.Body))
	}
	return nil, apiError(resp.StatusCode, resp.Body)
}

func (c *Client) Ping(ctx context.Context) error {
	_, err := c.call(ctx, "GET", "/_ping", nil, nil, "", 200)
	return err
}

func (c *Client) ImageInspect(ctx context.Context, id string) (*ImageInfo, error) {
	data, err := c.call(ctx, "GET", "/images/"+id+"/json", nil, nil, "", 200)
	if err != nil {
		return nil, err
	}
	var info ImageInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// ImageLoad streams image.tar to POST /images/load and fails on an error line of the answer.
func (c *Client) ImageLoad(ctx context.Context, tar io.Reader) error {
	resp, err := c.do(ctx, "POST", "/images/load", url.Values{"quiet": {"1"}}, tar, "application/x-tar")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return apiError(resp.StatusCode, resp.Body)
	}
	dec := json.NewDecoder(resp.Body)
	for {
		var line struct {
			Error string `json:"error"`
		}
		if err := dec.Decode(&line); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if line.Error != "" {
			return fmt.Errorf("image load: %s", line.Error)
		}
	}
}

func (c *Client) ContainerCreate(ctx context.Context, name string, body CreateBody) (string, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	data, err := c.call(ctx, "POST", "/containers/create", url.Values{"name": {name}}, bytes.NewReader(raw), "application/json", 201)
	if err != nil {
		return "", err
	}
	var out struct {
		ID string `json:"Id"`
	}
	if err := json.Unmarshal(data, &out); err != nil || out.ID == "" {
		return "", fmt.Errorf("create: unreadable answer")
	}
	return out.ID, nil
}

func (c *Client) ContainerList(ctx context.Context, labelFilters []string, all bool) ([]ContainerSummary, error) {
	q := url.Values{}
	if all {
		q.Set("all", "1")
	}
	if len(labelFilters) > 0 {
		f, _ := json.Marshal(map[string][]string{"label": labelFilters})
		q.Set("filters", string(f))
	}
	data, err := c.call(ctx, "GET", "/containers/json", q, nil, "", 200)
	if err != nil {
		return nil, err
	}
	var out []ContainerSummary
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) ContainerInspect(ctx context.Context, id string) (*Inspect, error) {
	data, err := c.call(ctx, "GET", "/containers/"+id+"/json", nil, nil, "", 200)
	if err != nil {
		return nil, err
	}
	var info Inspect
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

func (c *Client) ContainerStart(ctx context.Context, id string) error {
	_, err := c.call(ctx, "POST", "/containers/"+id+"/start", nil, nil, "", 204, 304)
	return err
}

func (c *Client) ContainerWait(ctx context.Context, id string) (int, error) {
	data, err := c.call(ctx, "POST", "/containers/"+id+"/wait", url.Values{"condition": {"next-exit"}}, nil, "", 200)
	if err != nil {
		return 0, err
	}
	var out struct {
		StatusCode int `json:"StatusCode"`
		Error      *struct {
			Message string `json:"Message"`
		} `json:"Error"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return 0, err
	}
	if out.Error != nil && out.Error.Message != "" {
		return out.StatusCode, fmt.Errorf("wait: %s", out.Error.Message)
	}
	return out.StatusCode, nil
}

func (c *Client) ContainerKill(ctx context.Context, id string) error {
	_, err := c.call(ctx, "POST", "/containers/"+id+"/kill", url.Values{"signal": {"KILL"}}, nil, "", 204)
	return err
}

func (c *Client) ContainerRemove(ctx context.Context, id string) error {
	_, err := c.call(ctx, "DELETE", "/containers/"+id, url.Values{"force": {"1"}, "v": {"1"}}, nil, "", 204)
	if err == nil || isNotFound(err) {
		return nil
	}
	var ae *APIError
	if asAPI(err, &ae) && ae.Status == 409 && strings.Contains(ae.Message, "already in progress") {
		return nil
	}
	return err
}

// ContainerAttach opens the raw (hijacked) attach stream and demultiplexes it in a goroutine.
func (c *Client) ContainerAttach(ctx context.Context, id string, stdout, stderr io.Writer) (<-chan error, error) {
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", c.sock)
	if err != nil {
		return nil, err
	}
	req := "POST /containers/" + id + "/attach?stream=1&stdout=1&stderr=1 HTTP/1.1\r\n" +
		"Host: docker\r\nUpgrade: tcp\r\nConnection: Upgrade\r\nContent-Length: 0\r\n\r\n"
	if _, err := io.WriteString(conn, req); err != nil {
		conn.Close()
		return nil, err
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		conn.Close()
		return nil, err
	}
	if resp.StatusCode != http.StatusSwitchingProtocols && resp.StatusCode != http.StatusOK {
		defer conn.Close()
		return nil, apiError(resp.StatusCode, resp.Body)
	}
	done := make(chan error, 1)
	stop := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-stop:
		}
	}()
	go func() {
		err := Demux(br, stdout, stderr)
		close(stop)
		conn.Close()
		done <- err
	}()
	return done, nil
}

// Events subscribes to the events of one container. The headers arrive when the subscription is active.
func (c *Client) Events(ctx context.Context, id string) (<-chan Event, error) {
	f, _ := json.Marshal(map[string][]string{"container": {id}, "type": {"container"}})
	resp, err := c.do(ctx, "GET", "/events", url.Values{"filters": {string(f)}}, nil, "")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		defer resp.Body.Close()
		return nil, apiError(resp.StatusCode, resp.Body)
	}
	ch := make(chan Event, 16)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		dec := json.NewDecoder(resp.Body)
		for {
			var raw struct {
				Type   string `json:"Type"`
				Action string `json:"Action"`
				Actor  struct {
					ID         string            `json:"ID"`
					Attributes map[string]string `json:"Attributes"`
				} `json:"Actor"`
			}
			if err := dec.Decode(&raw); err != nil {
				return
			}
			ev := Event{Type: raw.Type, Action: raw.Action, ID: raw.Actor.ID, Attributes: raw.Actor.Attributes}
			select {
			case ch <- ev:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}
