//go:build e2e

package framework

import (
	"context"
	"fmt"
	"slices"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

// WaitDeploymentImage waits until a Deployment runs image in all its ready
// replicas, with no old pod left.
func (c *Cluster) WaitDeploymentImage(ctx context.Context, namespace, name, image string) error {
	return wait.PollUntilContextTimeout(ctx, 3*time.Second, 5*time.Minute, true, func(ctx context.Context) (bool, error) {
		d, err := c.Kube.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
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
