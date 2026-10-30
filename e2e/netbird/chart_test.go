//go:build e2e

package netbird_test

import (
	"testing"

	"github.com/netbirdio/helms/e2e/shared"
)

// TestChart walks one installation through its life, with the backends
// upgraded from NB_E2E_FROM_VERSION to the chart's defaults halfway through.
func TestChart(t *testing.T) {
	l := &shared.Lifecycle{Env: env, BackendImage: backendImage}
	shared.Run(t,
		shared.Step{Name: "the dashboard, the API and the IdP answer through the ingress", Run: l.HTTPRoutes},
		shared.Step{Name: "peers connect directly", Run: l.DirectConnection},
		shared.Step{Name: "an upgrade to the chart's backends keeps the peers", Run: l.Upgrade},
		shared.Step{Name: "peers connect through the relay", Run: l.RelayedConnection},
		shared.Step{Name: "management keeps its peers across a restart", Run: l.ManagementRestart},
	)
}

func backendImage(component, version string) string {
	return "netbirdio/" + component + ":" + version
}
