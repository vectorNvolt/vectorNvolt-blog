// Package app wires the command line to the sequence: configuration from the environment,
// signal handling, and the list and sweep subcommands. SFR-AGE-018 (list), SFR-AGE-019 (sweep),
// SFR-AGT-012 (list the inputs and params of tool profiles).
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"text/tabwriter"
	"time"

	"agent-exec/internal/cli"
	"agent-exec/internal/engine"
	"agent-exec/internal/profile"
	"agent-exec/internal/sequence"
	"agent-exec/internal/wsfs"
)

const (
	// DefaultWorkspace is the agent workspace inside the agent container.
	DefaultWorkspace = "/workspace"
	// EnvWorkspace and EnvDockerHost exist for tests; the defaults are the production values.
	EnvWorkspace  = "AGENT_EXEC_WORKSPACE"
	EnvDockerHost = "AGENT_EXEC_DOCKER_HOST"
)

// Main runs one invocation and returns the exit code.
func Main(args []string, stdout, stderr io.Writer, getenv func(string) string) int {
	req, err := cli.Parse(args)
	if err != nil {
		fmt.Fprintf(stderr, "%s: usage: %v\n", sequence.Prog, err)
		return sequence.ExitTool
	}
	root := getenv(EnvWorkspace)
	if root == "" {
		root = DefaultWorkspace
	}
	eng, err := engine.NewClient(getenv(EnvDockerHost))
	if err != nil {
		fmt.Fprintf(stderr, "%s: setup: %v\n", sequence.Prog, err)
		return sequence.ExitTool
	}
	cancel := make(chan struct{})
	sigc := make(chan os.Signal, 2)
	signal.Notify(sigc, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(sigc)
	go func() {
		<-sigc
		close(cancel)
	}()

	d := &sequence.Deps{
		WS: wsfs.New(root), Engine: eng, Stdout: stdout, Stderr: stderr, Cancel: cancel,
	}
	switch req.Sub {
	case "list":
		return List(d, req.JSON)
	case "sweep":
		return sequence.Sweep(d, req.SweepKey, req.SweepVal).Exit
	default:
		return sequence.Execute(d, req).Exit
	}
}

// Entry is one row of `list`.
type Entry struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Version     string `json:"version"`
	ImageID     string `json:"image_id"`
	TarPresent  bool   `json:"image_tar_present"`
	TarMatches  bool   `json:"image_tar_matches"`
	ImageLoaded bool   `json:"image_loaded"`
	Valid       bool   `json:"valid"`
	Error       string `json:"error,omitempty"`
	// Kind tool only (USC-009 alt-4): what a correct `tool` call needs.
	Inputs      []EntryInput             `json:"inputs,omitempty"`
	Params      map[string]profile.Param `json:"params,omitempty"`
	InputPolicy string                   `json:"input_policy,omitempty"`
}

// EntryInput is one declared input of a tool profile.
type EntryInput struct {
	Name string `json:"name"`
	profile.Input
}

// Entries reads .tools/ and reports every profile. It creates nothing and needs no engine
// (an unreachable engine only means image_loaded is false). SFR-AGE-018.
func Entries(ctx context.Context, ws *wsfs.Workspace, eng engine.Engine) ([]Entry, error) {
	list, err := profile.LoadAll(ws)
	if err != nil {
		return nil, err
	}
	out := []Entry{}
	for _, l := range list {
		e := Entry{Name: l.Dir, Version: l.Version, Valid: l.Err == nil}
		if l.Err != nil {
			e.Error = l.Err.Error()
		}
		if l.Profile != nil {
			e.Name, e.Kind, e.ImageID = l.Profile.Name, l.Profile.Kind, l.Profile.ImageID
			if l.Profile.Kind == profile.KindTool {
				for _, n := range l.Profile.SortedInputNames() {
					e.Inputs = append(e.Inputs, EntryInput{Name: n, Input: l.Profile.Inputs[n]})
				}
				e.Params = l.Profile.Params
				e.InputPolicy = l.Profile.InputPolicy
			}
		}
		tarRel := profile.ToolsDir + "/" + l.Dir + "/image.tar"
		if f, err := ws.OpenRegular(tarRel); err == nil {
			e.TarPresent = true
			if ok, err := profile.TarHasImageID(f, e.ImageID); err == nil && ok {
				e.TarMatches = true
			}
			f.Close()
		}
		if eng != nil && e.ImageID != "" {
			cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			if img, err := eng.ImageInspect(cctx, e.ImageID); err == nil && img.ID == e.ImageID {
				e.ImageLoaded = true
			}
			cancel()
		}
		out = append(out, e)
	}
	return out, nil
}

// List prints the profiles, as a table or as JSON.
func List(d *sequence.Deps, asJSON bool) int {
	entries, err := Entries(context.Background(), d.WS, d.Engine)
	if err != nil {
		fmt.Fprintf(d.Stderr, "%s: setup: cannot read %s: %v\n", sequence.Prog, profile.ToolsDir, err)
		return sequence.ExitTool
	}
	if asJSON {
		enc := json.NewEncoder(d.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(entries); err != nil {
			return sequence.ExitTool
		}
		return 0
	}
	tw := tabwriter.NewWriter(d.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tKIND\tVERSION\tIMAGE_ID\tTAR\tTAR_MATCH\tLOADED\tVALID")
	for _, e := range entries {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%v\t%v\t%v\t%v\n", e.Name, e.Kind, e.Version, e.ImageID, e.TarPresent, e.TarMatches, e.ImageLoaded, e.Valid)
		if e.Error != "" {
			fmt.Fprintf(d.Stderr, "%s: profile %s: %s\n", sequence.Prog, e.Name, e.Error)
		}
	}
	tw.Flush()
	return 0
}
