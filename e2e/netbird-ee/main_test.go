//go:build e2e

// Package netbirdee_test installs the netbird-ee chart into a Kind cluster,
// fresh or by migrating a netbird release when NB_E2E_FROM_CHART=netbird, and
// connects real NetBird peers through it. Start with TestChart in
// chart_test.go; e2e/README.md describes the setup and the knobs.
package netbirdee_test

import (
	"testing"

	"github.com/netbirdio/helms/e2e/framework"
	"github.com/netbirdio/helms/e2e/framework/netbird"
	"github.com/netbirdio/helms/e2e/shared"
)

// env is the environment TestMain brought up, shared by every test.
var env *netbird.Env

func TestMain(m *testing.M) {
	shared.Main(m, netbird.Config{
		Chart:       netbird.ChartEnterprise,
		FromChart:   framework.EnvOr("NB_E2E_FROM_CHART", netbird.ChartEnterprise),
		ClusterName: "netbird-ee-chart-e2e",
	}, &env)
}
