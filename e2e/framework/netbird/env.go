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

	// RelayReplicas is how many relays EnablePerPodRelays runs, relay i at RelayHost(i).
	RelayReplicas = 2

	// Namespace and ReleaseName are where and as what the chart is installed.
	Namespace   = "netbird"
	ReleaseName = "netbird"

	// BootstrapTimeout bounds Bootstrap.
	BootstrapTimeout = 20 * time.Minute

	// tlsSecretName is the Secret examples/kind's ingresses reference.
	tlsSecretName = "netbird-local-tls"

	defaultFromVersion = "0.74.0"

	// EditionCommunity and EditionEnterprise are the chart's editions.
	EditionCommunity  = "community"
	EditionEnterprise = "enterprise"

	envFromVersion = "NB_E2E_FROM_VERSION"
	envRootless    = "NB_E2E_ROOTLESS"
)

// Config is what a test package brings up.
type Config struct {
	// Edition is the edition under test: EditionCommunity or EditionEnterprise.
	Edition string
	// FromEdition is the edition installed first; empty means Edition. The
	// only other choice is EditionCommunity before EditionEnterprise, the switch.
	FromEdition string
	// ClusterName names the Kind cluster, and the peers' containers after it,
	// so test packages can run side by side.
	ClusterName string
}

// Env is a Kind cluster with ingress-nginx and the chart installed, and the
// peers the tests started outside it.
type Env struct {
	// Edition is the edition under test: EditionCommunity or EditionEnterprise.
	// FromEdition is the edition installed first, Edition unless the suite
	// tests the switch from community to enterprise.
	Edition     string
	FromEdition string
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
	// perPodRelays is whether EnablePerPodRelays ran, so upgrades keep it.
	perPodRelays bool
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
	edition := cfg.Edition
	if edition != EditionCommunity && edition != EditionEnterprise {
		return nil, fmt.Errorf("edition must be %s or %s, not %q", EditionCommunity, EditionEnterprise, edition)
	}
	fromEdition := cmp.Or(cfg.FromEdition, edition)
	if fromEdition != edition && (fromEdition != EditionCommunity || edition != EditionEnterprise) {
		return nil, fmt.Errorf("cannot go from %s to %s: the only switch is from %s to %s", fromEdition, edition, EditionCommunity, EditionEnterprise)
	}
	defaultVersion, err := chartBackendVersion(ctx, edition)
	if err != nil {
		return nil, err
	}
	// The enterprise edition has no earlier release to upgrade from yet, and
	// a switch keeps the backend version: the enterprise store must not be older.
	fromVersion := defaultFromVersion
	if edition == EditionEnterprise {
		fromVersion = defaultVersion
	}
	workDir, err := os.MkdirTemp("", cfg.ClusterName+"-*")
	if err != nil {
		return nil, fmt.Errorf("create work dir: %w", err)
	}
	e := &Env{
		clusterName:    cfg.ClusterName,
		workDir:        workDir,
		Edition:        edition,
		FromEdition:    fromEdition,
		DefaultVersion: defaultVersion,
		FromVersion:    framework.EnvOr(envFromVersion, fromVersion),
		Rootless:       framework.EnvBool(envRootless),
	}
	log.Printf("e2e: edition %s with backends %s, then %s with %s; rootless %t", e.FromEdition, e.FromVersion, e.Edition, e.DefaultVersion, e.Rootless)

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
	log.Printf("e2e: installing the %s edition with backends %s", e.FromEdition, e.FromVersion)
	if err := e.InstallChart(ctx, e.FromEdition, e.FromVersion); err != nil {
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
	if err := e.Cluster.CreateTLSSecret(ctx, Namespace, tlsSecretName, e.ca, hostnames()...); err != nil {
		return err
	}
	return os.WriteFile(e.caPath(), e.ca.CertPEM, 0o644)
}

// RelayHost is relay i's hostname once EnablePerPodRelays runs one relay per pod.
func RelayHost(i int) string {
	return fmt.Sprintf("relay-%d.%s", i, Domain)
}

// hostnames are every hostname the chart serves, all at the ingress.
func hostnames() []string {
	hosts := []string{Domain, DashboardDomain}
	for i := range RelayReplicas {
		hosts = append(hosts, RelayHost(i))
	}
	return hosts
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
