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

// communityChartDir is the netbird chart; its examples/rootless serves both charts.
var communityChartDir = framework.ChartDir(ChartCommunity)

// Enterprise is whether the chart under test is netbird-ee.
func (e *Env) Enterprise() bool {
	return e.Chart == ChartEnterprise
}

// valuesPrefix is where the netbird chart's values sit in chart: netbird-ee
// wraps it as a dependency.
func valuesPrefix(chart string) string {
	if chart == ChartEnterprise {
		return "netbird."
	}
	return ""
}

// CommunityVersion is the community release an enterprise version such as
// 0.82.0-enterprise.1 is built on, and any other version unchanged. The
// community images (the netbird chart's, netbird-ee's relay, the client) use it.
func CommunityVersion(version string) string {
	base, _, _ := strings.Cut(version, "-enterprise.")
	return base
}

// InstallChart installs chart, or upgrades the release to it, from its
// examples/kind under the suite's hostnames, with the backends at version.
func (e *Env) InstallChart(ctx context.Context, chart, version string) error {
	files, err := e.valuesFiles(chart)
	if err != nil {
		return err
	}
	args := []string{"upgrade", "--install", ReleaseName, framework.ChartDir(chart), "--namespace", Namespace,
		"--wait", "--timeout", "10m"}
	for _, f := range files {
		args = append(args, "--values", f)
	}
	for _, component := range []string{"management", "signal", "relay"} {
		tag := version
		if chart == ChartCommunity || component == "relay" {
			tag = CommunityVersion(version)
		}
		args = append(args, "--set", fmt.Sprintf("%sbackend.split.%s.image.tag=%s", valuesPrefix(chart), component, tag))
	}
	_, err = e.Cluster.Helm(ctx, args...)
	return err
}

// valuesFiles returns chart's examples/kind with the suite's hostnames, the
// netbird chart's examples/rootless when rootless, and the suite's own
// additions, the last two nested where chart expects them.
func (e *Env) valuesFiles(chart string) ([]string, error) {
	example, err := os.ReadFile(filepath.Join(framework.ChartDir(chart), "examples", "kind", "values.yaml"))
	if err != nil {
		return nil, err
	}
	kindValues := filepath.Join(e.workDir, "kind-values.yaml")
	if err := os.WriteFile(kindValues, []byte(strings.ReplaceAll(string(example), exampleDomain, Domain)), 0o644); err != nil {
		return nil, err
	}
	files := []string{kindValues}
	if e.Rootless {
		rootless, err := os.ReadFile(filepath.Join(communityChartDir, "examples", "rootless", "values.yaml"))
		if err != nil {
			return nil, err
		}
		rootlessValues, err := writeValues(chart, filepath.Join(e.workDir, "rootless-values.yaml"), string(rootless))
		if err != nil {
			return nil, err
		}
		files = append(files, rootlessValues)
	}

	// No owner, so POST /api/setup creates the account and returns a token.
	suiteValues, err := writeValues(chart, filepath.Join(e.workDir, "e2e-values.yaml"), `backend:
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

// writeValues writes netbird chart values to path, under netbird: when chart
// is netbird-ee, and returns path. Indenting every line keeps block scalars
// intact.
func writeValues(chart, path, values string) (string, error) {
	if chart == ChartEnterprise {
		values = "netbird:\n" + regexp.MustCompile(`(?m)^(.)`).ReplaceAllString(values, "  $1")
	}
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

var backendTag = regexp.MustCompile(`(?m)^ +repository: \S*/management\S*\n +tag: "?([^"\s]+)"?`)

// chartBackendVersion reads the management tag from chart's values.yaml, the
// version the chart installs by default.
func chartBackendVersion(chart string) (string, error) {
	chartDir := framework.ChartDir(chart)
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
