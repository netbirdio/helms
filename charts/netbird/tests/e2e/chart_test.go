//go:build e2e

package e2e

import (
	"testing"

	"github.com/netbirdio/helms/charts/netbird/tests/e2e/framework"
)

// TestChart walks one installation through its life. The steps run in order and
// stop at the first failure: later steps reuse the peers earlier ones start, and
// the backends are upgraded under them halfway through.
func TestChart(t *testing.T) {
	steps := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"the dashboard, the API and the IdP answer through the ingress", testHTTPRoutes},
		{"peers connect directly", testDirectConnection},
		{"an upgrade to the chart's backends keeps the peers", testUpgrade},
		{"peers connect through the relay", testRelayedConnection},
		{"management keeps its peers across a restart", testManagementRestart},
	}
	for _, step := range steps {
		if !t.Run(step.name, step.run) {
			t.FailNow()
		}
	}
}

// The peers the steps start, in the order they start them.
var alpha, beta, gamma, delta *framework.Peer

// testHTTPRoutes checks what a browser uses: the dashboard, the API and the
// embedded IdP, all through the ingress.
func testHTTPRoutes(t *testing.T) {
	requireHTTPRoutes(t)
}

// testDirectConnection starts two peers and checks they connect directly,
// which needs management to enroll them and signal to exchange candidates.
func testDirectConnection(t *testing.T) {
	alpha, beta = startPeers(t, false, "alpha", "beta")
	requireConnected(t, alpha, beta, connP2P)
	requireConnected(t, beta, alpha, connP2P)
}

// testUpgrade upgrades the backends from FromVersion to the chart's defaults
// and checks the peers survive it.
func testUpgrade(t *testing.T) {
	if env.FromVersion == env.DefaultVersion {
		t.Skipf("installed at the chart's default %s; nothing to upgrade", env.DefaultVersion)
	}
	if err := env.InstallChart(t.Context(), env.DefaultVersion); err != nil {
		t.Fatal(err)
	}
	for _, component := range []string{"management", "signal", "relay"} {
		image := "netbirdio/" + component + ":" + env.DefaultVersion
		if err := env.WaitDeploymentImage(t.Context(), "netbird-"+component, image); err != nil {
			t.Fatal(err)
		}
	}
	requireHTTPRoutes(t)
	requirePeersListed(t, alpha, beta)
	requireConnected(t, alpha, beta, connP2P)
}

// testRelayedConnection starts two peers that may only use the relay and checks
// they connect through it.
//
// It runs after the upgrade: a 0.74.0 management sends the peers it updates when
// another peer joins no relay URLs, and clients then drop the relay.
func testRelayedConnection(t *testing.T) {
	gamma, delta = startPeers(t, true, "gamma", "delta")
	requireConnected(t, gamma, delta, connRelayed)
	requireConnected(t, delta, gamma, connRelayed)
}

// testManagementRestart deletes management's pod and checks every peer is still
// listed, so the store persisted, and the connections come back.
func testManagementRestart(t *testing.T) {
	if err := env.RestartManagement(t.Context()); err != nil {
		t.Fatal(err)
	}
	requirePeersListed(t, alpha, beta, gamma, delta)
	requireConnected(t, alpha, beta, connP2P)
	requireConnected(t, gamma, delta, connRelayed)
}

// startPeers starts two peers with one setup key; forceRelay keeps them off
// direct connections.
func startPeers(t *testing.T, forceRelay bool, a, b string) (*framework.Peer, *framework.Peer) {
	t.Helper()
	peers, err := env.StartPeers(t.Context(), forceRelay, a, b)
	if err != nil {
		t.Fatal(err)
	}
	return peers[0], peers[1]
}
