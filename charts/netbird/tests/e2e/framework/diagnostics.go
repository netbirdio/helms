//go:build e2e

package framework

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LogDiagnostics prints what most often explains a failed bootstrap: the
// chart's pods and the end of their logs.
func (e *Env) LogDiagnostics() {
	if e.Kube == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	for _, ns := range []string{Namespace, ingressNamespace} {
		pods, err := e.Kube.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			log.Printf("e2e: list pods in %s: %v", ns, err)
			continue
		}
		for _, p := range pods.Items {
			log.Printf("e2e: pod %s/%s: %s", ns, p.Name, podSummary(p))
			if !podReady(p) {
				log.Printf("e2e: %s/%s log:\n%s", ns, p.Name, e.podLogs(ctx, ns, p.Name, 30))
			}
		}
	}
}

// CollectArtifacts writes the cluster's, the release's and the peers' state for
// CI to keep, when NB_E2E_ARTIFACTS_DIR asks for it. The environment is torn
// down before the run reports, so this is the only chance to read them.
func (e *Env) CollectArtifacts() {
	dir := os.Getenv(envArtifactsDir)
	if dir == "" {
		return
	}
	if err := os.MkdirAll(filepath.Join(dir, "peers"), 0o755); err != nil {
		log.Printf("e2e: create %s: %v", dir, err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			log.Printf("e2e: write %s: %v", name, err)
		}
	}

	if e.kubeconfig != "" {
		for name, args := range map[string][]string{
			"helm-values.yaml":   {"get", "values", ReleaseName, "--namespace", Namespace, "--all"},
			"helm-manifest.yaml": {"get", "manifest", ReleaseName, "--namespace", Namespace},
			"helm-history.txt":   {"history", ReleaseName, "--namespace", Namespace},
		} {
			out, _ := e.helm(ctx, args...)
			write(name, out)
		}
	}
	if e.Kube != nil {
		write("cluster.txt", e.clusterState(ctx))
	}
	for _, p := range e.peers {
		s, _ := command(ctx, "docker", "exec", p.container, "netbird", "status", "--detail")
		write(filepath.Join("peers", p.Name+".log"), p.logs(ctx)+"\n--- netbird status --detail\n"+s)
	}
	if e.kind != nil {
		if err := e.kind.CollectLogs(kindClusterName, filepath.Join(dir, "kind")); err != nil {
			log.Printf("e2e: collect the Kind logs: %v", err)
		}
	}
}

// clusterState lists the pods and events of the chart's and the ingress's
// namespaces, and every pod's log.
func (e *Env) clusterState(ctx context.Context) string {
	var b strings.Builder
	for _, ns := range []string{Namespace, ingressNamespace} {
		pods, err := e.Kube.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			fmt.Fprintf(&b, "list pods in %s: %v\n", ns, err)
			continue
		}
		for _, p := range pods.Items {
			fmt.Fprintf(&b, "=== pod %s/%s: %s\n%s\n", ns, p.Name, podSummary(p), e.podLogs(ctx, ns, p.Name, 500))
		}
		events, err := e.Kube.CoreV1().Events(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			continue
		}
		fmt.Fprintf(&b, "=== events in %s\n", ns)
		for _, ev := range events.Items {
			fmt.Fprintf(&b, "%s %s %s/%s: %s\n", ev.LastTimestamp.Format(time.RFC3339), ev.Type,
				ev.InvolvedObject.Kind, ev.InvolvedObject.Name, ev.Message)
		}
	}
	return b.String()
}

func (e *Env) podLogs(ctx context.Context, ns, pod string, lines int64) string {
	stream, err := e.Kube.CoreV1().Pods(ns).GetLogs(pod, &corev1.PodLogOptions{TailLines: &lines}).Stream(ctx)
	if err != nil {
		return fmt.Sprintf("(no log: %v)", err)
	}
	defer func() { _ = stream.Close() }()
	out, _ := io.ReadAll(stream)
	return string(out)
}

func podSummary(p corev1.Pod) string {
	s := string(p.Status.Phase)
	for _, c := range p.Status.ContainerStatuses {
		s += fmt.Sprintf(" %s(ready=%t restarts=%d", c.Name, c.Ready, c.RestartCount)
		if w := c.State.Waiting; w != nil {
			s += " " + w.Reason
		}
		s += ")"
	}
	return s
}

func podReady(p corev1.Pod) bool {
	for _, c := range p.Status.Conditions {
		if c.Type == corev1.PodReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}
