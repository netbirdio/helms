//go:build e2e

package shared

import (
	"bytes"
	"maps"
	"testing"

	"github.com/netbirdio/helms/e2e/framework/netbird"
)

// Step is one stage of a release's life.
type Step struct {
	Name string
	Run  func(t *testing.T)
}

// Run runs steps in order and stops at the first failure: later steps reuse
// the peers earlier ones start.
func Run(t *testing.T, steps ...Step) {
	for _, step := range steps {
		if !t.Run(step.Name, step.Run) {
			t.FailNow()
		}
	}
}

// Lifecycle walks one installation through its life: peers connect directly,
// the release is upgraded under them, peers connect through the relay, and
// management restarts. Its methods are the steps, in that order.
type Lifecycle struct {
	Env *netbird.Env
	// BackendImage is the image the chart under test runs component at version.
	BackendImage func(component, version string) string

	// The peers the steps start, in the order they start them.
	alpha, beta, gamma, delta *netbird.Peer
}

// HTTPRoutes checks what a browser uses: the dashboard, the API and the
// embedded IdP, all through the ingress.
func (l *Lifecycle) HTTPRoutes(t *testing.T) {
	RequireHTTPRoutes(t, l.Env)
}

// DirectConnection starts two peers and checks they connect directly, which
// needs management to enroll them and signal to exchange candidates.
func (l *Lifecycle) DirectConnection(t *testing.T) {
	l.alpha, l.beta = l.startPeers(t, false, "alpha", "beta")
	RequireConnected(t, l.alpha, l.beta, ConnP2P)
	RequireConnected(t, l.beta, l.alpha, ConnP2P)
}

// Upgrade upgrades the release from FromChart to Chart and the backends from
// FromVersion to the chart's defaults, and checks the peers and the credential
// Secrets survive it. From netbird to netbird-ee it is the migration
// charts/netbird-ee/docs/migration.md describes.
func (l *Lifecycle) Upgrade(t *testing.T) {
	env := l.Env
	if env.FromChart == env.Chart && env.FromVersion == env.DefaultVersion {
		t.Skipf("installed %s at its default %s; nothing to upgrade", env.Chart, env.DefaultVersion)
	}
	before, err := env.CredentialSecrets(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := env.InstallChart(t.Context(), env.Chart, env.DefaultVersion); err != nil {
		t.Fatal(err)
	}
	for _, component := range []string{"management", "signal", "relay"} {
		if err := env.WaitDeploymentImage(t.Context(), "netbird-"+component, l.BackendImage(component, env.DefaultVersion)); err != nil {
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
	RequireHTTPRoutes(t, env)
	RequirePeersListed(t, env, l.alpha, l.beta)
	RequireConnected(t, l.alpha, l.beta, ConnP2P)
}

// RelayedConnection starts two peers that may only use the relay and checks
// they connect through it.
//
// It runs after the upgrade: a 0.74.0 management sends the peers it updates
// when another peer joins no relay URLs, and clients then drop the relay.
func (l *Lifecycle) RelayedConnection(t *testing.T) {
	l.gamma, l.delta = l.startPeers(t, true, "gamma", "delta")
	RequireConnected(t, l.gamma, l.delta, ConnRelayed)
	RequireConnected(t, l.delta, l.gamma, ConnRelayed)
}

// ManagementRestart deletes management's pod and checks every peer is still
// listed, so the store persisted, and the connections come back.
func (l *Lifecycle) ManagementRestart(t *testing.T) {
	if err := l.Env.RestartManagement(t.Context()); err != nil {
		t.Fatal(err)
	}
	RequirePeersListed(t, l.Env, l.alpha, l.beta, l.gamma, l.delta)
	RequireConnected(t, l.alpha, l.beta, ConnP2P)
	RequireConnected(t, l.gamma, l.delta, ConnRelayed)
}

// startPeers starts two peers with one setup key; forceRelay keeps them off
// direct connections.
func (l *Lifecycle) startPeers(t *testing.T, forceRelay bool, a, b string) (*netbird.Peer, *netbird.Peer) {
	t.Helper()
	peers, err := l.Env.StartPeers(t.Context(), forceRelay, a, b)
	if err != nil {
		t.Fatal(err)
	}
	return peers[0], peers[1]
}
