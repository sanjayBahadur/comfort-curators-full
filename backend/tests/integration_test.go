package tests

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"comfort-curators-backend/internal/platform/app"
	"comfort-curators-backend/internal/platform/health"
)

func TestIntegrationWiredHealthEndpoints(t *testing.T) {
	t.Setenv("CC_DB_USER", "testuser")
	t.Setenv("CC_DB_PASS", "testpass")
	t.Setenv("CC_DB_NAME", "testdb")
	t.Setenv("CC_SKIP_DB", "true")
	t.Setenv("CC_HTTP_PORT", "18080")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- app.RunAPI(ctx)
	}()

	baseURL := "http://127.0.0.1:18080"
	if err := waitForServer(baseURL+"/health/live", 5*time.Second); err != nil {
		t.Fatalf("server did not become ready: %v", err)
	}

	resp, err := http.Get(baseURL + "/health/live")
	if err != nil {
		t.Fatalf("liveness request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("liveness: expected 200, got %d", resp.StatusCode)
	}

	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("liveness: expected application/json Content-Type, got %s", ct)
	}

	body, _ := io.ReadAll(resp.Body)
	var livenessResp health.HealthResponse
	if err := json.Unmarshal(body, &livenessResp); err != nil {
		t.Fatalf("liveness: decode error: %v (body: %s)", err, body)
	}
	if livenessResp.Status != health.StatusOK {
		t.Errorf("liveness: expected status ok, got %s", livenessResp.Status)
	}
	if livenessResp.Time == "" {
		t.Error("liveness: time field must not be empty")
	}

	resp2, err := http.Get(baseURL + "/health/ready")
	if err != nil {
		t.Fatalf("readiness request: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("readiness: expected 200, got %d", resp2.StatusCode)
	}

	body2, _ := io.ReadAll(resp2.Body)
	var readinessResp health.HealthResponse
	if err := json.Unmarshal(body2, &readinessResp); err != nil {
		t.Fatalf("readiness: decode error: %v (body: %s)", err, body2)
	}
	if readinessResp.Status != health.StatusOK {
		t.Errorf("readiness: expected status ok, got %s", readinessResp.Status)
	}
	if readinessResp.Time == "" {
		t.Error("readiness: time field must not be empty")
	}

	cancel()
}

func TestIntegrationCorrelationIDMiddleware(t *testing.T) {
	t.Setenv("CC_DB_USER", "testuser")
	t.Setenv("CC_DB_PASS", "testpass")
	t.Setenv("CC_DB_NAME", "testdb")
	t.Setenv("CC_SKIP_DB", "true")
	t.Setenv("CC_HTTP_PORT", "18081")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		app.RunAPI(ctx)
	}()

	baseURL := "http://127.0.0.1:18081"
	if err := waitForServer(baseURL+"/health/live", 5*time.Second); err != nil {
		t.Fatalf("server did not become ready: %v", err)
	}

	resp, err := http.Get(baseURL + "/health/live")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	cid := resp.Header.Get("X-Correlation-ID")
	if cid == "" {
		t.Error("X-Correlation-ID header must be present in response")
	}
	rid := resp.Header.Get("X-Request-ID")
	if rid == "" {
		t.Error("X-Request-ID header must be present in response")
	}
	if cid != rid {
		t.Error("X-Correlation-ID and X-Request-ID must be equal")
	}

	cancel()
}

func TestIntegrationConfigFailureFailsFast(t *testing.T) {
	t.Setenv("CC_DB_USER", "")
	t.Setenv("CC_DB_PASS", "")
	t.Setenv("CC_DB_NAME", "")

	ctx := context.Background()
	err := app.RunAPI(ctx)
	if err == nil {
		t.Fatal("expected configuration error due to missing required env vars")
	}
	if !strings.Contains(err.Error(), "configuration") {
		t.Errorf("error should mention configuration, got: %v", err)
	}
}

func waitForServer(url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	cli := &http.Client{Timeout: 500 * time.Millisecond}
	for time.Now().Before(deadline) {
		resp, err := cli.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return context.DeadlineExceeded
}
