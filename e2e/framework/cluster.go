//go:build e2e

// Package framework is the plumbing the charts' end-to-end tests share: a Kind
// cluster with ingress-nginx, helm, a CA for the ingress certificates, a
// port-forward to reach the ingress from the host, and logs on failure. It
// knows nothing about any chart. A chart's own plumbing lives in a subpackage,
// such as framework/netbird, and its tests in a top-level package, such as
// e2e/netbird.
package framework

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/kind/pkg/apis/config/v1alpha4"
	"sigs.k8s.io/kind/pkg/cluster"
)

const (
	// KindNetwork is the Docker network Kind puts its nodes on. Containers that
	// join it reach the ingress on the node.
	KindNetwork = "kind"

	// The node image has to match the Kind version in go.mod.
	defaultKindNodeImage = "kindest/node:v1.37.0@sha256:a1ed56cfb0e7b93589bdf97c8cd566405a265939e3620fc4f5de89adff580ae5"
	envKindNodeImage     = "NB_E2E_KIND_NODE_IMAGE"

	// IngressNamespace holds ingress-nginx, at the controller version the
	// Taskfile's dev cluster runs (1.12.1).
	IngressNamespace    = "ingress-nginx"
	ingressChartVersion = "4.12.1"
)

// Cluster is a one-node Kind cluster.
type Cluster struct {
	// Name is the Kind cluster's name. Each test package uses its own, so
	// packages can run side by side.
	Name string
	// Kube is a client for the cluster.
	Kube kubernetes.Interface
	// Kubeconfig is the path of the cluster's kubeconfig.
	Kubeconfig string
	// NodeIP is the node's address on KindNetwork.
	NodeIP string

	provider   *cluster.Provider
	restConfig *rest.Config
	workDir    string
}

// CreateCluster creates a one-node Kind cluster labelled for ingress-nginx,
// replacing one an interrupted run left under the same name. workDir holds its
// kubeconfig and helm's files. On error it returns the cluster as far as it got,
// so the caller can still collect logs and delete it.
func CreateCluster(ctx context.Context, name, workDir string) (*Cluster, error) {
	c := &Cluster{
		Name:       name,
		Kubeconfig: filepath.Join(workDir, "kubeconfig"),
		provider:   cluster.NewProvider(),
		workDir:    workDir,
	}
	if err := c.provider.Delete(name, c.Kubeconfig); err != nil {
		return nil, fmt.Errorf("delete a leftover Kind cluster: %w", err)
	}
	config := &v1alpha4.Cluster{
		Nodes: []v1alpha4.Node{{
			Role:   v1alpha4.ControlPlaneRole,
			Labels: map[string]string{"ingress-ready": "true"},
		}},
	}
	err := c.provider.Create(name,
		cluster.CreateWithV1Alpha4Config(config),
		cluster.CreateWithNodeImage(EnvOr(envKindNodeImage, defaultKindNodeImage)),
		cluster.CreateWithKubeconfigPath(c.Kubeconfig),
		cluster.CreateWithWaitForReady(3*time.Minute),
		cluster.CreateWithDisplayUsage(false),
		cluster.CreateWithDisplaySalutation(false),
	)
	if err != nil {
		return c, fmt.Errorf("create the Kind cluster: %w", err)
	}

	c.restConfig, err = clientcmd.BuildConfigFromFlags("", c.Kubeconfig)
	if err != nil {
		return c, fmt.Errorf("load kubeconfig: %w", err)
	}
	c.Kube, err = kubernetes.NewForConfig(c.restConfig)
	if err != nil {
		return c, fmt.Errorf("create Kubernetes clientset: %w", err)
	}

	out, err := Command(ctx, "docker", "inspect", "-f",
		fmt.Sprintf(`{{(index .NetworkSettings.Networks %q).IPAddress}}`, KindNetwork),
		name+"-control-plane")
	if err != nil {
		return c, err
	}
	c.NodeIP = strings.TrimSpace(out)
	if c.NodeIP == "" {
		return c, fmt.Errorf("the Kind node has no IPv4 address on the %s network", KindNetwork)
	}
	return c, nil
}

// Delete deletes the Kind cluster.
func (c *Cluster) Delete() error {
	return c.provider.Delete(c.Name, c.Kubeconfig)
}

// CollectLogs writes Kind's node and component logs under dir.
func (c *Cluster) CollectLogs(dir string) error {
	return c.provider.CollectLogs(c.Name, dir)
}

// InstallIngressNginx installs ingress-nginx with host ports on the node, so
// containers on KindNetwork reach it on 80 and 443 at NodeIP.
func (c *Cluster) InstallIngressNginx(ctx context.Context) error {
	_, err := c.Helm(ctx, "upgrade", "--install", "ingress-nginx", "ingress-nginx",
		"--repo", "https://kubernetes.github.io/ingress-nginx",
		"--version", ingressChartVersion,
		"--namespace", IngressNamespace, "--create-namespace",
		"--set", "controller.hostPort.enabled=true",
		"--set", "controller.service.type=ClusterIP",
		"--set-string", "controller.nodeSelector.ingress-ready=true",
		// Nothing here is worth the webhook's start-up race with the charts'
		// Ingresses.
		"--set", "controller.admissionWebhooks.enabled=false",
		"--wait", "--timeout", "5m")
	return err
}

// CreateNamespace creates a namespace, enforcing the restricted Pod Security
// Standard when restricted is set.
func (c *Cluster) CreateNamespace(ctx context.Context, name string, restricted bool) error {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	if restricted {
		ns.Labels = map[string]string{
			"pod-security.kubernetes.io/enforce":         "restricted",
			"pod-security.kubernetes.io/enforce-version": "latest",
		}
	}
	if _, err := c.Kube.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("create namespace %s: %w", name, err)
	}
	return nil
}

// CreateTLSSecret creates a TLS Secret holding a certificate ca issues for
// hosts.
func (c *Cluster) CreateTLSSecret(ctx context.Context, namespace, name string, ca *CertAuthority, hosts ...string) error {
	certPEM, keyPEM, err := ca.Issue(hosts...)
	if err != nil {
		return err
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Type:       corev1.SecretTypeTLS,
		Data: map[string][]byte{
			corev1.TLSCertKey:       certPEM,
			corev1.TLSPrivateKeyKey: keyPEM,
		},
	}
	if _, err := c.Kube.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("create the TLS Secret %s: %w", name, err)
	}
	return nil
}

// Helm runs helm against the cluster, with its own repository config and
// cache, so the user's repositories play no part.
func (c *Cluster) Helm(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "helm", append(args, "--kubeconfig", c.Kubeconfig)...)
	cmd.Env = append(os.Environ(),
		"HELM_REPOSITORY_CONFIG="+filepath.Join(c.workDir, "helm", "repositories.yaml"),
		"HELM_REPOSITORY_CACHE="+filepath.Join(c.workDir, "helm", "cache"),
	)
	return runCommand(cmd)
}
