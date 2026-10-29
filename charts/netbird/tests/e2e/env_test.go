//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/kind/pkg/apis/config/v1alpha4"
	"sigs.k8s.io/kind/pkg/cluster"
)

const (
	kindClusterName = "netbird-chart-e2e"
	// kindNetwork is the Docker network Kind puts its nodes on. The peers join
	// it to reach the ingress on the node.
	kindNetwork = "kind"

	// The node image has to match the Kind version in go.mod.
	defaultKindNodeImage = "kindest/node:v1.37.0@sha256:a1ed56cfb0e7b93589bdf97c8cd566405a265939e3620fc4f5de89adff580ae5"

	// ingress-nginx at the controller version the Taskfile's dev cluster runs
	// (1.12.1), with host ports on the node so peers reach it on 443.
	ingressNamespace    = "ingress-nginx"
	ingressChartVersion = "4.12.1"

	namespace   = "netbird"
	releaseName = "netbird"

	// examples/kind serves exampleDomain; the suite installs it under domain,
	// which the peers resolve to the Kind node.
	exampleDomain   = "netbird.localhost"
	domain          = "netbird.e2e.test"
	dashboardDomain = "dashboard." + domain
	// tlsSecretName is the Secret examples/kind's ingresses reference.
	tlsSecretName = "netbird-local-tls"

	// chartDir is the chart, relative to this package.
	chartDir = "../.."

	defaultFromVersion = "0.74.0"

	envKindNodeImage = "NB_E2E_KIND_NODE_IMAGE"
	envFromVersion   = "NB_E2E_FROM_VERSION"
	envRootless      = "NB_E2E_ROOTLESS"
	envKeep          = "NB_E2E_KEEP"
	envArtifactsDir  = "NB_E2E_ARTIFACTS_DIR"

	bootstrapTimeout = 20 * time.Minute
)

// testEnv is the environment TestMain brought up. Tests reach it through
// requireEnv.
var testEnv *environment

// environment is a Kind cluster with ingress-nginx and the chart installed,
// and the peers the tests started outside it.
type environment struct {
	kind       *cluster.Provider
	kubeconfig string
	restConfig *rest.Config
	clientset  kubernetes.Interface

	workDir string
	// nodeIP is the Kind node's address on kindNetwork, where the ingress
	// listens. The peers resolve domain and dashboardDomain to it.
	nodeIP string
	ca     *certAuthority

	// fromVersion is the backend version installed first; defaultVersion is
	// the chart's. They are equal when there is no upgrade to test.
	fromVersion    string
	defaultVersion string
	rootless       bool

	ingress *ingressForward
	api     *apiClient
	peers   []*peer
}

// bootstrap brings the environment up. On error it returns whatever it got as
// far as, so the caller can still collect logs and tear it down.
func bootstrap(ctx context.Context) (*environment, error) {
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
	env := &environment{
		workDir:        workDir,
		defaultVersion: defaultVersion,
		fromVersion:    envOr(envFromVersion, defaultFromVersion),
		rootless:       envBool(envRootless),
	}
	log.Printf("e2e: backends %s, then %s; rootless %t", env.fromVersion, env.defaultVersion, env.rootless)

	log.Print("e2e: creating the Kind cluster")
	if err := env.createCluster(ctx); err != nil {
		return env, err
	}
	log.Print("e2e: installing ingress-nginx")
	if err := env.installIngress(ctx); err != nil {
		return env, err
	}
	if err := env.prepareNamespace(ctx); err != nil {
		return env, err
	}
	log.Printf("e2e: installing the chart with backends %s", env.fromVersion)
	if err := env.installChart(ctx, env.fromVersion); err != nil {
		return env, err
	}
	env.ingress, err = env.forwardIngress(ctx)
	if err != nil {
		return env, err
	}
	env.api = newAPIClient(env.ingress.client(env.ca), "https://"+domain)
	log.Print("e2e: activating the account through /api/setup")
	if err := env.api.setup(ctx); err != nil {
		return env, err
	}
	return env, nil
}

// createCluster creates a one-node Kind cluster labelled for ingress-nginx.
func (e *environment) createCluster(ctx context.Context) error {
	e.kubeconfig = filepath.Join(e.workDir, "kubeconfig")
	e.kind = cluster.NewProvider()

	// A cluster left behind by an interrupted run holds the name.
	if err := e.kind.Delete(kindClusterName, e.kubeconfig); err != nil {
		return fmt.Errorf("delete a leftover Kind cluster: %w", err)
	}
	config := &v1alpha4.Cluster{
		Nodes: []v1alpha4.Node{{
			Role:   v1alpha4.ControlPlaneRole,
			Labels: map[string]string{"ingress-ready": "true"},
		}},
	}
	err := e.kind.Create(kindClusterName,
		cluster.CreateWithV1Alpha4Config(config),
		cluster.CreateWithNodeImage(envOr(envKindNodeImage, defaultKindNodeImage)),
		cluster.CreateWithKubeconfigPath(e.kubeconfig),
		cluster.CreateWithWaitForReady(3*time.Minute),
		cluster.CreateWithDisplayUsage(false),
		cluster.CreateWithDisplaySalutation(false),
	)
	if err != nil {
		return fmt.Errorf("create the Kind cluster: %w", err)
	}

	e.restConfig, err = clientcmd.BuildConfigFromFlags("", e.kubeconfig)
	if err != nil {
		return fmt.Errorf("load kubeconfig: %w", err)
	}
	e.clientset, err = kubernetes.NewForConfig(e.restConfig)
	if err != nil {
		return fmt.Errorf("create Kubernetes clientset: %w", err)
	}

	out, err := command(ctx, "docker", "inspect", "-f",
		fmt.Sprintf(`{{(index .NetworkSettings.Networks %q).IPAddress}}`, kindNetwork),
		kindClusterName+"-control-plane")
	if err != nil {
		return err
	}
	e.nodeIP = strings.TrimSpace(out)
	if e.nodeIP == "" {
		return fmt.Errorf("the Kind node has no IPv4 address on the %s network", kindNetwork)
	}
	return nil
}

// installIngress installs ingress-nginx with host ports on the node, so peers on
// Kind's network reach it at the node's address.
func (e *environment) installIngress(ctx context.Context) error {
	_, err := e.helm(ctx, "upgrade", "--install", "ingress-nginx", "ingress-nginx",
		"--repo", "https://kubernetes.github.io/ingress-nginx",
		"--version", ingressChartVersion,
		"--namespace", ingressNamespace, "--create-namespace",
		"--set", "controller.hostPort.enabled=true",
		"--set", "controller.service.type=ClusterIP",
		"--set-string", "controller.nodeSelector.ingress-ready=true",
		// Nothing here is worth the webhook's start-up race with the chart's
		// Ingresses.
		"--set", "controller.admissionWebhooks.enabled=false",
		"--wait", "--timeout", "5m")
	return err
}

// prepareNamespace creates the chart's namespace, enforcing the restricted Pod
// Security Standard when rootless, and the TLS Secret its ingresses reference.
func (e *environment) prepareNamespace(ctx context.Context) error {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
	if e.rootless {
		ns.Labels = map[string]string{
			"pod-security.kubernetes.io/enforce":         "restricted",
			"pod-security.kubernetes.io/enforce-version": "latest",
		}
	}
	if _, err := e.clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("create namespace %s: %w", namespace, err)
	}

	var err error
	e.ca, err = newCertAuthority()
	if err != nil {
		return err
	}
	certPEM, keyPEM, err := e.ca.issue(domain, dashboardDomain)
	if err != nil {
		return err
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: tlsSecretName, Namespace: namespace},
		Type:       corev1.SecretTypeTLS,
		Data: map[string][]byte{
			corev1.TLSCertKey:       certPEM,
			corev1.TLSPrivateKeyKey: keyPEM,
		},
	}
	if _, err := e.clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("create the TLS Secret: %w", err)
	}
	return os.WriteFile(e.caPath(), e.ca.certPEM, 0o644)
}

// installChart installs or upgrades the chart from examples/kind under the
// suite's hostnames, with the backends at version.
func (e *environment) installChart(ctx context.Context, version string) error {
	files, err := e.valuesFiles()
	if err != nil {
		return err
	}
	args := []string{"upgrade", "--install", releaseName, chartDir, "--namespace", namespace,
		"--wait", "--timeout", "10m"}
	for _, f := range files {
		args = append(args, "--values", f)
	}
	for _, component := range []string{"management", "signal", "relay"} {
		args = append(args, "--set", fmt.Sprintf("backend.split.%s.image.tag=%s", component, version))
	}
	_, err = e.helm(ctx, args...)
	return err
}

// valuesFiles returns examples/kind with the suite's hostnames, examples/rootless
// when rootless, and the suite's own additions.
func (e *environment) valuesFiles() ([]string, error) {
	example, err := os.ReadFile(filepath.Join(chartDir, "examples", "kind", "values.yaml"))
	if err != nil {
		return nil, err
	}
	kindValues := filepath.Join(e.workDir, "kind-values.yaml")
	if err := os.WriteFile(kindValues, []byte(strings.ReplaceAll(string(example), exampleDomain, domain)), 0o644); err != nil {
		return nil, err
	}
	files := []string{kindValues}
	if e.rootless {
		files = append(files, filepath.Join(chartDir, "examples", "rootless", "values.yaml"))
	}

	// No owner, so POST /api/setup creates the account and returns a token.
	suiteValues := filepath.Join(e.workDir, "e2e-values.yaml")
	err = os.WriteFile(suiteValues, []byte(`backend:
  split:
    management:
      env:
        NB_SETUP_PAT_ENABLED: "true"
      config:
        embeddedIdp:
          owner:
            email: ""
            password: ""
`), 0o644)
	return append(files, suiteValues), err
}

// helm runs helm against the suite's cluster, with its own repository config
// and cache, so the user's repositories play no part.
func (e *environment) helm(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "helm", append(args, "--kubeconfig", e.kubeconfig)...)
	cmd.Env = append(os.Environ(),
		"HELM_REPOSITORY_CONFIG="+filepath.Join(e.workDir, "helm", "repositories.yaml"),
		"HELM_REPOSITORY_CACHE="+filepath.Join(e.workDir, "helm", "cache"),
	)
	return runCommand(cmd)
}

func (e *environment) caPath() string {
	return filepath.Join(e.workDir, "ca.crt")
}

// waitDeploymentImage waits until a Deployment runs image in all its ready
// replicas, with no old pod left.
func (e *environment) waitDeploymentImage(ctx context.Context, name, image string) error {
	return wait.PollUntilContextTimeout(ctx, 3*time.Second, 5*time.Minute, true, func(ctx context.Context) (bool, error) {
		d, err := e.clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if d.Spec.Template.Spec.Containers[0].Image != image {
			return false, fmt.Errorf("deployment %s runs %s, want %s", name, d.Spec.Template.Spec.Containers[0].Image, image)
		}
		return d.Status.ObservedGeneration >= d.Generation &&
			d.Status.UpdatedReplicas == *d.Spec.Replicas &&
			d.Status.ReadyReplicas == *d.Spec.Replicas &&
			d.Status.Replicas == *d.Spec.Replicas, nil
	})
}

// teardown removes the peers and the cluster, unless NB_E2E_KEEP is set.
func (e *environment) teardown() {
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

var backendTag = regexp.MustCompile(`(?m)^        repository: netbirdio/management\n        tag: "?([^"\s]+)"?`)

// chartBackendVersion reads the management tag from the chart's values.yaml,
// the version the chart installs by default.
func chartBackendVersion() (string, error) {
	raw, err := os.ReadFile(filepath.Join(chartDir, "values.yaml"))
	if err != nil {
		return "", err
	}
	m := backendTag.FindSubmatch(raw)
	if m == nil {
		return "", fmt.Errorf("no management image tag in %s/values.yaml", chartDir)
	}
	return string(m[1]), nil
}

// command runs a CLI and returns its output, or an error carrying it.
func command(ctx context.Context, name string, args ...string) (string, error) {
	return runCommand(exec.CommandContext(ctx, name, args...))
}

func runCommand(cmd *exec.Cmd) (string, error) {
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s: %w\n%s", strings.Join(cmd.Args, " "), err, out)
	}
	return string(out), nil
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

// envBool reads a boolean knob. An unparsable value is reported rather than
// silently read as false.
func envBool(name string) bool {
	val := os.Getenv(name)
	if val == "" {
		return false
	}
	b, err := strconv.ParseBool(val)
	if err != nil {
		log.Printf("e2e: ignoring %s=%q: %v", name, val, err)
		return false
	}
	return b
}
