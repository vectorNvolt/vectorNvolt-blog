// Package authz is the compiled-in default-deny policy of the engine authorization
// plugin ephemeral-authz (SSR-AGE-007). It reads nothing from the workspace; the only
// filesystem access is the symlink check on bind sources at create.
package authz

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"agent-exec/internal/limits"
)

// Policy holds the only injectable setting: the filesystem root against which bind
// sources are checked for symlink components (tests use a temp dir).
type Policy struct {
	Root string
}

// Default is the policy used by the plugin binary.
var Default = &Policy{Root: "/"}

// Decide applies the default policy.
func Decide(method, uri string, body []byte) (bool, string) {
	return Default.Decide(method, uri, body)
}

const (
	idPat  = `[A-Za-z0-9][A-Za-z0-9_.-]*`
	refPat = `[A-Za-z0-9][A-Za-z0-9_.:@/-]*`
)

var (
	versionRe = regexp.MustCompile(`^/v[0-9]+\.[0-9]+(/.*)$`)
	execIDRe  = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
	nameRe    = regexp.MustCompile(`^ephemeral-([a-z0-9][a-z0-9-]{0,69})$`) // id (<=64) plus optional -fetch; id is checked against the label
)

type route struct {
	method string
	re     *regexp.Regexp
	// emptyBody: the request body must be empty (start).
	emptyBody bool
	// create: body checked against the create invariants.
	create bool
}

func r(method, pattern string) route {
	return route{method: method, re: regexp.MustCompile("^" + pattern + "$")}
}

// SSR-AGE-007: endpoint allowlist (USS-005 "Engine authorization plugin"). Anything
// not listed is denied (exec, archive, export, commit, update, build, images/create,
// push, tag, networks, volumes, plugins, swarm, session, grpc, ...).
var routes = []route{
	r("GET", `/_ping`),
	r("HEAD", `/_ping`),
	r("GET", `/version`),
	r("GET", `/info`),
	r("GET", `/images/json`),
	r("GET", `/images/`+refPat+`/json`),
	r("POST", `/images/load`),
	{method: "POST", re: regexp.MustCompile(`^/containers/create$`), create: true},
	r("GET", `/containers/json`),
	r("GET", `/containers/`+idPat+`/json`),
	{method: "POST", re: regexp.MustCompile(`^/containers/` + idPat + `/start$`), emptyBody: true},
	r("POST", `/containers/`+idPat+`/attach`),
	r("GET", `/containers/`+idPat+`/logs`),
	r("POST", `/containers/`+idPat+`/wait`),
	r("POST", `/containers/`+idPat+`/kill`),
	r("DELETE", `/containers/`+idPat),
	r("GET", `/events`),
}

// Decide returns whether the request is allowed and a one-line reason. It never
// panics: a panic is a deny.
func (p *Policy) Decide(method, uri string, body []byte) (allow bool, msg string) {
	defer func() {
		if rec := recover(); rec != nil {
			allow, msg = false, fmt.Sprintf("internal error: %v", rec)
		}
	}()

	rawPath, rawQuery, _ := strings.Cut(uri, "?")
	// Percent-encoded paths are never needed and are a classic way to confuse a router.
	if strings.Contains(rawPath, "%") {
		return false, "encoded path"
	}
	if rawPath != pathpkg.Clean(rawPath) || !strings.HasPrefix(rawPath, "/") {
		return false, "path not canonical"
	}
	// Optional API version prefix /v1.NN is stripped; the policy is version independent.
	if m := versionRe.FindStringSubmatch(rawPath); m != nil {
		rawPath = m[1]
	}
	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		return false, "unparsable query"
	}

	for _, rt := range routes {
		if rt.method != method || !rt.re.MatchString(rawPath) {
			continue
		}
		if rt.emptyBody && len(body) != 0 {
			return false, "start with a body is not allowed"
		}
		if rt.create {
			if reason := p.checkCreate(query, body); reason != "" {
				return false, reason
			}
		}
		return true, "allowed"
	}
	return false, "endpoint not on the allowlist"
}

// createReq holds the fields of POST /containers/create that the policy looks at.
// Unknown fields are ignored (Docker sends many); dangerous ones are listed explicitly.
type createReq struct {
	User       string
	Labels     map[string]string
	Volumes    map[string]json.RawMessage
	HostConfig *hostConfig
}

type hostConfig struct {
	Binds             []string
	Privileged        bool
	CapAdd            []string
	CapDrop           []string
	SecurityOpt       []string
	ReadonlyRootfs    bool
	NetworkMode       string
	PidMode           string
	IpcMode           string
	UTSMode           string
	UsernsMode        string
	CgroupnsMode      string
	GroupAdd          []string
	Devices           []json.RawMessage
	DeviceRequests    []json.RawMessage
	DeviceCgroupRules []string
	Sysctls           map[string]json.RawMessage
	CgroupParent      string
	PortBindings      map[string]json.RawMessage
	PublishAllPorts   bool
	Links             []string
	VolumesFrom       []string
	VolumeDriver      string
	Runtime           string
	OomKillDisable    *bool
	Memory            int64
	NanoCpus          int64
	PidsLimit         *int64
	AutoRemove        bool
	Mounts            []mount
	Tmpfs             map[string]string
	MaskedPaths       json.RawMessage
	ReadonlyPaths     json.RawMessage
	LogConfig         struct{ Type string }
}

type mount struct {
	Type        string
	Source      string
	Target      string
	ReadOnly    bool
	BindOptions *struct {
		Propagation      string
		CreateMountpoint bool
	}
	VolumeOptions json.RawMessage
	ImageOptions  json.RawMessage
	TmpfsOptions  *struct {
		SizeBytes int64
	}
}

func present(raw json.RawMessage) bool {
	t := bytes.TrimSpace(raw)
	return len(t) > 0 && string(t) != "null"
}

// checkCreate returns "" when the create request satisfies every invariant, else the
// reason of the first violation.
func (p *Policy) checkCreate(query url.Values, body []byte) string {
	if len(bytes.TrimSpace(body)) == 0 {
		return "create without body"
	}
	var req createReq
	dec := json.NewDecoder(bytes.NewReader(body))
	if err := dec.Decode(&req); err != nil {
		return "create body unparsable"
	}
	if _, err := dec.Token(); err != io.EOF {
		return "create body has trailing data"
	}
	hc := req.HostConfig
	if hc == nil {
		return "create without HostConfig"
	}

	// SSR-AGE-001/002: user and groups.
	if want := fmt.Sprintf("%d:%d", limits.AgentUID, limits.AgentGID); req.User != want {
		return "User must be " + want
	}
	if len(hc.GroupAdd) > 0 {
		return "GroupAdd must be empty"
	}

	// Privilege.
	if hc.Privileged {
		return "Privileged must be false"
	}
	if len(hc.CapAdd) > 0 {
		return "CapAdd must be empty"
	}
	if !contains(hc.CapDrop, "ALL") {
		return "CapDrop must contain ALL"
	}
	if reason := checkSecurityOpt(hc.SecurityOpt); reason != "" {
		return reason
	}
	if !hc.ReadonlyRootfs {
		return "ReadonlyRootfs must be true"
	}
	if present(hc.MaskedPaths) || present(hc.ReadonlyPaths) {
		return "MaskedPaths/ReadonlyPaths must not be set"
	}

	// Network and namespaces.
	if hc.NetworkMode != "none" && hc.NetworkMode != "host" {
		return "NetworkMode must be none or host"
	}
	for name, v := range map[string]string{
		"PidMode": hc.PidMode, "IpcMode": hc.IpcMode, "UTSMode": hc.UTSMode,
		"UsernsMode": hc.UsernsMode,
	} {
		if v != "" && v != "private" {
			return name + " must be private"
		}
	}
	// Cgroup namespace must be host: gVisor has no cgroup namespaces; host = the agent container's view.
	if hc.CgroupnsMode != "host" {
		return "CgroupnsMode must be host"
	}

	// Forbidden fields.
	switch {
	case len(hc.Devices) > 0:
		return "Devices forbidden"
	case len(hc.DeviceRequests) > 0:
		return "DeviceRequests forbidden"
	case len(hc.DeviceCgroupRules) > 0:
		return "DeviceCgroupRules forbidden"
	case len(hc.Sysctls) > 0:
		return "Sysctls forbidden"
	case hc.CgroupParent != "":
		return "CgroupParent forbidden"
	case len(hc.PortBindings) > 0 || hc.PublishAllPorts:
		return "port publishing forbidden"
	case len(hc.Links) > 0:
		return "Links forbidden"
	case len(hc.VolumesFrom) > 0:
		return "VolumesFrom forbidden"
	case len(req.Volumes) > 0:
		return "anonymous Volumes forbidden"
	case hc.VolumeDriver != "":
		return "VolumeDriver forbidden"
	// Design decision: binds only through HostConfig.Mounts, never Binds (explicit Type
	// and Target). Tmpfs is accepted as the HostConfig.Tmpfs map or as tmpfs Mounts
	// (checkTmpfsMap, checkMounts).
	case len(hc.Binds) > 0:
		return "Binds forbidden, use Mounts"
	}
	switch hc.LogConfig.Type {
	case "", "json-file", "local", "none":
	default:
		return "LogConfig driver not allowed"
	}

	if hc.Runtime != "" && hc.Runtime != "runc" {
		return "Runtime must be runc"
	}

	// SSR-AGE-008: limits.
	if hc.OomKillDisable != nil && *hc.OomKillDisable {
		return "OomKillDisable must be false"
	}
	if hc.Memory <= 0 || hc.Memory > limits.MaxMemoryBytes {
		return "Memory must be set and within the ceiling"
	}
	if hc.NanoCpus <= 0 || hc.NanoCpus > limits.MaxNanoCPUs {
		return "NanoCpus must be set and within the ceiling"
	}
	if hc.PidsLimit == nil || *hc.PidsLimit <= 0 || *hc.PidsLimit > limits.MaxPids {
		return "PidsLimit must be set and within the ceiling"
	}

	// SSR-AGE-013: identity (name and label carry the same exec id).
	id, reason := checkIdentity(query, req.Labels)
	if reason != "" {
		return reason
	}

	if !hc.AutoRemove {
		return "AutoRemove must be true"
	}

	if reason := p.checkMounts(id, hc.Mounts); reason != "" {
		return reason
	}
	return checkTmpfsMap(hc.Tmpfs, hc.Mounts)
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// checkSecurityOpt requires no-new-privileges and rejects every option that weakens
// the default confinement. Stricter than the table: any label option and any seccomp
// option (not only unconfined) is denied, because the tool never sets one.
func checkSecurityOpt(opts []string) string {
	nnp := false
	for _, o := range opts {
		o = strings.TrimSpace(o)
		norm := strings.Replace(o, ":", "=", 1)
		key, val, _ := strings.Cut(norm, "=")
		key = strings.ToLower(key)
		val = strings.ToLower(val)
		switch key {
		case "no-new-privileges":
			if val == "" || val == "true" {
				nnp = true
			} else {
				return "no-new-privileges must be true"
			}
		case "seccomp":
			return "seccomp option forbidden"
		case "apparmor":
			if val == "unconfined" {
				return "apparmor=unconfined forbidden"
			}
		case "label":
			return "label option forbidden"
		case "systempaths":
			return "systempaths option forbidden"
		}
	}
	if !nnp {
		return "SecurityOpt must contain no-new-privileges"
	}
	return ""
}

func checkIdentity(query url.Values, labels map[string]string) (id, reason string) {
	names := query["name"]
	if len(names) != 1 {
		return "", "name query parameter required"
	}
	m := nameRe.FindStringSubmatch(names[0])
	if m == nil {
		return "", "name must be ephemeral-<id> or ephemeral-<id>-fetch"
	}
	label, ok := labels["ephemeral.exec"]
	if !ok {
		return "", "label ephemeral.exec required"
	}
	if !execIDRe.MatchString(label) {
		return "", "label ephemeral.exec is not a valid id"
	}
	full := m[1]
	if label == full || (strings.HasSuffix(full, "-fetch") && label == strings.TrimSuffix(full, "-fetch")) {
		return label, ""
	}
	return "", "name and label ephemeral.exec differ"
}

type bindRule struct{ ro, rw bool }

// SSR-AGE-003: bind source allowlist (target equals source).
func bindRules(id string) map[string]bindRule {
	out := "/workspace/.output/exec/" + id
	return map[string]bindRule{
		"/workspace/src":        {ro: true},
		"/workspace/build":      {ro: true, rw: true},
		"/workspace/.venv":      {ro: true, rw: true},
		"/workspace/.npm-env":   {ro: true, rw: true},
		"/workspace/.conda-env": {ro: true, rw: true},
		"/workspace/.packages":  {ro: true},
		out + "/in":             {ro: true},
		out + "/dl":             {ro: true, rw: true},
		out + "/out":            {rw: true},
	}
}

func tmpfsCeilings() map[string]int64 {
	return map[string]int64{
		"/work":     limits.WorkTmpfsBytes,
		"/tmp":      limits.TmpTmpfsBytes,
		"/home/run": limits.HomeTmpfsBytes,
	}
}

// SSR-AGE-004: HostConfig.Tmpfs map (target -> comma-separated options). Targets are the
// three scratch areas, never also present in Mounts; size= is required and bounded by
// the ceiling; only a fixed set of options is accepted.
func checkTmpfsMap(tmpfs map[string]string, mounts []mount) string {
	if len(tmpfs) == 0 {
		return ""
	}
	ceilings := tmpfsCeilings()
	inMounts := map[string]bool{}
	for _, m := range mounts {
		inMounts[m.Target] = true
	}
	for target, opts := range tmpfs {
		max, ok := ceilings[target]
		if !ok {
			return "tmpfs target not allowed: " + target
		}
		if inMounts[target] {
			return "duplicate mount target " + target
		}
		size := int64(-1)
		for _, o := range strings.Split(opts, ",") {
			switch {
			case o == "rw", o == "ro", o == "noexec", o == "exec", o == "nosuid", o == "suid", o == "nodev", o == "dev":
			case strings.HasPrefix(o, "size="):
				if size != -1 {
					return "tmpfs size given twice: " + target
				}
				n, ok := parseSize(strings.TrimPrefix(o, "size="))
				if !ok {
					return "tmpfs size unparsable: " + target
				}
				size = n
			case strings.HasPrefix(o, "mode="):
				if _, err := strconv.ParseUint(strings.TrimPrefix(o, "mode="), 8, 32); err != nil {
					return "tmpfs mode unparsable: " + target
				}
			default:
				return "tmpfs option not allowed: " + target
			}
		}
		if size <= 0 || size > max {
			return "tmpfs size must be set and within the ceiling: " + target
		}
	}
	return ""
}

// parseSize reads a byte count: plain digits with an optional k, m or g suffix (powers
// of 1024, case-insensitive), as docker accepts for size=. Overflow is a failure.
func parseSize(s string) (int64, bool) {
	mult := int64(1)
	if s != "" {
		switch s[len(s)-1] {
		case 'k', 'K':
			mult = 1 << 10
		case 'm', 'M':
			mult = 1 << 20
		case 'g', 'G':
			mult = 1 << 30
		}
		if mult != 1 {
			s = s[:len(s)-1]
		}
	}
	if s == "" {
		return 0, false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n > (1<<62)/mult {
		return 0, false
	}
	return n * mult, true
}

func (p *Policy) checkMounts(id string, mounts []mount) string {
	rules := bindRules(id)
	ceilings := tmpfsCeilings()
	seen := map[string]bool{}
	for _, m := range mounts {
		if strings.Contains(m.Source, "docker.sock") || strings.Contains(m.Target, "docker.sock") {
			return "engine socket mount forbidden"
		}
		if seen[m.Target] {
			return "duplicate mount target " + m.Target
		}
		seen[m.Target] = true
		if present(m.VolumeOptions) || present(m.ImageOptions) {
			return "volume/image options forbidden"
		}
		switch m.Type {
		case "bind":
			rule, ok := rules[m.Source]
			if !ok || m.Source != pathpkg.Clean(m.Source) {
				return "bind source not allowed: " + m.Source
			}
			if m.Target != m.Source {
				return "bind target must equal source: " + m.Source
			}
			if m.ReadOnly && !rule.ro {
				return "bind must be read-write: " + m.Source
			}
			if !m.ReadOnly && !rule.rw {
				return "bind must be read-only: " + m.Source
			}
			if m.BindOptions != nil {
				if m.BindOptions.Propagation != "" && m.BindOptions.Propagation != "rprivate" {
					return "bind propagation forbidden: " + m.Source
				}
				if m.BindOptions.CreateMountpoint {
					return "CreateMountpoint forbidden: " + m.Source
				}
			}
			if err := p.noSymlink(m.Source); err != nil {
				return "bind source: " + err.Error()
			}
		case "tmpfs":
			max, ok := ceilings[m.Target]
			if !ok {
				return "tmpfs target not allowed: " + m.Target
			}
			if m.Source != "" {
				return "tmpfs must have no source"
			}
			if m.TmpfsOptions == nil || m.TmpfsOptions.SizeBytes <= 0 || m.TmpfsOptions.SizeBytes > max {
				return "tmpfs size must be set and within the ceiling: " + m.Target
			}
		default:
			return "mount type not allowed: " + m.Type
		}
	}
	return ""
}

// noSymlink Lstats every component of an absolute path below the policy root and
// fails on a symlink or a missing component (a bind source must exist).
func (p *Policy) noSymlink(src string) error {
	root := p.Root
	if root == "" {
		root = "/"
	}
	cur := root
	for _, seg := range strings.Split(strings.TrimPrefix(src, "/"), "/") {
		cur = filepath.Join(cur, seg)
		fi, err := os.Lstat(cur)
		if err != nil {
			return fmt.Errorf("%s: %v", src, err)
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink component in %s", src)
		}
	}
	return nil
}
