//go:build e2e

// Package netbird is the plumbing behind the netbird chart's end-to-end tests.
// It installs the chart into a framework.Cluster, runs NetBird peers in Docker
// next to it, and talks to management through the ingress. It asserts nothing:
// the tests in e2e/netbird decide what passes.
package netbird

import (
	"cmp"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/netbirdio/helms/e2e/framework"
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

	// tlsSecretName is the Secret examples/kind's ingresses reference.
	tlsSecretName = "netbird-local-tls"

	defaultFromVersion = "0.74.0"

	// ChartCommunity and ChartEnterprise are the charts the suite can test.
	ChartCommunity  = "netbird"
	ChartEnterprise = "netbird-ee"

	envFromVersion = "NB_E2E_FROM_VERSION"
	envRootless    = "NB_E2E_ROOTLESS"
)

// Config is what a test package brings up.
type Config struct {
	// Chart is the chart under test: ChartCommunity or ChartEnterprise.
	Chart string
	// FromChart is the chart installed first; empty means Chart. The only
	// other choice is ChartCommunity before ChartEnterprise, the migration.
	FromChart string
	// ClusterName names the Kind cluster, and the peers' containers after it,
	// so test packages can run side by side.
	ClusterName string
}

// Env is a Kind cluster with ingress-nginx and the chart installed, and the
// peers the tests started outside it.
type Env struct {
	// Chart is the chart under test: ChartCommunity or ChartEnterprise.
	// FromChart is the chart installed first, Chart unless the suite tests
	// the migration from netbird to netbird-ee.
	Chart     string
	FromChart string
	// FromVersion is the backend version installed first, DefaultVersion the
	// chart's. They are equal when there is no upgrade to test.
	FromVersion    string
	DefaultVersion string
	// Rootless is whether examples/rootless is layered in.
	Rootless bool

	// Cluster is the Kind cluster the chart runs in.
	Cluster *framework.Cluster
	// API is the management REST API as the account owner.
	API *APIClient

	clusterName string
	workDir     string
	ca          *framework.CertAuthority
	ingress     *framework.IngressForward
	peers       []*Peer
}

// Bootstrap brings the environment up: the cluster, ingress-nginx, the chart at
// FromVersion, and the account. On error it returns whatever it got as far as,
// so the caller can still collect logs and tear it down.
func Bootstrap(ctx context.Context, cfg Config) (*Env, error) {
	for _, tool := range []string{"docker", "helm"} {
		if _, err := exec.LookPath(tool); err != nil {
			return nil, fmt.Errorf("%s is required: %w", tool, err)
		}
	}
	chart := cfg.Chart
	if chart != ChartCommunity && chart != ChartEnterprise {
		return nil, fmt.Errorf("chart must be %s or %s, not %q", ChartCommunity, ChartEnterprise, chart)
	}
	fromChart := cmp.Or(cfg.FromChart, chart)
	if fromChart != chart && (fromChart != ChartCommunity || chart != ChartEnterprise) {
		return nil, fmt.Errorf("cannot go from %s to %s: the only migration is from %s to %s", fromChart, chart, ChartCommunity, ChartEnterprise)
	}
	defaultVersion, err := chartBackendVersion(chart)
	if err != nil {
		return nil, err
	}
	// netbird-ee has no earlier release to upgrade from yet, and a migration
	// keeps the backend version: the enterprise store must not be older.
	fromVersion := defaultFromVersion
	if chart == ChartEnterprise {
		fromVersion = defaultVersion
	}
	workDir, err := os.MkdirTemp("", cfg.ClusterName+"-*")
	if err != nil {
		return nil, fmt.Errorf("create work dir: %w", err)
	}
	e := &Env{
		clusterName:    cfg.ClusterName,
		workDir:        workDir,
		Chart:          chart,
		FromChart:      fromChart,
		DefaultVersion: defaultVersion,
		FromVersion:    framework.EnvOr(envFromVersion, fromVersion),
		Rootless:       framework.EnvBool(envRootless),
	}
	log.Printf("e2e: chart %s with backends %s, then %s with %s; rootless %t", e.FromChart, e.FromVersion, e.Chart, e.DefaultVersion, e.Rootless)
	if e.Enterprise() {
		log.Print("e2e: vendoring the netbird chart into netbird-ee")
		if _, err := framework.Command(ctx, "helm", "dependency", "update", framework.ChartDir(ChartEnterprise)); err != nil {
			return e, err
		}
	}

	log.Print("e2e: creating the Kind cluster")
	e.Cluster, err = framework.CreateCluster(ctx, e.clusterName, workDir)
	if err != nil {
		return e, err
	}
	log.Print("e2e: installing ingress-nginx")
	if err := e.Cluster.InstallIngressNginx(ctx); err != nil {
		return e, err
	}
	if err := e.prepareNamespace(ctx); err != nil {
		return e, err
	}
	log.Printf("e2e: installing %s with backends %s", e.FromChart, e.FromVersion)
	if err := e.InstallChart(ctx, e.FromChart, e.FromVersion); err != nil {
		return e, err
	}
	e.ingress, err = e.Cluster.ForwardIngress(ctx)
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

// prepareNamespace creates the chart's namespace, enforcing the restricted Pod
// Security Standard when rootless, and the TLS Secret its ingresses reference.
// The peers trust the CA through the file it writes.
func (e *Env) prepareNamespace(ctx context.Context) error {
	if err := e.Cluster.CreateNamespace(ctx, Namespace, e.Rootless); err != nil {
		return err
	}
	var err error
	e.ca, err = framework.NewCertAuthority(e.clusterName)
	if err != nil {
		return err
	}
	if err := e.Cluster.CreateTLSSecret(ctx, Namespace, tlsSecretName, e.ca, Domain, DashboardDomain); err != nil {
		return err
	}
	return os.WriteFile(e.caPath(), e.ca.CertPEM, 0o644)
}

func (e *Env) caPath() string {
	return filepath.Join(e.workDir, "ca.crt")
}

// HTTPClient returns a client that reaches the chart's ingress for any of its
// hostnames, trusting the suite's CA.
func (e *Env) HTTPClient() *http.Client {
	return e.ingress.Client(e.ca)
}

// Teardown removes the peers and the cluster, unless NB_E2E_KEEP is set.
func (e *Env) Teardown() {
	if framework.Keep() {
		kubeconfig := ""
		if e.Cluster != nil {
			kubeconfig = e.Cluster.Kubeconfig
		}
		log.Printf("e2e: NB_E2E_KEEP is set, leaving the environment running; kubeconfig: %s", kubeconfig)
		return
	}
	if e.ingress != nil {
		e.ingress.Stop()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	for _, p := range e.peers {
		p.remove(ctx)
	}
	if e.Cluster != nil {
		if err := e.Cluster.Delete(); err != nil {
			log.Printf("e2e: delete the Kind cluster: %v", err)
		}
	}
	if err := os.RemoveAll(e.workDir); err != nil {
		log.Printf("e2e: remove %s: %v", e.workDir, err)
	}
}
