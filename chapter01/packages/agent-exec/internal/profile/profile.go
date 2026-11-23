// Package profile loads and validates the operator-supplied execution profiles
// (/workspace/.tools/<name>/profile.yml). Profiles are read on every call, never cached.
// SFR-AGE-002 (reject invalid profile), SSR-AGE-005 (caller supplies data only),
// SSR-AGE-008 (ceilings), SSR-AGE-006 (images only from the operator),
// SSR-AGT-006 (kind tool: fixed entry, argv, inputs, input policy, typed params; never a mount or network).
package profile

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"agent-exec/internal/limits"
	"agent-exec/internal/wsfs"
)

const (
	KindCode    = "code"
	KindBuild   = "build"
	KindInstall = "install"
	KindTool    = "tool"

	EntryAgent     = "agent"
	EntryFixedArgs = "fixed-args"
	EntryFixed     = "fixed" // kind tool: the whole command line is the profile's argv

	ToolsDir        = ".tools"
	maxProfileBytes = 256 << 10
)

// EnvDirNames is the fixed set of environment folders a profile may name.
var EnvDirNames = []string{".venv", ".npm-env", ".conda-env"}

// InputRoots are the workspace folders a profile may allow inputs from (never .tools/, .process/, .git).
var InputRoots = []string{"src/", "build/", ".input/", ".agent/", ".output/"}

var (
	imageIDRe   = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	nameRe      = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
	inputNameRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
	envNameRe   = regexp.MustCompile(`^[A-Z_][A-Z0-9_]{0,63}$`)
	reservedEnv = map[string]bool{"OUT": true, "IN": true, "HOME": true}
)

type Input struct {
	PathUnder []string `yaml:"path_under" json:"path_under"`
	Glob      string   `yaml:"glob" json:"glob"`
	Multiple  bool     `yaml:"multiple" json:"multiple"`
	Optional  bool     `yaml:"optional" json:"optional"`
}

type Limits struct {
	Mem      string  `yaml:"mem"`
	Cpus     float64 `yaml:"cpus"`
	Pids     int64   `yaml:"pids"`
	TimeoutS int     `yaml:"timeout_s"`
	OutMax   string  `yaml:"out_max"`
}

type Profile struct {
	Name          string            `yaml:"name"`
	Kind          string            `yaml:"kind"`
	ImageID       string            `yaml:"image_id"`
	Ref           string            `yaml:"ref"`
	Entry         string            `yaml:"entry"`
	Argv          []string          `yaml:"argv"`
	FetchArgv     []string          `yaml:"fetch_argv"`
	EnvDirs       map[string]string `yaml:"env_dirs"`
	BuildDir      bool              `yaml:"build_dir"`
	RequiresBuild bool              `yaml:"requires_build"`
	Inputs        map[string]Input  `yaml:"inputs"`
	Env           map[string]string `yaml:"env"`
	TmpExec       bool              `yaml:"tmp_exec"`
	Limits        Limits            `yaml:"limits"`
	// Kind tool only (USS-009 "Tool runs"): the input filter and the typed parameters.
	InputPolicy string           `yaml:"input_policy"`
	Params      map[string]Param `yaml:"params"`

	// Derived by Validate.
	MemBytes    int64 `yaml:"-"`
	NanoCPUs    int64 `yaml:"-"`
	Pids        int64 `yaml:"-"`
	TimeoutS    int   `yaml:"-"`
	OutMaxBytes int64 `yaml:"-"`
}

// Loaded is one folder of .tools/ with its parse result.
type Loaded struct {
	Dir     string
	Profile *Profile // nil when Err is set
	Err     error
	Version string // from tool.json, may be empty
}

// LoadAll reads every .tools/*/profile.yml. A missing .tools/ is an empty store.
func LoadAll(ws *wsfs.Workspace) ([]Loaded, error) {
	names, err := ws.ReadDir(ToolsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []Loaded
	for _, n := range names {
		if strings.HasPrefix(n, ".") {
			continue
		}
		st, err := ws.Stat(ToolsDir + "/" + n)
		if err != nil || !st.IsDir() {
			continue
		}
		l := Loaded{Dir: n}
		data, err := ws.ReadFile(ToolsDir+"/"+n+"/profile.yml", maxProfileBytes)
		if err != nil {
			l.Err = fmt.Errorf("profile.yml: %w", err)
		} else if p, err := Parse(data, n); err != nil {
			l.Err = err
		} else {
			l.Profile = p
		}
		if tj, err := ws.ReadFile(ToolsDir+"/"+n+"/tool.json", maxProfileBytes); err == nil {
			l.Version = toolVersion(tj)
		}
		out = append(out, l)
	}
	return out, nil
}

// Find returns the folder named name, or nil.
func Find(list []Loaded, name string) *Loaded {
	for i := range list {
		if list[i].Dir == name {
			return &list[i]
		}
	}
	return nil
}

func toolVersion(data []byte) string {
	var tj struct {
		Version any `json:"version"`
	}
	if json.Unmarshal(data, &tj) != nil || tj.Version == nil {
		return ""
	}
	return fmt.Sprint(tj.Version)
}

// Parse decodes a profile, rejecting unknown fields, and validates it. dir is the folder name.
func Parse(data []byte, dir string) (*Profile, error) {
	var p Profile
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&p); err != nil {
		if err == io.EOF {
			return nil, errors.New("empty profile")
		}
		return nil, fmt.Errorf("profile.yml: %w", err)
	}
	if err := p.Validate(dir); err != nil {
		return nil, err
	}
	return &p, nil
}

// Validate checks the schema and the ceilings and fills the derived fields.
func (p *Profile) Validate(dir string) error {
	if !nameRe.MatchString(p.Name) {
		return fmt.Errorf("name %q is not valid", p.Name)
	}
	if p.Name != dir {
		return fmt.Errorf("name %q differs from folder %q", p.Name, dir)
	}
	switch p.Kind {
	case KindCode, KindBuild, KindInstall, KindTool:
	default:
		return fmt.Errorf("kind %q is not code, build, install or tool", p.Kind)
	}
	if !imageIDRe.MatchString(p.ImageID) {
		return fmt.Errorf("image_id %q is not sha256:<64 hex>", p.ImageID)
	}
	if err := p.validateEntry(); err != nil {
		return err
	}
	if err := p.validateDirs(); err != nil {
		return err
	}
	if err := p.validateInputs(); err != nil {
		return err
	}
	if err := p.validateTool(); err != nil {
		return err
	}
	for k := range p.Env {
		if !envNameRe.MatchString(k) {
			return fmt.Errorf("env: %q is not a variable name", k)
		}
		if reservedEnv[k] {
			return fmt.Errorf("env: %s is set by the tool", k)
		}
	}
	return p.validateLimits()
}

func (p *Profile) validateEntry() error {
	switch p.Kind {
	case KindCode:
		if p.Entry != EntryAgent {
			return errors.New("code profile needs entry: agent")
		}
		if len(p.Argv) > 0 || len(p.FetchArgv) > 0 {
			return errors.New("code profile has no argv or fetch_argv")
		}
	case KindBuild:
		if p.Entry != EntryFixedArgs || len(p.Argv) == 0 {
			return errors.New("build profile needs entry: fixed-args and argv")
		}
		if len(p.FetchArgv) > 0 {
			return errors.New("fetch_argv is for install only")
		}
	case KindInstall:
		if p.Entry != EntryFixedArgs || len(p.Argv) == 0 || len(p.FetchArgv) == 0 {
			return errors.New("install profile needs entry: fixed-args, argv and fetch_argv")
		}
	case KindTool:
		if p.Entry != EntryFixed || len(p.Argv) == 0 {
			return errors.New("tool profile needs entry: fixed and argv")
		}
		if len(p.FetchArgv) > 0 {
			return errors.New("fetch_argv is for install only")
		}
	}
	for _, a := range append(append([]string{}, p.Argv...), p.FetchArgv...) {
		if a == "" || strings.ContainsRune(a, 0) {
			return errors.New("argv entries must be non-empty strings")
		}
	}
	return nil
}

func (p *Profile) validateDirs() error {
	rw := 0
	for d, mode := range p.EnvDirs {
		if !contains(EnvDirNames, d) {
			return fmt.Errorf("env_dirs: %q is not one of %s", d, strings.Join(EnvDirNames, ", "))
		}
		switch mode {
		case "ro":
		case "rw":
			rw++
		default:
			return fmt.Errorf("env_dirs: %s mode %q is not ro or rw", d, mode)
		}
	}
	switch p.Kind {
	case KindInstall:
		if rw != 1 {
			return errors.New("install profile needs exactly one env dir with mode rw (the target)")
		}
		if p.BuildDir || p.RequiresBuild {
			return errors.New("install profile does not use build/")
		}
	case KindBuild:
		if rw != 0 {
			return errors.New("env dirs are read-only for build")
		}
		if !p.BuildDir {
			return errors.New("build profile needs build_dir: true")
		}
		if p.RequiresBuild {
			return errors.New("requires_build is for code only")
		}
	case KindCode:
		if rw != 0 {
			return errors.New("env dirs are read-only for code")
		}
		if p.RequiresBuild && !p.BuildDir {
			return errors.New("requires_build needs build_dir: true")
		}
	case KindTool:
		if len(p.EnvDirs) > 0 {
			return errors.New("env_dirs: a tool profile mounts no env dir (SSR-AGT-006)")
		}
		if p.RequiresBuild && !p.BuildDir {
			return errors.New("requires_build needs build_dir: true")
		}
	}
	return nil
}

func (p *Profile) validateInputs() error {
	if p.Kind == KindInstall && len(p.Inputs) > 0 {
		return errors.New("install profile has no inputs")
	}
	for n, in := range p.Inputs {
		if !inputNameRe.MatchString(n) {
			return fmt.Errorf("inputs: name %q is not valid", n)
		}
		if len(in.PathUnder) == 0 {
			return fmt.Errorf("inputs.%s: path_under is empty", n)
		}
		for _, pu := range in.PathUnder {
			c := path.Clean(pu)
			if path.IsAbs(c) || c == ".." || strings.HasPrefix(c, "../") {
				return fmt.Errorf("inputs.%s: path_under %q must be relative to the workspace", n, pu)
			}
			ok := false
			for _, r := range InputRoots {
				if c+"/" == r || strings.HasPrefix(c+"/", r) {
					ok = true
				}
			}
			if !ok {
				return fmt.Errorf("inputs.%s: path_under %q is not an allowed folder", n, pu)
			}
		}
		if in.Glob != "" {
			if _, err := path.Match(in.Glob, "x"); err != nil {
				return fmt.Errorf("inputs.%s: glob: %v", n, err)
			}
		}
	}
	return nil
}

// validateLimits requires every limit and rejects values above the compiled-in ceilings. SSR-AGE-008.
func (p *Profile) validateLimits() error {
	mem, err := ParseSize(p.Limits.Mem)
	if err != nil {
		return fmt.Errorf("limits.mem: %v", err)
	}
	if mem > limits.MaxMemoryBytes {
		return fmt.Errorf("limits.mem %s is above the ceiling %d bytes", p.Limits.Mem, limits.MaxMemoryBytes)
	}
	if !(p.Limits.Cpus > 0) {
		return errors.New("limits.cpus must be set and positive")
	}
	nano := int64(math.Round(p.Limits.Cpus * 1e9))
	if nano > limits.MaxNanoCPUs {
		return fmt.Errorf("limits.cpus %v is above the ceiling", p.Limits.Cpus)
	}
	if p.Limits.Pids <= 0 || p.Limits.Pids > limits.MaxPids {
		return fmt.Errorf("limits.pids %d must be in 1..%d", p.Limits.Pids, limits.MaxPids)
	}
	if p.Limits.TimeoutS <= 0 || p.Limits.TimeoutS > limits.MaxTimeoutS {
		return fmt.Errorf("limits.timeout_s %d must be in 1..%d", p.Limits.TimeoutS, limits.MaxTimeoutS)
	}
	out, err := ParseSize(p.Limits.OutMax)
	if err != nil {
		return fmt.Errorf("limits.out_max: %v", err)
	}
	if out > limits.MaxOutBytes {
		return fmt.Errorf("limits.out_max %s is above the ceiling %d bytes", p.Limits.OutMax, limits.MaxOutBytes)
	}
	p.MemBytes, p.NanoCPUs, p.Pids, p.TimeoutS, p.OutMaxBytes = mem, nano, p.Limits.Pids, p.Limits.TimeoutS, out
	return nil
}

// TargetEnvDir is the env dir an install profile writes (its rw env dir).
func (p *Profile) TargetEnvDir() string {
	for d, m := range p.EnvDirs {
		if m == "rw" {
			return d
		}
	}
	return ""
}

// SortedEnvDirs lists the profile's env dirs in a fixed order.
func (p *Profile) SortedEnvDirs() []string {
	var out []string
	for d := range p.EnvDirs {
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

// Substitute replaces the <execId> placeholder in an argv.
func Substitute(argv []string, execID string) []string {
	out := make([]string, len(argv))
	for i, a := range argv {
		out[i] = strings.ReplaceAll(a, "<execId>", execID)
	}
	return out
}

// ParseSize reads sizes such as 512m, 2g, 64k, 1048576 (optional trailing b).
func ParseSize(s string) (int64, error) {
	t := strings.ToLower(strings.TrimSpace(s))
	t = strings.TrimSuffix(t, "b")
	if t == "" {
		return 0, fmt.Errorf("size %q is empty or not a size", s)
	}
	mult := int64(1)
	switch t[len(t)-1] {
	case 'k':
		mult, t = 1<<10, t[:len(t)-1]
	case 'm':
		mult, t = 1<<20, t[:len(t)-1]
	case 'g':
		mult, t = 1<<30, t[:len(t)-1]
	}
	n, err := strconv.ParseInt(t, 10, 64)
	if err != nil || n <= 0 || n > math.MaxInt64/mult {
		return 0, fmt.Errorf("size %q is not a positive size like 512m", s)
	}
	return n * mult, nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// TarImageIDs reads, without loading the tarball, the image IDs a docker-save tarball can have:
// the config digest from manifest.json (classic image store ID) and, when index.json is present,
// the digest of each top-level manifests entry (containerd image store ID). One pass over the tar.
func TarImageIDs(r io.Reader) ([]string, error) {
	hexRe := regexp.MustCompile(`^[0-9a-f]{64}$`)
	digestRe := regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	tr := tar.NewReader(r)
	var ids []string
	add := func(id string) {
		if !contains(ids, id) {
			ids = append(ids, id)
		}
	}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch path.Clean(h.Name) {
		case "manifest.json":
			data, err := io.ReadAll(io.LimitReader(tr, 1<<20))
			if err != nil {
				return nil, err
			}
			var m []struct {
				Config string `json:"Config"`
			}
			if err := json.Unmarshal(data, &m); err != nil || len(m) == 0 {
				return nil, errors.New("unreadable manifest.json in image.tar")
			}
			cfg := m[0].Config
			hex := strings.TrimSuffix(strings.TrimPrefix(cfg, "blobs/sha256/"), ".json")
			if !hexRe.MatchString(hex) {
				return nil, fmt.Errorf("unexpected config name %q in image.tar", cfg)
			}
			add("sha256:" + hex)
		case "index.json":
			data, err := io.ReadAll(io.LimitReader(tr, 1<<20))
			if err != nil {
				return nil, err
			}
			var ix struct {
				Manifests []struct {
					Digest string `json:"digest"`
				} `json:"manifests"`
			}
			if err := json.Unmarshal(data, &ix); err != nil {
				return nil, errors.New("unreadable index.json in image.tar")
			}
			for _, d := range ix.Manifests {
				if !digestRe.MatchString(d.Digest) {
					return nil, fmt.Errorf("unexpected digest %q in index.json of image.tar", d.Digest)
				}
				add(d.Digest)
			}
		}
	}
	if len(ids) == 0 {
		return nil, errors.New("no image ID in manifest.json or index.json of image.tar")
	}
	return ids, nil
}

// TarHasImageID reports whether id is one of the image IDs TarImageIDs finds in the tarball.
func TarHasImageID(r io.Reader, id string) (bool, error) {
	ids, err := TarImageIDs(r)
	if err != nil {
		return false, err
	}
	return contains(ids, id), nil
}
