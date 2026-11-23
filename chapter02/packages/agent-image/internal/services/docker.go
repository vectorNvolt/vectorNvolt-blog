// IP-DOCKER-ENGINE adapter: docker CLI calls on the local engine (root inside the VM).
// Only build, load, inspect, history and version are offered; no run/create/start and no image removal exists (SSR-IMG-012).
package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"agent-image/internal/config"
)

// Docker offers typed operations on the local docker engine.
type Docker struct {
	Host *Host
	Live io.Writer // receives the live docker build output (nil: quiet)
}

func NewDocker(h *Host) *Docker { return &Docker{Host: h} }

func (d *Docker) run(ctx context.Context, args ...string) ([]byte, error) {
	return d.Host.Exec(ctx, append([]string{config.DockerExe}, args...)...)
}

// Version proves the engine is reachable (engine_check).
func (d *Docker) Version(ctx context.Context) error {
	_, err := d.run(ctx, "version")
	return err
}

// Build builds ref from dockerfile (a name at the tar root) and the tar build context on stdin.
// --progress=plain gives line-oriented output that is streamed live to d.Live (the only call that is).
// extra build options (e.g. "--label", "k=v" or "--build-arg", "K=v") go between -f and -t.
func (d *Docker) Build(ctx context.Context, dockerfile, ref string, contextTar []byte, extra ...string) error {
	args := append([]string{config.DockerExe}, "build", "--progress=plain", "-f", dockerfile)
	args = append(args, extra...)
	args = append(args, "-t", ref, "-")
	_, err := d.Host.ExecStream(ctx, contextTar, d.Live, args...)
	return err
}

// Load feeds a docker save archive to `docker load` on stdin; its output goes to d.Live.
func (d *Docker) Load(ctx context.Context, archive []byte) error {
	_, err := d.Host.ExecStream(ctx, archive, d.Live, config.DockerExe, "load")
	return err
}

// LabelOf returns the value of label on ref and whether ref exists. A missing image is reported
// only when the engine says "no such image"; any other failure is an error. The label is read from
// the Config.Labels JSON (no quote characters in the command line).
func (d *Docker) LabelOf(ctx context.Context, ref, label string) (string, bool, error) {
	labels, err := d.ImageLabels(ctx, ref)
	if err != nil {
		var we *HostError
		if errors.As(err, &we) && we.Err == nil && strings.Contains(strings.ToLower(we.Stderr), "no such image") {
			return "", false, nil
		}
		return "", false, err
	}
	return labels[label], true, nil
}

// Inspect fails when ref does not exist.
func (d *Docker) Inspect(ctx context.Context, ref string) error {
	_, err := d.run(ctx, "image", "inspect", ref)
	return err
}

// ImageExists reports the image absent only when the engine explicitly says "no such image"
// (SFR-IMG-006); any other failure, including a failure to run docker, is returned as an error.
func (d *Docker) ImageExists(ctx context.Context, ref string) (bool, error) {
	err := d.Inspect(ctx, ref)
	if err == nil {
		return true, nil
	}
	var we *HostError
	if errors.As(err, &we) && we.Err == nil && strings.Contains(strings.ToLower(we.Stderr), "no such image") {
		return false, nil
	}
	return false, err
}

// ImageEnv returns the Config.Env entries ("NAME=value") of ref.
func (d *Docker) ImageEnv(ctx context.Context, ref string) ([]string, error) {
	out, err := d.run(ctx, "image", "inspect", "--format", "{{json .Config.Env}}", ref)
	if err != nil {
		return nil, err
	}
	var env []string
	if err := json.Unmarshal(bytes.TrimSpace(out), &env); err != nil {
		return nil, fmt.Errorf("cannot parse Config.Env of %s: %w", ref, err)
	}
	return env, nil
}

// ImageLabels returns the Config.Labels of ref.
func (d *Docker) ImageLabels(ctx context.Context, ref string) (map[string]string, error) {
	out, err := d.run(ctx, "image", "inspect", "--format", "{{json .Config.Labels}}", ref)
	if err != nil {
		return nil, err
	}
	var labels map[string]string
	if err := json.Unmarshal(bytes.TrimSpace(out), &labels); err != nil {
		return nil, fmt.Errorf("cannot parse Config.Labels of %s: %w", ref, err)
	}
	return labels, nil
}

// History returns the untruncated CreatedBy line of every layer of ref, newest first.
func (d *Docker) History(ctx context.Context, ref string) ([]string, error) {
	out, err := d.run(ctx, "history", "--no-trunc", "--format", "{{.CreatedBy}}", ref)
	if err != nil {
		return nil, err
	}
	text := strings.TrimRight(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n")
	if text == "" {
		return nil, nil
	}
	return strings.Split(text, "\n"), nil
}
