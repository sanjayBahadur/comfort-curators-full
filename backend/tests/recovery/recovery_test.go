package recovery_test

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func baseURL() string {
	if u := os.Getenv("CC_BASE_URL"); u != "" {
		return u
	}
	port := os.Getenv("CC_HTTP_PORT")
	if port == "" {
		port = "8080"
	}
	return "http://127.0.0.1:" + port
}

func TestRecoveryEndpointExists(t *testing.T) {
	u := strings.TrimRight(baseURL(), "/") + "/health/recovery"
	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Get(u)
	if err != nil {
		t.Skipf("API not reachable at %s: %v (skipping integration test)", baseURL(), err)
		return
	}
	defer resp.Body.Close()

	// /health/recovery discloses backup/migration/outbox internals and is
	// gated to staff (internal/recovery: iam.RequireRole(RoleStaff)). An
	// unauthenticated request must therefore be refused. The recovery report
	// contents are verified by the staff-authenticated acceptance probe and
	// by the package's own unit tests.
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 from /health/recovery without staff auth, got %d", resp.StatusCode)
	}
}

func TestHealthReadyReportsDependencies(t *testing.T) {
	u := strings.TrimRight(baseURL(), "/") + "/health/ready"
	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Get(u)
	if err != nil {
		t.Skipf("API not reachable at %s: %v (skipping integration test)", baseURL(), err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 200 or 503 from /health/ready, got %d", resp.StatusCode)
	}

	var ready struct {
		Status string            `json:"status"`
		Checks map[string]string `json:"checks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ready); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if ready.Status != "ok" && ready.Status != "degraded" {
		t.Errorf("expected status ok or degraded, got %q", ready.Status)
	}

	if ready.Checks == nil || len(ready.Checks) == 0 {
		t.Error("expected checks to be populated in readiness response")
	}
}

func TestHealthLiveIsOK(t *testing.T) {
	u := strings.TrimRight(baseURL(), "/") + "/health/live"
	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Get(u)
	if err != nil {
		t.Skipf("API not reachable at %s: %v (skipping integration test)", baseURL(), err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 from /health/live, got %d", resp.StatusCode)
	}

	var live struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&live); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if live.Status != "ok" {
		t.Errorf("expected status ok, got %q", live.Status)
	}
}
