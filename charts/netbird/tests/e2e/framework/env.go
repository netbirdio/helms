//go:build e2e

// Package framework is the plumbing behind the chart's end-to-end tests. It
// brings up a Kind cluster with ingress-nginx and the chart, runs NetBird peers
// in Docker next to it, and talks to management through the ingress. It asserts
// nothing: the tests in the parent package decide what passes.
package framework

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/kind/pkg/cluster"
)

const (
	// Domain and DashboardDomain are the hostnames the chart is installed
	// under. The peers resolve them to the Kind node, where the ingress listens.
	Domain          = "netbird.e2e.test"
	DashboardDomain = "dashboard." + Domain

	// Namespace and ReleaseName are where and as what the chart is installed.
	Namespace   = "netbird"
	ReleaseName = "netbird"

	// BootstrapTimeout bounds Bootstrap.
	BootstrapTimeout = 20 * time.Minute

	defaultFromVersion = "0.74.0"

	envFromVersion  = "NB_E2E_FROM_VERSION"
	envRootless     = "NB_E2E_ROOTLESS"
	envKeep         = "NB_E2E_KEEP"
	envArtifactsDir = "NB_E2E_ARTIFACTS_DIR"
)

// Env is a Kind cluster with ingress-nginx and the chart installed, and the
// peers the tests started outside it.
type Env struct {
	// FromVersion is the backend version installed first, DefaultVersion the
	// chart's. They are equal when there is no upgrade to test.
	FromVersion    string
	DefaultVersion string
	// Rootless is whether examples/rootless is layered in.
	Rootless bool

	// Kube is a client for the cluster.
	Kube kubernetes.Interface
	// API is the management REST API as the account owner.
	API *APIClient

	kind       *cluster.Provider
	kubeconfig string
	restConfig *rest.Config
	workDir    string
	// nodeIP is the Kind node's address on Kind's Docker network.
	nodeIP  string
	ca      *certAuthority
	ingress *ingressForward
	peers   []*Peer
}

// Bootstrap brings the environment up: the cluster, ingress-nginx, the chart at
// FromVersion, and the account. On error it returns whatever it got as far as,
// so the caller can still collect logs and tear it down.
func Bootstrap(ctx context.Context) (*Env, error) {
	for _, tool := range []string{"docker", "helm"} {
		if _, err := exec.LookPath(tool); err != nil {
			return nil, fmt.Errorf("%s is required: %w", tool, err)
		}
	}
	defaultVersion, err := chartBackendVersion()
	if err != nil {
		return nil, err
	}
	workDir, err := os.MkdirTemp("", "netbird-chart-e2e-*")
	if err != nil {
		return nil, fmt.Errorf("create work dir: %w", err)
	}
	e := &Env{
		workDir:        workDir,
		DefaultVersion: defaultVersion,
		FromVersion:    envOr(envFromVersion, defaultFromVersion),
		Rootless:       envBool(envRootless),
	}
	log.Printf("e2e: backends %s, then %s; rootless %t", e.FromVersion, e.DefaultVersion, e.Rootless)

	log.Print("e2e: creating the Kind cluster")
	if err := e.createCluster(ctx); err != nil {
		return e, err
	}
	log.Print("e2e: installing ingress-nginx")
	if err := e.installIngress(ctx); err != nil {
		return e, err
	}
	if err := e.prepareNamespace(ctx); err != nil {
		return e, err
	}
	log.Printf("e2e: installing the chart with backends %s", e.FromVersion)
	if err := e.InstallChart(ctx, e.FromVersion); err != nil {
		return e, err
	}
	e.ingress, err = e.forwardIngress(ctx)
	if err != nil {
		return e, err
	}
	e.API = newAPIClient(e.HTTPClient(), "https://"+Domain)
	log.Print("e2e: activating the account through /api/setup")
	if err := e.API.setup(ctx); err != nil {
		return e, err
	}
	return e, nil
}

// HTTPClient returns a client that reaches the chart's ingress for any of its
// hostnames, trusting the suite's CA.
func (e *Env) HTTPClient() *http.Client {
	return e.ingress.client(e.ca)
}

// Teardown removes the peers and the cluster, unless NB_E2E_KEEP is set.
func (e *Env) Teardown() {
	if envBool(envKeep) {
		log.Printf("e2e: %s is set, leaving the environment running; kubeconfig: %s", envKeep, e.kubeconfig)
		return
	}
	if e.ingress != nil {
		e.ingress.stop()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	for _, p := range e.peers {
		p.remove(ctx)
	}
	if e.kind != nil {
		if err := e.kind.Delete(kindClusterName, e.kubeconfig); err != nil {
			log.Printf("e2e: delete the Kind cluster: %v", err)
		}
	}
	if err := os.RemoveAll(e.workDir); err != nil {
		log.Printf("e2e: remove %s: %v", e.workDir, err)
	}
}
