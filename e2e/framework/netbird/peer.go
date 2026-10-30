//go:build e2e

package netbird

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/netbirdio/helms/e2e/framework"
)

const (
	envClientImage = "NB_E2E_CLIENT_IMAGE"

	// caInContainer is where the peers find the suite's CA. SSL_CERT_FILE
	// makes it the only root they trust.
	caInContainer = "/netbird-e2e-ca.crt"
)

// Peer is a NetBird client in a Docker container on Kind's network, outside the
// cluster, the way users' peers reach a deployment.
type Peer struct {
	Name      string
	container string
}

// Status is the part of `netbird status --json` the tests read.
type Status struct {
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
		Details []PeerStatus `json:"details"`
	} `json:"peers"`
}

// PeerStatus is how a client sees one of its peers.
type PeerStatus struct {
	IP       string `json:"netbirdIp"`
	Status   string `json:"status"`
	ConnType string `json:"connectionType"`
}

// RelayAvailable reports whether the client holds a connection to the chart's
// relay, the only one management hands out.
func (s Status) RelayAvailable() bool {
	for _, r := range s.Relays.Details {
		if strings.HasPrefix(r.URI, "rels://"+Domain) && r.Available {
			return true
		}
	}
	return false
}

// StartPeers starts a peer per name, enrolled with one new setup key.
// forceRelay keeps them off direct connections, so their traffic goes through
// the chart's relay.
func (e *Env) StartPeers(ctx context.Context, forceRelay bool, names ...string) ([]*Peer, error) {
	key, err := e.API.CreateSetupKey(ctx, "e2e-"+strings.Join(names, "-"))
	if err != nil {
		return nil, err
	}
	peers := make([]*Peer, 0, len(names))
	for _, name := range names {
		p, err := e.startPeer(ctx, name, key, forceRelay)
		if err != nil {
			return nil, err
		}
		peers = append(peers, p)
	}
	return peers, nil
}

func (e *Env) startPeer(ctx context.Context, name, setupKey string, forceRelay bool) (*Peer, error) {
	p := &Peer{Name: name, container: e.clusterName + "-" + name}
	p.remove(ctx)

	args := []string{"create", "--name", p.container, "--hostname", name,
		"--network", framework.KindNetwork,
		"--add-host", Domain + ":" + e.Cluster.NodeIP,
		"--add-host", DashboardDomain + ":" + e.Cluster.NodeIP,
		"--cap-add", "NET_ADMIN", "--cap-add", "SYS_ADMIN", "--cap-add", "SYS_RESOURCE",
		"--env", "NB_MANAGEMENT_URL=https://" + Domain,
		"--env", "NB_SETUP_KEY=" + setupKey,
		"--env", "NB_LOG_LEVEL=info",
		"--env", "SSL_CERT_FILE=" + caInContainer,
	}
	if forceRelay {
		args = append(args, "--env", "NB_FORCE_RELAY=true")
	}
	args = append(args, e.clientImage())
	if _, err := framework.Command(ctx, "docker", args...); err != nil {
		return nil, fmt.Errorf("create peer %s: %w", name, err)
	}
	e.peers = append(e.peers, p)
	if _, err := framework.Command(ctx, "docker", "cp", e.caPath(), p.container+":"+caInContainer); err != nil {
		return p, fmt.Errorf("copy the CA into peer %s: %w", name, err)
	}
	if _, err := framework.Command(ctx, "docker", "start", p.container); err != nil {
		return p, fmt.Errorf("start peer %s: %w", name, err)
	}
	return p, nil
}

// clientImage is the client the peers run: NB_E2E_CLIENT_IMAGE, or the release
// of the backends installed first. The peers keep it through the upgrade, as
// clients do when their server is upgraded.
func (e *Env) clientImage() string {
	return framework.EnvOr(envClientImage, "netbirdio/netbird:"+CommunityVersion(e.FromVersion))
}

// Status returns the client's `netbird status`.
func (p *Peer) Status(ctx context.Context) (Status, error) {
	var s Status
	out, err := framework.Command(ctx, "docker", "exec", p.container, "netbird", "status", "--json")
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal([]byte(out), &s); err != nil {
		return s, fmt.Errorf("parse the status of %s: %w: %s", p.Name, err, out)
	}
	return s, nil
}

// IP returns the peer's NetBird address, once it has one.
func (p *Peer) IP(ctx context.Context) (string, error) {
	var ip string
	var last error
	err := wait.PollUntilContextTimeout(ctx, 2*time.Second, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
		s, err := p.Status(ctx)
		if err != nil {
			last = err
			return false, nil
		}
		ip, _, _ = strings.Cut(s.IP, "/")
		return ip != "", nil
	})
	if err != nil {
		return "", fmt.Errorf("%s never got a NetBird address: %w (last: %v)", p.Name, err, last)
	}
	return ip, nil
}

// Ping sends one ping to ip from inside the peer and returns its output.
func (p *Peer) Ping(ctx context.Context, ip string) (string, error) {
	return framework.Command(ctx, "docker", "exec", p.container, "ping", "-c", "1", "-W", "2", ip)
}

func (p *Peer) logs(ctx context.Context) string {
	out, _ := framework.Command(ctx, "docker", "logs", "--tail", "200", p.container)
	return out
}

func (p *Peer) remove(ctx context.Context) {
	if out, err := framework.Command(ctx, "docker", "rm", "-f", p.container); err != nil && !strings.Contains(out, "No such container") {
		log.Printf("e2e: remove peer %s: %v", p.Name, err)
	}
}
