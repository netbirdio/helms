//go:build e2e

package netbird

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
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

// RelayAvailable reports whether the client holds a connection to one of the
// chart's relays.
func (s Status) RelayAvailable() bool {
	for _, r := range s.Relays.Details {
		u, err := url.Parse(r.URI)
		if err == nil && u.Scheme == "rels" && r.Available &&
			(u.Hostname() == Domain || strings.HasSuffix(u.Hostname(), "."+Domain)) {
			return true
		}
	}
	return false
}

// relayAvailable reports whether the client holds a connection to the relay at host.
func (s Status) relayAvailable(host string) bool {
	for _, r := range s.Relays.Details {
		if u, err := url.Parse(r.URI); err == nil && u.Hostname() == host && r.Available {
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
		p, err := e.startPeer(ctx, name, key, forceRelay, e.clientImage(), hostnames())
		if err != nil {
			return nil, err
		}
		peers = append(peers, p)
	}
	return peers, nil
}

// StartPeersOnRelays starts relay-only peers at the default client version, peer i with relay i as its home relay.
func (e *Env) StartPeersOnRelays(ctx context.Context, names ...string) ([]*Peer, error) {
	if len(names) > RelayReplicas {
		return nil, fmt.Errorf("%d peers for %d relays", len(names), RelayReplicas)
	}
	key, err := e.API.CreateSetupKey(ctx, "e2e-"+strings.Join(names, "-"))
	if err != nil {
		return nil, err
	}
	image := framework.EnvOr(envClientImage, "netbirdio/netbird:"+CommunityVersion(e.DefaultVersion))
	peers := make([]*Peer, 0, len(names))
	for i, name := range names {
		home := RelayHost(i)
		// Only its home relay resolves until it connects, so the peer can pick no other.
		p, err := e.startPeer(ctx, name, key, true, image, []string{Domain, DashboardDomain, home})
		if err != nil {
			return nil, err
		}
		if err := p.waitRelay(ctx, home); err != nil {
			return nil, err
		}
		var others []string
		for _, host := range hostnames() {
			if host != Domain && host != DashboardDomain && host != home {
				others = append(others, host)
			}
		}
		if err := p.addHosts(ctx, e.Cluster.NodeIP, others...); err != nil {
			return nil, err
		}
		peers = append(peers, p)
	}
	return peers, nil
}

func (e *Env) startPeer(ctx context.Context, name, setupKey string, forceRelay bool, image string, hosts []string) (*Peer, error) {
	p := &Peer{Name: name, container: e.clusterName + "-" + name}
	p.remove(ctx)

	args := []string{"create", "--name", p.container, "--hostname", name,
		"--network", framework.KindNetwork}
	for _, host := range hosts {
		args = append(args, "--add-host", host+":"+e.Cluster.NodeIP)
	}
	args = append(args,
		"--cap-add", "NET_ADMIN", "--cap-add", "SYS_ADMIN", "--cap-add", "SYS_RESOURCE",
		"--env", "NB_MANAGEMENT_URL=https://"+Domain,
		"--env", "NB_SETUP_KEY="+setupKey,
		"--env", "NB_LOG_LEVEL=info",
		"--env", "SSL_CERT_FILE="+caInContainer,
	)
	if forceRelay {
		args = append(args, "--env", "NB_FORCE_RELAY=true")
	}
	args = append(args, image)
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

// waitRelay waits until the client holds a connection to the relay at host.
func (p *Peer) waitRelay(ctx context.Context, host string) error {
	var last Status
	err := wait.PollUntilContextTimeout(ctx, 2*time.Second, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
		s, err := p.Status(ctx)
		if err != nil {
			return false, nil
		}
		last = s
		return s.relayAvailable(host), nil
	})
	if err != nil {
		return fmt.Errorf("%s never connected to the relay %s: %w (last relays: %+v)", p.Name, host, err, last.Relays.Details)
	}
	return nil
}

// addHosts appends hosts to the peer's bind-mounted /etc/hosts, resolving to ip.
func (p *Peer) addHosts(ctx context.Context, ip string, hosts ...string) error {
	var lines strings.Builder
	for _, host := range hosts {
		fmt.Fprintf(&lines, "%s %s\n", ip, host)
	}
	if _, err := framework.Command(ctx, "docker", "exec", p.container, "sh", "-c", "printf '%s' \"$0\" >> /etc/hosts", lines.String()); err != nil {
		return fmt.Errorf("add %v to the hosts of %s: %w", hosts, p.Name, err)
	}
	return nil
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
