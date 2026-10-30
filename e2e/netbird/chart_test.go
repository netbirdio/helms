//go:build e2e

package netbird_test

import (
	"bytes"
	"maps"
	"testing"

	"github.com/netbirdio/helms/e2e/framework/netbird"
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
		{"netbird-ee runs the enterprise images", testEnterpriseImages},
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
var alpha, beta, gamma, delta *netbird.Peer

// testHTTPRoutes checks what a browser uses: the dashboard, the API and the
// embedded IdP, all through the ingress.
func testHTTPRoutes(t *testing.T) {
	requireHTTPRoutes(t)
}

// testEnterpriseImages checks netbird-ee swapped in the enterprise images and
// they came up: the -cloud management, signal and dashboard, and the
// community relay, which has no enterprise build.
func testEnterpriseImages(t *testing.T) {
	if !env.Enterprise() {
		t.Skip("netbird-ee only")
	}
	images := map[string]string{"netbird-dashboard": "ghcr.io/netbirdio/dashboard-cloud:v2.96.0"}
	if env.Rootless {
		images["netbird-dashboard"] += "-rootless"
	}
	for _, component := range []string{"management", "signal", "relay"} {
		images["netbird-"+component] = backendImage(component, env.FromVersion)
	}
	for name, image := range images {
		if err := env.WaitDeploymentImage(t.Context(), name, image); err != nil {
			t.Fatal(err)
		}
	}
}

// backendImage is the image the chart under test runs component at version.
func backendImage(component, version string) string {
	if env.Enterprise() && component != "relay" {
		return "ghcr.io/netbirdio/" + component + "-cloud:" + version
	}
	return "netbirdio/" + component + ":" + version
}

// testDirectConnection starts two peers and checks they connect directly,
// which needs management to enroll them and signal to exchange candidates.
func testDirectConnection(t *testing.T) {
	alpha, beta = startPeers(t, false, "alpha", "beta")
	requireConnected(t, alpha, beta, connP2P)
	requireConnected(t, beta, alpha, connP2P)
}

// testUpgrade upgrades the backends from FromVersion to the chart's defaults
// and checks the peers and the credential Secrets survive it.
func testUpgrade(t *testing.T) {
	if env.FromVersion == env.DefaultVersion {
		t.Skipf("installed at the chart's default %s; nothing to upgrade", env.DefaultVersion)
	}
	before, err := env.CredentialSecrets(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := env.InstallChart(t.Context(), env.DefaultVersion); err != nil {
		t.Fatal(err)
	}
	for _, component := range []string{"management", "signal", "relay"} {
		image := backendImage(component, env.DefaultVersion)
		if err := env.WaitDeploymentImage(t.Context(), "netbird-"+component, image); err != nil {
			t.Fatal(err)
		}
	}
	after, err := env.CredentialSecrets(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for name := range before {
		if !maps.EqualFunc(before[name], after[name], bytes.Equal) {
			t.Fatalf("the upgrade changed the Secret %s", name)
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
func startPeers(t *testing.T, forceRelay bool, a, b string) (*netbird.Peer, *netbird.Peer) {
	t.Helper()
	peers, err := env.StartPeers(t.Context(), forceRelay, a, b)
	if err != nil {
		t.Fatal(err)
	}
	return peers[0], peers[1]
}
