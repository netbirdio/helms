//go:build e2e

// Package netbird_test installs the netbird chart into a Kind cluster and
// connects real NetBird peers through it. Start with TestChart in
// chart_test.go; e2e/README.md describes the setup and the knobs, and
// e2e/framework/netbird holds the plumbing.
package netbird_test

import (
	"context"
	"log"
	"os"
	"testing"

	"github.com/netbirdio/helms/e2e/framework/netbird"
)

// env is the environment TestMain brought up, shared by every test.
var env *netbird.Env

// TestMain brings the environment up once, runs the tests against it, and tears
// it down. Logs are collected when anything fails.
func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	ctx, cancel := context.WithTimeout(context.Background(), netbird.BootstrapTimeout)
	e, err := netbird.Bootstrap(ctx)
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

	env = e
	code := m.Run()
	if code != 0 {
		e.CollectArtifacts()
	}
	return code
}
