//go:build e2e

package netbirdee_test

import (
	"strings"
	"testing"

	"github.com/netbirdio/helms/e2e/shared"
)

// TestChart walks one installation through its life. When NB_E2E_FROM_CHART
// is netbird, the release starts on the netbird chart and the upgrade step is
// the migration to netbird-ee; the enterprise checks run after it either way.
func TestChart(t *testing.T) {
	l := &shared.Lifecycle{Env: env, BackendImage: backendImage}
	shared.Run(t,
		shared.Step{Name: "the dashboard, the API and the IdP answer through the ingress", Run: l.HTTPRoutes},
		shared.Step{Name: "peers connect directly", Run: l.DirectConnection},
		shared.Step{Name: "an upgrade to netbird-ee keeps the peers", Run: l.Upgrade},
		shared.Step{Name: "the enterprise images run", Run: testEnterpriseImages},
		shared.Step{Name: "management runs the enterprise server", Run: testEnterpriseServer},
		shared.Step{Name: "peers connect through the relay", Run: l.RelayedConnection},
		shared.Step{Name: "management keeps its peers across a restart", Run: l.ManagementRestart},
	)
}

// testEnterpriseImages checks netbird-ee swapped in the enterprise images and
// they came up: the -cloud management, signal and dashboard, and the
// community relay, which has no enterprise build.
func testEnterpriseImages(t *testing.T) {
	images := map[string]string{"netbird-dashboard": "ghcr.io/netbirdio/dashboard-cloud:v2.96.0"}
	if env.Rootless {
		images["netbird-dashboard"] += "-rootless"
	}
	for _, component := range []string{"management", "signal", "relay"} {
		images["netbird-"+component] = backendImage(component, env.DefaultVersion)
	}
	for name, image := range images {
		if err := env.WaitDeploymentImage(t.Context(), name, image); err != nil {
			t.Fatal(err)
		}
	}
}

// testEnterpriseServer checks management took the enterprise code path, which
// the image name alone does not show.
func testEnterpriseServer(t *testing.T) {
	logs, err := env.ManagementLogs(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logs, "Starting CloudServer") {
		t.Fatal(`management never logged "Starting CloudServer"`)
	}
}

// relayImage is netbird-ee's relay. The suite leaves it at the chart's
// default, since there is no enterprise relay to version with the backends.
const relayImage = "netbirdio/relay:0.76.2"

func backendImage(component, version string) string {
	if component == "relay" {
		return relayImage
	}
	return "ghcr.io/netbirdio/" + component + "-cloud:" + version
}
