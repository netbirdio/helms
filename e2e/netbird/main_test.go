//go:build e2e

// Package netbird_test installs the netbird chart into a Kind cluster and
// connects real NetBird peers through it. Start with TestChart in
// chart_test.go; e2e/README.md describes the setup and the knobs,
// e2e/shared holds the steps and e2e/framework/netbird the plumbing.
package netbird_test

import (
	"testing"

	"github.com/netbirdio/helms/e2e/framework/netbird"
	"github.com/netbirdio/helms/e2e/shared"
)

// env is the environment TestMain brought up, shared by every test.
var env *netbird.Env

func TestMain(m *testing.M) {
	shared.Main(m, netbird.Config{Chart: netbird.ChartCommunity, ClusterName: "netbird-chart-e2e"}, &env)
}
