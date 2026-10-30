//go:build e2e

package framework

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// command runs a CLI and returns its output, or an error carrying it.
func command(ctx context.Context, name string, args ...string) (string, error) {
	return runCommand(exec.CommandContext(ctx, name, args...))
}

func runCommand(cmd *exec.Cmd) (string, error) {
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s: %w\n%s", strings.Join(cmd.Args, " "), err, out)
	}
	return string(out), nil
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

// envBool reads a boolean knob. An unparsable value is reported rather than
// silently read as false.
func envBool(name string) bool {
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
