//go:build e2e

package framework

import (
	"context"
	"fmt"
	"slices"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

// WaitWorkloadImage waits until the Deployment name, or the StatefulSet name
// when there is no such Deployment, runs image in all its ready replicas, with
// no old pod left.
func (c *Cluster) WaitWorkloadImage(ctx context.Context, namespace, name, image string) error {
	return wait.PollUntilContextTimeout(ctx, 3*time.Second, 5*time.Minute, true, func(ctx context.Context) (bool, error) {
		var (
			kind                     = "deployment"
			meta                     metav1.ObjectMeta
			containers               []corev1.Container
			want                     int32
			observed                 int64
			updated, ready, replicas int32
		)
		d, err := c.Kube.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		switch {
		case err == nil:
			meta, containers, want = d.ObjectMeta, d.Spec.Template.Spec.Containers, *d.Spec.Replicas
			observed, updated, ready, replicas = d.Status.ObservedGeneration, d.Status.UpdatedReplicas, d.Status.ReadyReplicas, d.Status.Replicas
		case apierrors.IsNotFound(err):
			s, err := c.Kube.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			kind = "statefulset"
			meta, containers, want = s.ObjectMeta, s.Spec.Template.Spec.Containers, *s.Spec.Replicas
			observed, updated, ready, replicas = s.Status.ObservedGeneration, s.Status.UpdatedReplicas, s.Status.ReadyReplicas, s.Status.Replicas
		default:
			return false, err
		}
		if containers[0].Image != image {
			return false, fmt.Errorf("%s %s runs %s, want %s", kind, name, containers[0].Image, image)
		}
		return observed >= meta.Generation && updated == want && ready == want && replicas == want, nil
	})
}

// Logs returns the logs of the pods selector matches, one after another.
func (c *Cluster) Logs(ctx context.Context, namespace, selector string) (string, error) {
	list, err := c.Kube.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return "", err
	}
	if len(list.Items) == 0 {
		return "", fmt.Errorf("no pod matches %s", selector)
	}
	var logs string
	for _, p := range list.Items {
		logs += c.podLogs(ctx, namespace, p.Name, 10000)
	}
	return logs, nil
}

// RestartPods deletes the pods selector matches and waits until their
// replacements are ready.
func (c *Cluster) RestartPods(ctx context.Context, namespace, selector string) error {
	pods := c.Kube.CoreV1().Pods(namespace)
	opts := metav1.ListOptions{LabelSelector: selector}
	old, err := pods.List(ctx, opts)
	if err != nil {
		return err
	}
	if len(old.Items) == 0 {
		return fmt.Errorf("no pod matches %s", selector)
	}
	var oldNames []string
	for _, p := range old.Items {
		oldNames = append(oldNames, p.Name)
		if err := pods.Delete(ctx, p.Name, metav1.DeleteOptions{}); err != nil {
			return err
		}
	}
	return wait.PollUntilContextTimeout(ctx, 3*time.Second, 5*time.Minute, true, func(ctx context.Context) (bool, error) {
		list, err := pods.List(ctx, opts)
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
}
