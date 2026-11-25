//go:build e2e

package netbird

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/netbirdio/helms/e2e/framework"
)

const managementSelector = "app.kubernetes.io/name=netbird-management,app.kubernetes.io/instance=" + ReleaseName

// examples/kind serves exampleDomain; the suite installs it under Domain.
const exampleDomain = "netbird.localhost"

// chartDir is the netbird chart, which serves both editions.
var chartDir = framework.ChartDir("netbird")

// CommunityVersion is the community release an enterprise version such as
// 0.82.0-enterprise.1 is built on, and any other version unchanged. The
// community images (the community edition's, the relay, the client) use it.
func CommunityVersion(version string) string {
	base, _, _ := strings.Cut(version, "-enterprise.")
	return base
}

// InstallChart installs the chart at edition, or upgrades the release to it,
// from examples/kind under the suite's hostnames, with the backends at
// version. Values set by an earlier EnablePerPodRelays are kept.
func (e *Env) InstallChart(ctx context.Context, edition, version string) error {
	files, err := e.valuesFiles()
	if err != nil {
		return err
	}
	if e.perPodRelays {
		perPod, err := writeValues(filepath.Join(e.workDir, "per-pod-relay-values.yaml"), perPodRelayValues())
		if err != nil {
			return err
		}
		files = append(files, perPod)
	}
	args := []string{"upgrade", "--install", ReleaseName, chartDir, "--namespace", Namespace,
		"--wait", "--timeout", "10m", "--set", "edition=" + edition}
	for _, f := range files {
		args = append(args, "--values", f)
	}
	for _, component := range []string{"management", "signal", "relay"} {
		tag := version
		if edition == EditionCommunity || component == "relay" {
			tag = CommunityVersion(version)
		}
		args = append(args, "--set", fmt.Sprintf("backend.split.%s.image.tag=%s", component, tag))
	}
	_, err = e.Cluster.Helm(ctx, args...)
	return err
}

// valuesFiles returns examples/kind with the suite's hostnames,
// examples/rootless when rootless, and the suite's own additions.
func (e *Env) valuesFiles() ([]string, error) {
	example, err := os.ReadFile(filepath.Join(chartDir, "examples", "kind", "values.yaml"))
	if err != nil {
		return nil, err
	}
	kindValues := filepath.Join(e.workDir, "kind-values.yaml")
	if err := os.WriteFile(kindValues, []byte(strings.ReplaceAll(string(example), exampleDomain, Domain)), 0o644); err != nil {
		return nil, err
	}
	files := []string{kindValues}
	if e.Rootless {
		rootless, err := os.ReadFile(filepath.Join(chartDir, "examples", "rootless", "values.yaml"))
		if err != nil {
			return nil, err
		}
		rootlessValues, err := writeValues(filepath.Join(e.workDir, "rootless-values.yaml"), string(rootless))
		if err != nil {
			return nil, err
		}
		files = append(files, rootlessValues)
	}

	// No owner, so POST /api/setup creates the account and returns a token.
	suiteValues, err := writeValues(filepath.Join(e.workDir, "e2e-values.yaml"), `backend:
  split:
    management:
      env:
        NB_SETUP_PAT_ENABLED: "true"
      config:
        embeddedIdp:
          owner:
            email: ""
            password: ""
`)
	return append(files, suiteValues), err
}

// EnablePerPodRelays upgrades the release to RelayReplicas relays, relay i
// at RelayHost(i), each with its own Service and ingress rule.
func (e *Env) EnablePerPodRelays(ctx context.Context) error {
	e.perPodRelays = true
	return e.InstallChart(ctx, e.Edition, e.DefaultVersion)
}

// perPodRelayValues lists the relay hostnames under the ingress TLS, so
// ingress-nginx serves the suite's certificate for them.
func perPodRelayValues() string {
	var tlsHosts strings.Builder
	for _, host := range hostnames() {
		fmt.Fprintf(&tlsHosts, "\n              - %s", host)
	}
	return fmt.Sprintf(`backend:
  split:
    relay:
      replicaCount: %d
      config:
        exposedAddressTemplate: rels://relay-%%d.%s:443/relay
      ingress:
        tls:
          - secretName: %s
            hosts:%s
`, RelayReplicas, Domain, tlsSecretName, tlsHosts.String())
}

// writeValues writes chart values to path and returns path.
func writeValues(path, values string) (string, error) {
	return path, os.WriteFile(path, []byte(values), 0o644)
}

// WaitDeploymentImage waits until one of the chart's Deployments runs image in
// all its ready replicas, with no old pod left.
func (e *Env) WaitDeploymentImage(ctx context.Context, name, image string) error {
	return e.Cluster.WaitDeploymentImage(ctx, Namespace, name, image)
}

// ManagementLogs returns the logs of management's pods.
func (e *Env) ManagementLogs(ctx context.Context) (string, error) {
	return e.Cluster.Logs(ctx, Namespace, managementSelector)
}

// RestartManagement deletes management's pod and waits for its replacement to
// be ready.
func (e *Env) RestartManagement(ctx context.Context) error {
	if err := e.Cluster.RestartPods(ctx, Namespace, managementSelector); err != nil {
		return fmt.Errorf("management did not come back after its pod was deleted: %w", err)
	}
	return nil
}

// CredentialSecrets returns the data of the Secrets holding management's and
// relay's credentials, by Secret name.
func (e *Env) CredentialSecrets(ctx context.Context) (map[string]map[string][]byte, error) {
	data := map[string]map[string][]byte{}
	for _, name := range []string{"netbird-management-credentials", "netbird-relay-credentials"} {
		secret, err := e.Cluster.Kube.CoreV1().Secrets(Namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("get the Secret %s: %w", name, err)
		}
		data[name] = secret.Data
	}
	return data, nil
}

var managementImageTag = regexp.MustCompile(`image: "[^"]*/management\S*:([^"@]+)"`)

// chartBackendVersion renders management at edition and returns its image
// tag, the version the chart installs by default.
func chartBackendVersion(ctx context.Context, edition string) (string, error) {
	out, err := framework.Command(ctx, "helm", "template", ReleaseName, chartDir,
		"--set", "edition="+edition,
		"--values", filepath.Join(chartDir, "examples", "kind", "values.yaml"),
		"--show-only", "templates/split/management/deployment.yaml")
	if err != nil {
		return "", err
	}
	m := managementImageTag.FindStringSubmatch(out)
	if m == nil {
		return "", fmt.Errorf("no management image in the %s edition's render", edition)
	}
	return m[1], nil
}
