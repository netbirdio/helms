//go:build e2e

package netbird

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/netbirdio/helms/e2e/framework"
)

// LogDiagnostics prints what most often explains a failed bootstrap: the
// chart's and the ingress's pods and the end of their logs.
func (e *Env) LogDiagnostics() {
	if e.Cluster != nil {
		e.Cluster.LogPods(Namespace, framework.IngressNamespace)
	}
}

// CollectArtifacts writes the cluster's, the release's and the peers' state for
// CI to keep, when NB_E2E_ARTIFACTS_DIR asks for it. The environment is torn
// down before the run reports, so this is the only chance to read them.
func (e *Env) CollectArtifacts() {
	dir := framework.ArtifactsDir()
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

	if e.Cluster != nil {
		for name, args := range map[string][]string{
			"helm-values.yaml":   {"get", "values", ReleaseName, "--namespace", Namespace, "--all"},
			"helm-manifest.yaml": {"get", "manifest", ReleaseName, "--namespace", Namespace},
			"helm-history.txt":   {"history", ReleaseName, "--namespace", Namespace},
		} {
			out, _ := e.Cluster.Helm(ctx, args...)
			write(name, out)
		}
		if e.Cluster.Kube != nil {
			write("cluster.txt", e.Cluster.State(ctx, Namespace, framework.IngressNamespace))
		}
	}
	for _, p := range e.peers {
		s, _ := framework.Command(ctx, "docker", "exec", p.container, "netbird", "status", "--detail")
		write(filepath.Join("peers", p.Name+".log"), p.logs(ctx)+"\n--- netbird status --detail\n"+s)
	}
	if e.Cluster != nil {
		if err := e.Cluster.CollectLogs(filepath.Join(dir, "kind")); err != nil {
			log.Printf("e2e: collect the Kind logs: %v", err)
		}
	}
}
