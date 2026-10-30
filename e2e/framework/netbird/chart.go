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

// examples/kind serves exampleDomain; the suite installs it under Domain.
const exampleDomain = "netbird.localhost"

// chartDir is the netbird chart in this repository.
var chartDir = framework.ChartDir("netbird")

// InstallChart installs or upgrades the chart from examples/kind under the
// suite's hostnames, with the backends at version.
func (e *Env) InstallChart(ctx context.Context, version string) error {
	files, err := e.valuesFiles()
	if err != nil {
		return err
	}
	args := []string{"upgrade", "--install", ReleaseName, chartDir, "--namespace", Namespace,
		"--wait", "--timeout", "10m"}
	for _, f := range files {
		args = append(args, "--values", f)
	}
	for _, component := range []string{"management", "signal", "relay"} {
		args = append(args, "--set", fmt.Sprintf("backend.split.%s.image.tag=%s", component, version))
	}
	_, err = e.Cluster.Helm(ctx, args...)
	return err
}

// valuesFiles returns examples/kind with the suite's hostnames, examples/rootless
// when rootless, and the suite's own additions.
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

// WaitDeploymentImage waits until one of the chart's Deployments runs image in
// all its ready replicas, with no old pod left.
func (e *Env) WaitDeploymentImage(ctx context.Context, name, image string) error {
	return e.Cluster.WaitDeploymentImage(ctx, Namespace, name, image)
}

// RestartManagement deletes management's pod and waits for its replacement to
// be ready.
func (e *Env) RestartManagement(ctx context.Context) error {
	selector := "app.kubernetes.io/name=netbird-management,app.kubernetes.io/instance=" + ReleaseName
	if err := e.Cluster.RestartPods(ctx, Namespace, selector); err != nil {
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
