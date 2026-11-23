// SFR-IMG-001 (USC-003 step 1, err-1): validate the invocation (including the build files and the
// agent_graph package directory in the current directory) before any engine call.
package steps

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"

	"agent-image/internal/config"
	"agent-image/internal/services"
)

// Usage is printed for an invalid tag, an unknown flag or extra arguments.
const Usage = "usage: create_agent_image [--tag <tag>] [--rebuild-base]\n" +
	"  tag defaults to " + config.DefaultTag + " and must match " + config.TagPattern + "\n" +
	"  --rebuild-base builds the " + config.BaseImageRepo + " image even if it is present or an archive exists"

var tagRe = regexp.MustCompile(config.TagPattern)

// ParseArgs parses the command line (without the program name) and returns the tag.
// Unknown flags and positional arguments are errors. The --rebuild-base flag is accepted and ignored here;
// use ParseOptions to read it.
func ParseArgs(args []string) (string, error) {
	tag, _, err := ParseOptions(args)
	return tag, err
}

// ParseOptions is ParseArgs that also returns the --rebuild-base flag.
func ParseOptions(args []string) (tag string, rebuildBase bool, err error) {
	fs := flag.NewFlagSet("create_agent_image", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	t := fs.String("tag", config.DefaultTag, "")
	rb := fs.Bool("rebuild-base", false, "")
	if err := fs.Parse(args); err != nil {
		return "", false, err
	}
	if fs.NArg() > 0 {
		return "", false, errors.New("unexpected arguments")
	}
	return *t, *rb, nil
}

func ValidateTag(_ context.Context, env *Env) *Failure {
	if !tagRe.MatchString(env.Tag) {
		return &Failure{ExitCode: ExitPrerequisite, ExitState: StateNotCreated, Message: Usage}
	}
	if f := checkBuildContext(env); f != nil {
		return f
	}
	env.Image = config.ImageRef(env.Tag)
	return nil
}

// checkBuildContext requires every build file as a regular file in the current directory, then
// the agent_graph package directory holding at least the files of config.AgentGraphRequired.
func checkBuildContext(env *Env) *Failure {
	dir, err := os.Getwd()
	if err != nil {
		return fail(ExitPrerequisite, StateNotCreated, "cannot determine the current directory", err)
	}
	for _, name := range config.BuildContextFiles {
		if fi, err := os.Stat(filepath.Join(dir, name)); err != nil || !fi.Mode().IsRegular() {
			return &Failure{ExitCode: ExitPrerequisite, ExitState: StateNotCreated,
				Message: fmt.Sprintf("build file %s not found as a regular file in %s", name, dir)}
		}
	}
	for _, name := range config.BinFiles {
		if fi, err := os.Stat(filepath.Join(dir, filepath.FromSlash(name))); err != nil || !fi.Mode().IsRegular() || fi.Size() == 0 {
			return &Failure{ExitCode: ExitPrerequisite, ExitState: StateNotCreated,
				Message: fmt.Sprintf("build file %s not found as a non-empty regular file in %s; build it in P8 inside the VM and copy it into the bin folder of the build context", name, dir)}
		}
	}
	// Dockerfile.base is not part of the agent build-context tar; it is hashed here and sent alone to the base build.
	basePath := filepath.Join(dir, config.BaseDockerfileName)
	if fi, err := os.Stat(basePath); err != nil || !fi.Mode().IsRegular() || fi.Size() == 0 {
		return &Failure{ExitCode: ExitPrerequisite, ExitState: StateNotCreated,
			Message: fmt.Sprintf("build file %s not found as a non-empty regular file in %s", config.BaseDockerfileName, dir)}
	}
	baseData, err := os.ReadFile(basePath)
	if err != nil {
		return fail(ExitPrerequisite, StateNotCreated, "cannot read "+config.BaseDockerfileName, err)
	}
	env.BaseHash = services.BaseHash(baseData)
	env.BaseImage = config.BaseImageRef(env.BaseHash)
	pkgFiles, f := checkAgentGraph(dir)
	if f != nil {
		return f
	}
	env.ContextDir = dir
	env.ContextFiles = append(append([]string(nil), config.BuildContextFiles...), config.BinFiles...)
	env.ContextFiles = append(env.ContextFiles, pkgFiles...)
	env.Log.Infof("build context directory: %s", dir)
	return nil
}

// checkAgentGraph requires config.AgentGraphDir as a directory in dir and returns its build files.
func checkAgentGraph(dir string) ([]string, *Failure) {
	if fi, err := os.Stat(filepath.Join(dir, config.AgentGraphDir)); err != nil || !fi.IsDir() {
		return nil, &Failure{ExitCode: ExitPrerequisite, ExitState: StateNotCreated,
			Message: fmt.Sprintf("build directory %s not found as a directory in %s", config.AgentGraphDir, dir)}
	}
	files, err := services.PackageFiles(dir, config.AgentGraphDir)
	if err != nil {
		return nil, fail(ExitPrerequisite, StateNotCreated, "cannot read the build context", err)
	}
	have := make(map[string]bool, len(files))
	for _, f := range files {
		have[f] = true
	}
	for _, name := range config.AgentGraphRequired {
		if p := path.Join(config.AgentGraphDir, name); !have[p] {
			return nil, &Failure{ExitCode: ExitPrerequisite, ExitState: StateNotCreated,
				Message: fmt.Sprintf("build file %s not found as a regular file in %s", p, dir)}
		}
	}
	return files, nil
}
