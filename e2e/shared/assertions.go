//go:build e2e

// Package shared holds what the netbird and netbird-ee test packages share:
// the steps of a release's life and the checks behind them, written against a
// netbird.Env.
package shared

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/netbirdio/helms/e2e/framework/netbird"
)

// The connection types `netbird status` reports.
const (
	ConnP2P     = "P2P"
	ConnRelayed = "Relayed"
)

// RequireHTTPRoutes checks the dashboard, the API, which refuses a request
// without a token, and the embedded IdP's discovery document. The ingress
// answers 503 for a moment after a rollout, until it sees the new pod's
// endpoint, so each check waits for its answer.
func RequireHTTPRoutes(t *testing.T, env *netbird.Env) {
	t.Helper()
	if code, body := get(t, env, "https://"+netbird.DashboardDomain+"/", http.StatusOK); code != http.StatusOK || !strings.Contains(body, "<html") {
		t.Errorf("dashboard: got %d, want 200 with an HTML page", code)
	}
	if code, _ := get(t, env, "https://"+netbird.Domain+"/api/users", http.StatusUnauthorized); code != http.StatusUnauthorized {
		t.Errorf("GET /api/users without a token: got %d, want 401", code)
	}
	code, body := get(t, env, "https://"+netbird.Domain+"/oauth2/.well-known/openid-configuration", http.StatusOK)
	var discovery struct {
		Issuer string `json:"issuer"`
	}
	if code != http.StatusOK || json.Unmarshal([]byte(body), &discovery) != nil {
		t.Fatalf("OIDC discovery: got %d: %.200s", code, body)
	}
	if want := "https://" + netbird.Domain + "/oauth2"; discovery.Issuer != want {
		t.Errorf("OIDC issuer: got %q, want %q", discovery.Issuer, want)
	}
}

// get fetches url through the ingress until it answers want or two minutes
// pass, and returns the last answer.
func get(t *testing.T, env *netbird.Env, url string, want int) (int, string) {
	t.Helper()
	client := env.HTTPClient()
	var code int
	var body string
	var last error
	_ = wait.PollUntilContextTimeout(t.Context(), 2*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return false, err
		}
		resp, err := client.Do(req)
		if err != nil {
			last = err
			return false, nil
		}
		defer func() { _ = resp.Body.Close() }()
		raw, _ := io.ReadAll(resp.Body)
		code, body = resp.StatusCode, string(raw)
		return code == want, nil
	})
	if code == 0 {
		t.Fatalf("GET %s: %v", url, last)
	}
	return code, body
}

// RequireConnected waits until a reports b as connected with connType, with
// management, signal and the relay reachable, and a ping from a reaches b.
// Clients connect to a peer lazily, on its first packet, and may drop the first
// packets after connecting while their firewall catches up, so every attempt
// pings.
func RequireConnected(t *testing.T, a, b *netbird.Peer, connType string) {
	t.Helper()
	ctx := t.Context()
	bIP, err := b.IP(ctx)
	if err != nil {
		t.Fatal(err)
	}

	var last netbird.Status
	var lastErr, pingErr error
	var pingOut string
	err = wait.PollUntilContextTimeout(ctx, 3*time.Second, 4*time.Minute, true, func(ctx context.Context) (bool, error) {
		pingOut, pingErr = a.Ping(ctx, bIP)
		last, lastErr = a.Status(ctx)
		if pingErr != nil || lastErr != nil {
			return false, nil
		}
		if !last.Management.Connected || !last.Signal.Connected || !last.RelayAvailable() {
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
			a.Name, b.Name, bIP, connType, err, last, lastErr, pingErr, pingOut)
	}
}

// RequirePeersListed checks that management still lists every peer, which it
// only can if its store survived.
func RequirePeersListed(t *testing.T, env *netbird.Env, want ...*netbird.Peer) {
	t.Helper()
	var listed []string
	var last error
	err := wait.PollUntilContextTimeout(t.Context(), 3*time.Second, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
		peers, err := env.API.Peers(ctx)
		if err != nil {
			last = err
			return false, nil
		}
		listed = listed[:0]
		for _, p := range peers {
			listed = append(listed, p.Hostname)
		}
		for _, p := range want {
			if !slices.Contains(listed, p.Name) {
				return false, nil
			}
		}
		return true, nil
	})
	if err != nil {
		t.Fatalf("management lists %v, want all of the started peers: %v (last: %v)", listed, err, last)
	}
}
