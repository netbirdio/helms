//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	envClientImage = "NB_E2E_CLIENT_IMAGE"

	// caInContainer is where the peers find the suite's CA. SSL_CERT_FILE
	// makes it the only root they trust.
	caInContainer = "/netbird-e2e-ca.crt"
)

// peer is a NetBird client in a Docker container on Kind's network, outside the
// cluster, the way users' peers reach a deployment.
type peer struct {
	name      string
	container string
}

// status is the part of `netbird status --json` the tests read.
type status struct {
	IP         string `json:"netbirdIp"`
	Management struct {
		Connected bool `json:"connected"`
	} `json:"management"`
	Signal struct {
		Connected bool `json:"connected"`
	} `json:"signal"`
	Relays struct {
		Details []struct {
			URI       string `json:"uri"`
			Available bool   `json:"available"`
		} `json:"details"`
	} `json:"relays"`
	Peers struct {
		Details []struct {
			IP       string `json:"netbirdIp"`
			Status   string `json:"status"`
			ConnType string `json:"connectionType"`
		} `json:"details"`
	} `json:"peers"`
}

// startPeer runs a client enrolled with setupKey. forceRelay keeps it off
// direct connections, so its traffic goes through the chart's relay.
func (e *environment) startPeer(ctx context.Context, name, setupKey string, forceRelay bool) (*peer, error) {
	p := &peer{name: name, container: kindClusterName + "-" + name}
	p.remove(ctx)

	args := []string{"create", "--name", p.container, "--hostname", name,
		"--network", kindNetwork,
		"--add-host", domain + ":" + e.nodeIP,
		"--add-host", dashboardDomain + ":" + e.nodeIP,
		"--cap-add", "NET_ADMIN", "--cap-add", "SYS_ADMIN", "--cap-add", "SYS_RESOURCE",
		"--env", "NB_MANAGEMENT_URL=https://" + domain,
		"--env", "NB_SETUP_KEY=" + setupKey,
		"--env", "NB_LOG_LEVEL=info",
		"--env", "SSL_CERT_FILE=" + caInContainer,
	}
	if forceRelay {
		args = append(args, "--env", "NB_FORCE_RELAY=true")
	}
	args = append(args, e.clientImage())
	if _, err := command(ctx, "docker", args...); err != nil {
		return nil, fmt.Errorf("create peer %s: %w", name, err)
	}
	e.peers = append(e.peers, p)
	if _, err := command(ctx, "docker", "cp", e.caPath(), p.container+":"+caInContainer); err != nil {
		return p, fmt.Errorf("copy the CA into peer %s: %w", name, err)
	}
	if _, err := command(ctx, "docker", "start", p.container); err != nil {
		return p, fmt.Errorf("start peer %s: %w", name, err)
	}
	return p, nil
}

// clientImage is the client the peers run: NB_E2E_CLIENT_IMAGE, or the release
// of the backends installed first. The peers keep it through the upgrade, as
// clients do when their server is upgraded. A newer client against a 0.74.0
// management loses its relay URLs whenever another peer joins: that server
// sends peer updates without them, and newer clients read that as no relay.
func (e *environment) clientImage() string {
	return envOr(envClientImage, "netbirdio/netbird:"+e.fromVersion)
}

// startPeers starts a peer per name with one new setup key.
func (e *environment) startPeers(t *testing.T, forceRelay bool, names ...string) []*peer {
	t.Helper()
	ctx := t.Context()
	key, err := e.api.createSetupKey(ctx, "e2e-"+strings.Join(names, "-"))
	if err != nil {
		t.Fatal(err)
	}
	peers := make([]*peer, 0, len(names))
	for _, name := range names {
		p, err := e.startPeer(ctx, name, key, forceRelay)
		if err != nil {
			t.Fatal(err)
		}
		peers = append(peers, p)
	}
	return peers
}

func (p *peer) status(ctx context.Context) (status, error) {
	var s status
	out, err := command(ctx, "docker", "exec", p.container, "netbird", "status", "--json")
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal([]byte(out), &s); err != nil {
		return s, fmt.Errorf("parse the status of %s: %w: %s", p.name, err, out)
	}
	return s, nil
}

// ip returns the peer's NetBird address, once it has one.
func (p *peer) ip(ctx context.Context) (string, error) {
	var ip string
	var last error
	err := wait.PollUntilContextTimeout(ctx, 2*time.Second, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
		s, err := p.status(ctx)
		if err != nil {
			last = err
			return false, nil
		}
		ip, _, _ = strings.Cut(s.IP, "/")
		return ip != "", nil
	})
	if err != nil {
		return "", fmt.Errorf("%s never got a NetBird address: %w (last: %v)", p.name, err, last)
	}
	return ip, nil
}

// requireConnected waits until a reports b as connected with connType, with
// management, signal and the relay reachable, and a ping from a reaches b.
// Clients connect to a peer lazily, on its first packet, and may drop the first
// packets after connecting while their firewall catches up, so every attempt
// pings.
func requireConnected(t *testing.T, a, b *peer, connType string) {
	t.Helper()
	ctx := t.Context()
	bIP, err := b.ip(ctx)
	if err != nil {
		t.Fatal(err)
	}

	var last status
	var lastErr, pingErr error
	var pingOut string
	err = wait.PollUntilContextTimeout(ctx, 3*time.Second, 4*time.Minute, true, func(ctx context.Context) (bool, error) {
		pingOut, pingErr = command(ctx, "docker", "exec", a.container, "ping", "-c", "1", "-W", "2", bIP)
		last, lastErr = a.status(ctx)
		if pingErr != nil || lastErr != nil {
			return false, nil
		}
		if !last.Management.Connected || !last.Signal.Connected || !relayAvailable(last) {
			return false, nil
		}
		for _, d := range last.Peers.Details {
			if d.IP == bIP {
				return d.Status == "Connected" && d.ConnType == connType, nil
			}
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("%s never reached %s (%s) over %s: %v\nlast status: %+v (error: %v)\nlast ping: %v\n%s",
			a.name, b.name, bIP, connType, err, last, lastErr, pingErr, pingOut)
	}
}

// relayAvailable reports whether the client holds a connection to the chart's
// relay, the only one management hands out.
func relayAvailable(s status) bool {
	for _, r := range s.Relays.Details {
		if strings.HasPrefix(r.URI, "rels://"+domain) && r.Available {
			return true
		}
	}
	return false
}

func (p *peer) logs(ctx context.Context) string {
	out, _ := command(ctx, "docker", "logs", "--tail", "200", p.container)
	return out
}

func (p *peer) remove(ctx context.Context) {
	if out, err := command(ctx, "docker", "rm", "-f", p.container); err != nil && !strings.Contains(out, "No such container") {
		log.Printf("e2e: remove peer %s: %v", p.name, err)
	}
}
