package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"testing"
	"time"
)

func testBaseURL() string {
	if url := os.Getenv("CC_BASE_URL"); url != "" {
		return url
	}
	port := os.Getenv("CC_HTTP_PORT")
	if port == "" {
		port = "8080"
	}
	return "http://127.0.0.1:" + port
}

func liveStackAvailable() bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(testBaseURL() + "/health/live")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func postgresAvailable() bool {
	host := os.Getenv("CC_DB_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("CC_DB_PORT")
	if port == "" {
		port = "5432"
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// TestCapacityTargetProbeRunsAgainstLiveStack runs the exact acceptance probe
// that the phase-7 gate executes. It is skipped when no live stack is present
// so the unit/build test run stays hermetic.
func TestCapacityTargetProbeRunsAgainstLiveStack(t *testing.T) {
	if !liveStackAvailable() {
		t.Skip("live API stack not reachable; skipping capacity probe integration test")
	}
	if !postgresAvailable() {
		t.Skip("PostgreSQL not reachable; skipping capacity probe integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	if err := probeCCREL001CapacityTarget(ctx, testBaseURL()); err != nil {
		t.Fatalf("capacity probe failed against live stack: %v", err)
	}
}
