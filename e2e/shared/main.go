//go:build e2e

package shared

import (
	"context"
	"log"
	"os"
	"testing"

	"github.com/netbirdio/helms/e2e/framework/netbird"
)

// Main is a test package's TestMain: it brings cfg's environment up once,
// stores it in env, runs the tests, and tears it down. Logs are collected when
// anything fails.
func Main(m *testing.M, cfg netbird.Config, env **netbird.Env) {
	os.Exit(run(m, cfg, env))
}

func run(m *testing.M, cfg netbird.Config, env **netbird.Env) int {
	ctx, cancel := context.WithTimeout(context.Background(), netbird.BootstrapTimeout)
	e, err := netbird.Bootstrap(ctx, cfg)
	cancel()
	if e != nil {
		defer e.Teardown()
	}
	if err != nil {
		log.Printf("e2e: %v", err)
		if e != nil {
			e.LogDiagnostics()
			e.CollectArtifacts()
		}
		return 1
	}

	*env = e
	code := m.Run()
	if code != 0 {
		e.CollectArtifacts()
	}
	return code
}
