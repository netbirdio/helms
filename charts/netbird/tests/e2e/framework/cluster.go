//go:build e2e

package framework

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
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
	envKindNodeImage     = "NB_E2E_KIND_NODE_IMAGE"

	// ingress-nginx at the controller version the Taskfile's dev cluster runs
	// (1.12.1), with host ports on the node so peers reach it on 443.
	ingressNamespace    = "ingress-nginx"
	ingressChartVersion = "4.12.1"

	// tlsSecretName is the Secret examples/kind's ingresses reference.
	tlsSecretName = "netbird-local-tls"
)

// createCluster creates a one-node Kind cluster labelled for ingress-nginx.
func (e *Env) createCluster(ctx context.Context) error {
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
	e.Kube, err = kubernetes.NewForConfig(e.restConfig)
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
func (e *Env) installIngress(ctx context.Context) error {
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
func (e *Env) prepareNamespace(ctx context.Context) error {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: Namespace}}
	if e.Rootless {
		ns.Labels = map[string]string{
			"pod-security.kubernetes.io/enforce":         "restricted",
			"pod-security.kubernetes.io/enforce-version": "latest",
		}
	}
	if _, err := e.Kube.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("create namespace %s: %w", Namespace, err)
	}

	var err error
	e.ca, err = newCertAuthority()
	if err != nil {
		return err
	}
	certPEM, keyPEM, err := e.ca.issue(Domain, DashboardDomain)
	if err != nil {
		return err
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: tlsSecretName, Namespace: Namespace},
		Type:       corev1.SecretTypeTLS,
		Data: map[string][]byte{
			corev1.TLSCertKey:       certPEM,
			corev1.TLSPrivateKeyKey: keyPEM,
		},
	}
	if _, err := e.Kube.CoreV1().Secrets(Namespace).Create(ctx, secret, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("create the TLS Secret: %w", err)
	}
	return os.WriteFile(e.caPath(), e.ca.certPEM, 0o644)
}

func (e *Env) caPath() string {
	return filepath.Join(e.workDir, "ca.crt")
}
