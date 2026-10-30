//go:build e2e

package framework

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	// chartDir is the chart, relative to the test package, where go test runs.
	chartDir = "../.."

	// examples/kind serves exampleDomain; the suite installs it under Domain.
	exampleDomain = "netbird.localhost"
)

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
	_, err = e.helm(ctx, args...)
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

// WaitDeploymentImage waits until a Deployment runs image in all its ready
// replicas, with no old pod left.
func (e *Env) WaitDeploymentImage(ctx context.Context, name, image string) error {
	return wait.PollUntilContextTimeout(ctx, 3*time.Second, 5*time.Minute, true, func(ctx context.Context) (bool, error) {
		d, err := e.Kube.AppsV1().Deployments(Namespace).Get(ctx, name, metav1.GetOptions{})
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

// RestartManagement deletes management's pod and waits for its replacement to
// be ready.
func (e *Env) RestartManagement(ctx context.Context) error {
	pods := e.Kube.CoreV1().Pods(Namespace)
	selector := metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=netbird-management,app.kubernetes.io/instance=" + ReleaseName}
	old, err := pods.List(ctx, selector)
	if err != nil {
		return err
	}
	if len(old.Items) == 0 {
		return fmt.Errorf("no management pod matches %s", selector.LabelSelector)
	}
	var oldNames []string
	for _, p := range old.Items {
		oldNames = append(oldNames, p.Name)
		if err := pods.Delete(ctx, p.Name, metav1.DeleteOptions{}); err != nil {
			return err
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
		return fmt.Errorf("management did not come back after its pod was deleted: %w", err)
	}
	return nil
}

// helm runs helm against the suite's cluster, with its own repository config
// and cache, so the user's repositories play no part.
func (e *Env) helm(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "helm", append(args, "--kubeconfig", e.kubeconfig)...)
	cmd.Env = append(os.Environ(),
		"HELM_REPOSITORY_CONFIG="+filepath.Join(e.workDir, "helm", "repositories.yaml"),
		"HELM_REPOSITORY_CACHE="+filepath.Join(e.workDir, "helm", "cache"),
	)
	return runCommand(cmd)
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
