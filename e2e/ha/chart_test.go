//go:build e2e

package ha_test

import (
	"strings"
	"testing"

	"github.com/netbirdio/helms/e2e/framework/netbird"
	"github.com/netbirdio/helms/e2e/shared"
)

// TestChart walks one HA installation through its life. Peers reach a random
// management and signal replica through the ingress, so the connections only
// work when the replicas share their state through Redis and NATS.
func TestChart(t *testing.T) {
	l := &shared.Lifecycle{Env: env, BackendImage: backendImage}
	shared.Run(t,
		shared.Step{Name: "management and signal run two replicas each, not single instance", Run: testReplicas},
		shared.Step{Name: "the dashboard, the API and the IdP answer through the ingress", Run: l.HTTPRoutes},
		shared.Step{Name: "peers connect directly", Run: l.DirectConnection},
		shared.Step{Name: "peers connect through the relay", Run: l.RelayedConnection},
		shared.Step{Name: "new peers connect while a management and a signal replica are replaced", Run: testReplicaLoss},
		shared.Step{Name: "management keeps its peers across a restart of every replica", Run: l.ManagementRestart},
		shared.Step{Name: "peers on different relay pods connect", Run: l.PerPodRelays},
	)
}

// testReplicas checks both replicas of management and signal came up on the
// enterprise images and that every management replica runs the enterprise
// server.
func testReplicas(t *testing.T) {
	for _, component := range []string{"management", "signal"} {
		if err := env.WaitDeploymentImage(t.Context(), "netbird-"+component, backendImage(component, env.DefaultVersion)); err != nil {
			t.Fatal(err)
		}
	}
	logs, err := env.ManagementLogs(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(logs, "Starting CloudServer"); n < 2 {
		t.Fatalf(`want "Starting CloudServer" from both management replicas, got %d`, n)
	}
}

// testReplicaLoss deletes one management and one signal pod and, before their
// replacements are up, enrolls two new peers through the replicas left.
func testReplicaLoss(t *testing.T) {
	for _, component := range []string{"management", "signal"} {
		if err := env.RemoveOnePod(t.Context(), component); err != nil {
			t.Fatal(err)
		}
	}
	peers, err := env.StartPeers(t.Context(), false, "epsilon", "zeta")
	if err != nil {
		t.Fatal(err)
	}
	shared.RequireConnected(t, peers[0], peers[1], shared.ConnP2P)
	shared.RequireConnected(t, peers[1], peers[0], shared.ConnP2P)
	shared.RequirePeersListed(t, env, peers...)
	for _, component := range []string{"management", "signal"} {
		if err := env.WaitDeploymentImage(t.Context(), "netbird-"+component, backendImage(component, env.DefaultVersion)); err != nil {
			t.Fatal(err)
		}
	}
}

func backendImage(component, version string) string {
	if component == "relay" {
		return "netbirdio/relay:" + netbird.CommunityVersion(version)
	}
	return "ghcr.io/netbirdio/" + component + "-cloud:" + version
}
