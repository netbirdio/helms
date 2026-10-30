//go:build e2e

package framework

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

// IngressForward is a port-forward to the ingress controller's HTTPS port. The
// tests reach a chart through it: the host cannot route to Kind's network on
// every platform, and the ingress is what clients outside the cluster use too.
type IngressForward struct {
	port     uint16
	stopChan chan struct{}
}

// ForwardIngress forwards a free local port to ingress-nginx's HTTPS port.
func (c *Cluster) ForwardIngress(ctx context.Context) (*IngressForward, error) {
	pods, err := c.Kube.CoreV1().Pods(IngressNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/component=controller",
	})
	if err != nil {
		return nil, fmt.Errorf("list ingress controller pods: %w", err)
	}
	var pod string
	for _, p := range pods.Items {
		if p.Status.Phase == corev1.PodRunning {
			pod = p.Name
			break
		}
	}
	if pod == "" {
		return nil, fmt.Errorf("no running ingress controller pod in %s", IngressNamespace)
	}

	transport, upgrader, err := spdy.RoundTripperFor(c.restConfig)
	if err != nil {
		return nil, err
	}
	url := c.Kube.CoreV1().RESTClient().Post().
		Resource("pods").Namespace(IngressNamespace).Name(pod).SubResource("portforward").URL()
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, url)

	f := &IngressForward{stopChan: make(chan struct{})}
	ready := make(chan struct{})
	fw, err := portforward.New(dialer, []string{"0:443"}, f.stopChan, ready, io.Discard, io.Discard)
	if err != nil {
		return nil, err
	}
	errChan := make(chan error, 1)
	go func() { errChan <- fw.ForwardPorts() }()
	select {
	case <-ready:
	case err := <-errChan:
		return nil, fmt.Errorf("forward to the ingress controller: %w", err)
	case <-ctx.Done():
		close(f.stopChan)
		return nil, ctx.Err()
	}
	ports, err := fw.GetPorts()
	if err != nil {
		return nil, err
	}
	f.port = ports[0].Local
	return f, nil
}

// Client returns an HTTP client that sends every request to the ingress, with
// the host name of its URL for SNI and Host, trusting only ca.
func (f *IngressForward) Client(ca *CertAuthority) *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return dialer.DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", f.port))
			},
			TLSClientConfig:   &tls.Config{RootCAs: ca.Pool(), MinVersion: tls.VersionTLS12},
			ForceAttemptHTTP2: true,
		},
		// Tests check the first answer, not where a redirect leads.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// Stop closes the port-forward.
func (f *IngressForward) Stop() {
	close(f.stopChan)
}
