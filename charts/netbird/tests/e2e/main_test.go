//go:build e2e

// Package e2e installs the chart into a Kind cluster and connects real NetBird
// peers through it, so it needs nothing but Docker, Go and helm:
//
//	task chart:e2e
//
// The chart is installed from examples/kind, with only its hostnames swapped
// for ones the suite controls, behind ingress-nginx. Peers run in Docker
// containers on Kind's network, outside the cluster, and reach management,
// signal and the relay through the ingress the way users' peers do. The suite
// talks to the management API through the same ingress, with a token from the
// unauthenticated POST /api/setup; nothing is written into the store directly.
//
// The backends are installed at NB_E2E_FROM_VERSION, checked, and then upgraded
// to the chart's defaults and checked again with the same peers.
//
// Environment knobs:
//
//	NB_E2E_KIND_NODE_IMAGE  the Kind node image, which picks the Kubernetes version
//	NB_E2E_FROM_VERSION     the backend version installed first (default 0.74.0);
//	                        the chart's default skips the upgrade
//	NB_E2E_ROOTLESS         layer examples/rootless into a namespace that
//	                        enforces the restricted Pod Security Standard
//	NB_E2E_CLIENT_IMAGE     the NetBird client the peers run (default: the
//	                        release of NB_E2E_FROM_VERSION)
//	NB_E2E_KEEP             leave the cluster and the peers running
//	NB_E2E_ARTIFACTS_DIR    on failure, write the cluster, chart and peer logs here
package e2e

import (
	"context"
	"log"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	ctx, cancel := context.WithTimeout(context.Background(), bootstrapTimeout)
	env, err := bootstrap(ctx)
	cancel()
	if env != nil {
		defer env.teardown()
	}
	if err != nil {
		log.Printf("e2e: %v", err)
		if env != nil {
			env.logDiagnostics()
			env.collectArtifacts()
		}
		return 1
	}

	testEnv = env
	code := m.Run()
	if code != 0 {
		env.collectArtifacts()
	}
	return code
}

// requireEnv returns the environment TestMain brought up.
func requireEnv(t *testing.T) *environment {
	t.Helper()
	if testEnv == nil {
		t.Fatal("the e2e environment is not running")
	}
	return testEnv
}
