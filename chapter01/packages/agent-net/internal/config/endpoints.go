// Endpoint list file: the hostnames (or IPv4 literals) the agents may reach on TCP 443. Shared by create_agent_net
// (arm, check-gate) and refresh_endpoints, so both read the same source.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/netip"
	"regexp"
	"strings"
)

const (
	// DefaultEndpointsFile is read when it exists; DefaultModelAPIs is the fallback when it does not.
	DefaultEndpointsFile = "/etc/agent-net/endpoints.json"
	// EnvEndpointsFile overrides the path of the endpoints file. A path set this way must exist.
	EnvEndpointsFile = "AGENT_NET_ENDPOINTS_FILE"
)

var hostLabelRe = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)

// validEndpoint accepts an IPv4 literal or a host name (labels of letters, digits and hyphens, not purely numeric).
func validEndpoint(s string) bool {
	if a, err := netip.ParseAddr(s); err == nil {
		return a.Is4()
	}
	if s == "" || len(s) > 253 {
		return false
	}
	labels := strings.Split(s, ".")
	for _, l := range labels {
		if !hostLabelRe.MatchString(l) {
			return false
		}
	}
	// A name whose last label is numeric is a malformed address, not a host name.
	last := labels[len(labels)-1]
	return strings.Trim(last, "0123456789") != ""
}

// ParseEndpoints parses `{"endpoints": ["host", ...]}`: at least one entry, each a valid host name or IPv4 literal,
// no duplicates (case-insensitive).
func ParseEndpoints(data []byte) ([]string, error) {
	var doc struct {
		Endpoints []string `json:"endpoints"`
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return nil, errors.New("invalid JSON: unexpected data after the JSON object")
	}
	if len(doc.Endpoints) == 0 {
		return nil, errors.New(`"endpoints" is missing or empty`)
	}
	seen := map[string]bool{}
	for _, e := range doc.Endpoints {
		if !validEndpoint(e) {
			return nil, fmt.Errorf("%q is not a valid host name or IPv4 address", e)
		}
		k := strings.ToLower(e)
		if seen[k] {
			return nil, fmt.Errorf("duplicate endpoint %q", e)
		}
		seen[k] = true
	}
	return doc.Endpoints, nil
}

// CheckEndpointsAccess refuses a file that is not a regular file, not owned by root, or writable by group or others.
func CheckEndpointsAccess(path string, uid int, mode fs.FileMode) error {
	switch {
	case !mode.IsRegular():
		return fmt.Errorf("%s is not a regular file", path)
	case uid != 0:
		return fmt.Errorf("%s is not owned by root (uid %d)", path, uid)
	case mode.Perm()&0o022 != 0:
		return fmt.Errorf("%s is writable by group or others (mode %04o)", path, mode.Perm())
	}
	return nil
}

// ReadEndpointsFile checks and parses the endpoints file. stat may be nil: the ownership and mode check is then
// skipped (injected hosts of tests). A missing file is an error wrapping fs.ErrNotExist.
func ReadEndpointsFile(path string, read func(string) ([]byte, error), stat func(string) (int, fs.FileMode, error)) ([]string, error) {
	if stat != nil {
		uid, mode, err := stat(path)
		if err != nil {
			return nil, err
		}
		if err := CheckEndpointsAccess(path, uid, mode); err != nil {
			return nil, err
		}
	}
	data, err := read(path)
	if err != nil {
		return nil, err
	}
	list, err := ParseEndpoints(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return list, nil
}
