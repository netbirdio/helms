//go:build e2e

package e2e

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

// ingressForward is a port-forward to the ingress controller's HTTPS port. The
// suite reaches the chart through it: the host cannot route to Kind's network
// on every platform, and the ingress is what the peers use too.
type ingressForward struct {
	port     uint16
	stopChan chan struct{}
}

func (e *environment) forwardIngress(ctx context.Context) (*ingressForward, error) {
	pods, err := e.clientset.CoreV1().Pods(ingressNamespace).List(ctx, metav1.ListOptions{
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
		return nil, fmt.Errorf("no running ingress controller pod in %s", ingressNamespace)
	}

	transport, upgrader, err := spdy.RoundTripperFor(e.restConfig)
	if err != nil {
		return nil, err
	}
	url := e.clientset.CoreV1().RESTClient().Post().
		Resource("pods").Namespace(ingressNamespace).Name(pod).SubResource("portforward").URL()
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, url)

	f := &ingressForward{stopChan: make(chan struct{})}
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

// client returns an HTTP client that sends every request to the ingress, with
// the host name of its URL for SNI and Host, trusting only ca.
func (f *ingressForward) client(ca *certAuthority) *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return dialer.DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", f.port))
			},
			TLSClientConfig:   &tls.Config{RootCAs: ca.pool(), MinVersion: tls.VersionTLS12},
			ForceAttemptHTTP2: true,
		},
		// The dashboard and the IdP redirect; the tests check the first answer.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func (f *ingressForward) stop() {
	close(f.stopChan)
}
