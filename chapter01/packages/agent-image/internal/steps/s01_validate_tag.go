// SFR-IMG-001 (USC-003 step 1, err-1): validate the invocation (including the build files in the
// current directory) before any engine call.
package steps

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"agent-image/internal/config"
)

// Usage is printed for an invalid tag, an unknown flag or extra arguments.
const Usage = "usage: create_agent_image [--tag <tag>]\n" +
	"  tag defaults to " + config.DefaultTag + " and must match " + config.TagPattern

var tagRe = regexp.MustCompile(config.TagPattern)

// ParseArgs parses the command line (without the program name) and returns the tag.
// Unknown flags and positional arguments are errors.
func ParseArgs(args []string) (string, error) {
	fs := flag.NewFlagSet("create_agent_image", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	tag := fs.String("tag", config.DefaultTag, "")
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	if fs.NArg() > 0 {
		return "", errors.New("unexpected arguments")
	}
	return *tag, nil
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

// checkBuildContext requires every build file as a regular file in the current directory.
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
	env.ContextDir = dir
	env.Log.Infof("build context directory: %s", dir)
	return nil
}
