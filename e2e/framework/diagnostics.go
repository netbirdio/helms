//go:build e2e

package framework

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LogPods prints what most often explains a failed bootstrap: the pods in
// namespaces and the end of the logs of those not ready.
func (c *Cluster) LogPods(namespaces ...string) {
	if c.Kube == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	for _, ns := range namespaces {
		pods, err := c.Kube.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			log.Printf("e2e: list pods in %s: %v", ns, err)
			continue
		}
		for _, p := range pods.Items {
			log.Printf("e2e: pod %s/%s: %s", ns, p.Name, podSummary(p))
			if !podReady(p) {
				log.Printf("e2e: %s/%s log:\n%s", ns, p.Name, c.podLogs(ctx, ns, p.Name, 30))
			}
		}
	}
}

// State lists the pods and events of namespaces, and every pod's log.
func (c *Cluster) State(ctx context.Context, namespaces ...string) string {
	var b strings.Builder
	for _, ns := range namespaces {
		pods, err := c.Kube.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			fmt.Fprintf(&b, "list pods in %s: %v\n", ns, err)
			continue
		}
		for _, p := range pods.Items {
			fmt.Fprintf(&b, "=== pod %s/%s: %s\n%s\n", ns, p.Name, podSummary(p), c.podLogs(ctx, ns, p.Name, 500))
		}
		events, err := c.Kube.CoreV1().Events(ns).List(ctx, metav1.ListOptions{})
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

func (c *Cluster) podLogs(ctx context.Context, ns, pod string, lines int64) string {
	stream, err := c.Kube.CoreV1().Pods(ns).GetLogs(pod, &corev1.PodLogOptions{TailLines: &lines}).Stream(ctx)
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
