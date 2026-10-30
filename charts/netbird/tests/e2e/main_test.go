//go:build e2e

// Package e2e installs the chart into a Kind cluster and connects real NetBird
// peers through it. Start with TestChart in chart_test.go; README.md describes
// the setup and the knobs, and the framework package holds the plumbing.
package e2e

import (
	"context"
	"log"
	"os"
	"testing"

	"github.com/netbirdio/helms/charts/netbird/tests/e2e/framework"
)

// env is the environment TestMain brought up, shared by every test.
var env *framework.Env

// TestMain brings the environment up once, runs the tests against it, and tears
// it down. Logs are collected when anything fails.
func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	ctx, cancel := context.WithTimeout(context.Background(), framework.BootstrapTimeout)
	e, err := framework.Bootstrap(ctx)
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
