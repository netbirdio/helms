//go:build e2e

// Package ha_test installs the netbird chart's enterprise edition with
// examples/ha, two replicas of management and signal over Postgres, Redis and
// a 3-node NATS cluster the suite installs, and connects real NetBird peers
// through it. Start with TestChart in chart_test.go; e2e/README.md describes
// the setup and the knobs.
package ha_test

import (
	"testing"

	"github.com/netbirdio/helms/e2e/framework/netbird"
	"github.com/netbirdio/helms/e2e/shared"
)

// env is the environment TestMain brought up, shared by every test.
var env *netbird.Env

func TestMain(m *testing.M) {
	shared.Main(m, netbird.Config{
		Edition:     netbird.EditionEnterprise,
		HA:          true,
		ClusterName: "netbird-ha-chart-e2e",
	}, &env)
}
