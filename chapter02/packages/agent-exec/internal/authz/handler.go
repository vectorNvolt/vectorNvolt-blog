package authz

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
)

// Logger receives one line per deny decision (stderr by default).
var Logger = log.New(os.Stderr, "ephemeral-authz: ", log.LstdFlags)

const pluginContentType = "application/vnd.docker.plugins.v1.2+json"

// Request is the AuthZReq/AuthZRes payload of the legacy plugin protocol.
type Request struct {
	User               string
	UserAuthNMethod    string
	RequestMethod      string
	RequestURI         string
	RequestBody        []byte
	RequestHeaders     map[string]string
	ResponseBody       []byte
	ResponseHeaders    map[string]string
	ResponseStatusCode int
}

// Response is the plugin's answer.
type Response struct {
	Allow bool
	Msg   string
	Err   string
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", pluginContentType)
	_ = json.NewEncoder(w).Encode(v)
}

// NewHandler serves the legacy authorization plugin protocol (IP-ENGINE-AUTHZ).
func NewHandler(p *Policy) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/Plugin.Activate", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, map[string][]string{"Implements": {"authz"}})
	})
	mux.HandleFunc("/AuthZPlugin.AuthZReq", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, authzReq(p, req))
	})
	mux.HandleFunc("/AuthZPlugin.AuthZRes", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// The decision is made on the request; the response is always allowed.
		writeJSON(w, Response{Allow: true})
	})
	return mux
}

func authzReq(p *Policy, req *http.Request) (resp Response) {
	var in Request
	defer func() {
		if rec := recover(); rec != nil {
			resp = Response{Msg: fmt.Sprintf("internal error: %v", rec)}
		}
		if !resp.Allow {
			Logger.Printf("deny %s %s: %s", in.RequestMethod, in.RequestURI, resp.Msg)
		}
	}()
	data, err := io.ReadAll(http.MaxBytesReader(nil, req.Body, 16<<20))
	if err != nil {
		return Response{Msg: "unreadable plugin request"}
	}
	if err := json.Unmarshal(data, &in); err != nil {
		return Response{Msg: "unparsable plugin request"}
	}
	allow, msg := p.Decide(in.RequestMethod, in.RequestURI, in.RequestBody)
	return Response{Allow: allow, Msg: msg}
}
