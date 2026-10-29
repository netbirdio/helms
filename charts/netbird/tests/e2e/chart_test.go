//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	connP2P     = "P2P"
	connRelayed = "Relayed"
)

// TestChart runs in order: later steps reuse the peers earlier ones start, and
// the backends are upgraded under them halfway through.
func TestChart(t *testing.T) {
	e := requireEnv(t)
	var alpha, beta, gamma, delta *peer

	steps := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"the dashboard, the API and the IdP answer through the ingress", func(t *testing.T) {
			e.checkHTTP(t)
		}},
		{"peers connect directly", func(t *testing.T) {
			// Direct connections need signal to exchange candidates.
			peers := e.startPeers(t, false, "alpha", "beta")
			alpha, beta = peers[0], peers[1]
			requireConnected(t, alpha, beta, connP2P)
			requireConnected(t, beta, alpha, connP2P)
		}},
		{"an upgrade to the chart's backends keeps the peers", func(t *testing.T) {
			if e.fromVersion == e.defaultVersion {
				t.Skipf("installed at the chart's default %s; nothing to upgrade", e.defaultVersion)
			}
			if err := e.installChart(t.Context(), e.defaultVersion); err != nil {
				t.Fatal(err)
			}
			for _, component := range []string{"management", "signal", "relay"} {
				image := "netbirdio/" + component + ":" + e.defaultVersion
				if err := e.waitDeploymentImage(t.Context(), "netbird-"+component, image); err != nil {
					t.Fatal(err)
				}
			}
			e.checkHTTP(t)
			e.requirePeersListed(t, alpha, beta)
			requireConnected(t, alpha, beta, connP2P)
		}},
		// On the chart's backends only: a 0.74.0 management sends the peers it
		// updates when another peer joins no relay URLs, and clients then drop
		// the relay.
		{"peers connect through the relay", func(t *testing.T) {
			peers := e.startPeers(t, true, "gamma", "delta")
			gamma, delta = peers[0], peers[1]
			requireConnected(t, gamma, delta, connRelayed)
			requireConnected(t, delta, gamma, connRelayed)
		}},
		{"management keeps its peers across a restart", func(t *testing.T) {
			e.restartManagement(t)
			e.requirePeersListed(t, alpha, beta, gamma, delta)
			requireConnected(t, alpha, beta, connP2P)
			requireConnected(t, gamma, delta, connRelayed)
		}},
	}
	for _, step := range steps {
		// Each step builds on the ones before it.
		if !t.Run(step.name, step.run) {
			t.FailNow()
		}
	}
}

// checkHTTP checks the routes a browser uses: the dashboard, the API, which
// refuses a request without a token, and the embedded IdP's discovery document.
// The ingress answers 503 for a moment after a rollout, until it sees the new
// pod's endpoint, so each check waits for its answer.
func (e *environment) checkHTTP(t *testing.T) {
	client := e.ingress.client(e.ca)
	get := func(url string, want int) (int, string) {
		t.Helper()
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

	if code, body := get("https://"+dashboardDomain+"/", http.StatusOK); code != http.StatusOK || !strings.Contains(body, "<html") {
		t.Errorf("dashboard: got %d, want 200 with an HTML page", code)
	}
	if code, _ := get("https://"+domain+"/api/users", http.StatusUnauthorized); code != http.StatusUnauthorized {
		t.Errorf("GET /api/users without a token: got %d, want 401", code)
	}
	code, body := get("https://"+domain+"/oauth2/.well-known/openid-configuration", http.StatusOK)
	var discovery struct {
		Issuer string `json:"issuer"`
	}
	if code != http.StatusOK || json.Unmarshal([]byte(body), &discovery) != nil {
		t.Fatalf("OIDC discovery: got %d: %.200s", code, body)
	}
	if want := "https://" + domain + "/oauth2"; discovery.Issuer != want {
		t.Errorf("OIDC issuer: got %q, want %q", discovery.Issuer, want)
	}
}

// restartManagement deletes management's pod and waits for its replacement.
func (e *environment) restartManagement(t *testing.T) {
	ctx := t.Context()
	pods := e.clientset.CoreV1().Pods(namespace)
	selector := metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=netbird-management,app.kubernetes.io/instance=" + releaseName}
	old, err := pods.List(ctx, selector)
	if err != nil {
		t.Fatal(err)
	}
	if len(old.Items) == 0 {
		t.Fatalf("no management pod matches %s", selector.LabelSelector)
	}
	var oldNames []string
	for _, p := range old.Items {
		oldNames = append(oldNames, p.Name)
		if err := pods.Delete(ctx, p.Name, metav1.DeleteOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	err = wait.PollUntilContextTimeout(ctx, 3*time.Second, 5*time.Minute, true, func(ctx context.Context) (bool, error) {
		list, err := pods.List(ctx, selector)
		if err != nil {
			return false, err
		}
		for _, p := range list.Items {
			if slices.Contains(oldNames, p.Name) || p.DeletionTimestamp != nil {
				return false, nil
			}
			for _, c := range p.Status.ContainerStatuses {
				if !c.Ready {
					return false, nil
				}
			}
		}
		return len(list.Items) > 0, nil
	})
	if err != nil {
		t.Fatalf("management did not come back after its pod was deleted: %v", err)
	}
}

// requirePeersListed checks that management still lists every peer, which it
// only can if its store survived.
func (e *environment) requirePeersListed(t *testing.T, want ...*peer) {
	t.Helper()
	var listed []string
	var last error
	err := wait.PollUntilContextTimeout(t.Context(), 3*time.Second, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
		peers, err := e.api.peers(ctx)
		if err != nil {
			last = err
			return false, nil
		}
		listed = listed[:0]
		for _, p := range peers {
			listed = append(listed, p.Hostname)
		}
		for _, p := range want {
			if !slices.Contains(listed, p.name) {
				return false, nil
			}
		}
		return true, nil
	})
	if err != nil {
		t.Fatalf("management lists %v, want all of the started peers: %v (last: %v)", listed, err, last)
	}
}
