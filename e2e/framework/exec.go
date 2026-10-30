//go:build e2e

package framework

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const (
	envKeep         = "NB_E2E_KEEP"
	envArtifactsDir = "NB_E2E_ARTIFACTS_DIR"
)

// Command runs a CLI and returns its output, or an error carrying it.
func Command(ctx context.Context, name string, args ...string) (string, error) {
	return runCommand(exec.CommandContext(ctx, name, args...))
}

func runCommand(cmd *exec.Cmd) (string, error) {
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s: %w\n%s", strings.Join(cmd.Args, " "), err, out)
	}
	return string(out), nil
}

// ChartDir returns the directory of the repository's chart name, wherever go
// test runs from.
func ChartDir(name string) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "charts", name)
}

// Keep reports whether NB_E2E_KEEP asks to leave the environment running.
func Keep() bool {
	return EnvBool(envKeep)
}

// ArtifactsDir is where NB_E2E_ARTIFACTS_DIR asks for logs on failure, or ""
// when it does not.
func ArtifactsDir() string {
	return os.Getenv(envArtifactsDir)
}

// EnvOr reads a knob, or returns fallback when it is unset.
func EnvOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

// EnvBool reads a boolean knob. An unparsable value is reported rather than
// silently read as false.
func EnvBool(name string) bool {
	val := os.Getenv(name)
	if val == "" {
		return false
	}
	b, err := strconv.ParseBool(val)
	if err != nil {
		log.Printf("e2e: ignoring %s=%q: %v", name, val, err)
		return false
	}
	return b
}
