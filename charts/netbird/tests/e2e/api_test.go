//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
)

// apiClient is the part of the management REST API the suite uses, against
// management's public URL. It is written out rather than imported so the suite
// does not pin a NetBird module version apart from the images it tests.
type apiClient struct {
	http  *http.Client
	base  string
	token string
}

type apiPeer struct {
	ID        string `json:"id"`
	Hostname  string `json:"hostname"`
	IP        string `json:"ip"`
	Connected bool   `json:"connected"`
}

func newAPIClient(client *http.Client, base string) *apiClient {
	return &apiClient{http: client, base: base}
}

// setup creates the account and its owner through the unauthenticated
// POST /api/setup, which returns a token while NB_SETUP_PAT_ENABLED is set.
// Management answers through the ingress a moment after its pod is ready.
func (a *apiClient) setup(ctx context.Context) error {
	req := map[string]any{
		"email":         "admin@netbird.e2e.test",
		"password":      "Netbird-e2e-Passw0rd!",
		"name":          "E2E Admin",
		"create_pat":    true,
		"pat_expire_in": 1,
	}
	var resp struct {
		Token string `json:"personal_access_token"`
	}
	var last error
	err := wait.PollUntilContextTimeout(ctx, 3*time.Second, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
		status, err := a.do(ctx, http.MethodPost, "/api/setup", req, &resp)
		switch {
		case err == nil:
			return true, nil
		case status >= 500 || status == 0:
			last = err
			return false, nil
		default:
			return false, err
		}
	})
	if err != nil {
		return fmt.Errorf("POST /api/setup: %w (last: %v)", err, last)
	}
	if resp.Token == "" {
		return fmt.Errorf("POST /api/setup returned no token; is NB_SETUP_PAT_ENABLED set?")
	}
	a.token = resp.Token
	return nil
}

// createSetupKey returns a reusable setup key.
func (a *apiClient) createSetupKey(ctx context.Context, name string) (string, error) {
	var resp struct {
		Key string `json:"key"`
	}
	_, err := a.do(ctx, http.MethodPost, "/api/setup-keys", map[string]any{
		"name":        name,
		"type":        "reusable",
		"expires_in":  86400,
		"auto_groups": []string{},
	}, &resp)
	if err != nil {
		return "", fmt.Errorf("create setup key %s: %w", name, err)
	}
	return resp.Key, nil
}

func (a *apiClient) peers(ctx context.Context) ([]apiPeer, error) {
	var peers []apiPeer
	if _, err := a.do(ctx, http.MethodGet, "/api/peers", nil, &peers); err != nil {
		return nil, fmt.Errorf("list peers: %w", err)
	}
	return peers, nil
}

// do sends a JSON request and decodes a 2xx answer into out. It returns the
// status code, 0 if there was no answer. A request that got no answer, or a
// gateway error from the ingress, is sent again a few times: the port-forward
// and the ingress drop the odd stream.
func (a *apiClient) do(ctx context.Context, method, path string, body, out any) (int, error) {
	var raw []byte
	if body != nil {
		var err error
		if raw, err = json.Marshal(body); err != nil {
			return 0, err
		}
	}
	var status int
	var err error
	for attempt := 1; ; attempt++ {
		status, err = a.send(ctx, method, path, raw, out)
		retryable := status == 0 || status == http.StatusBadGateway ||
			status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
		if err == nil || !retryable || attempt == 3 || ctx.Err() != nil {
			return status, err
		}
		time.Sleep(2 * time.Second)
	}
}

func (a *apiClient) send(ctx context.Context, method, path string, body []byte, out any) (int, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.base+path, reader)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if a.token != "" {
		req.Header.Set("Authorization", "Token "+a.token)
	}
	resp, err := a.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, err
	}
	if resp.StatusCode/100 != 2 {
		return resp.StatusCode, fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, bytes.TrimSpace(raw))
	}
	if out == nil {
		return resp.StatusCode, nil
	}
	return resp.StatusCode, json.Unmarshal(raw, out)
}
