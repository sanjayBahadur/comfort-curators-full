package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	if len(os.Args) != 3 || os.Args[1] != "--phase" {
		fmt.Fprintf(os.Stderr, "usage: tests/acceptance/run --phase <1-7>\n")
		os.Exit(64)
	}

	var phase int
	switch os.Args[2] {
	case "1":
		phase = 1
	case "2":
		phase = 2
	case "3":
		phase = 3
	case "4":
		phase = 4
	case "5":
		phase = 5
	case "6":
		phase = 6
	case "7":
		phase = 7
	default:
		fmt.Fprintf(os.Stderr, "usage: tests/acceptance/run --phase <1-7>\n")
		os.Exit(64)
	}

	root, err := findRepoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	oracle, err := loadOracle(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading oracle: %v\n", err)
		os.Exit(1)
	}

	oracleHash, err := computeOracleHash(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error computing oracle hash: %v\n", err)
		os.Exit(1)
	}

	probes := registeredProbes()
	if len(probes) != 55 {
		fmt.Fprintf(os.Stderr, "error: registered %d probes, expected 55\n", len(probes))
		os.Exit(1)
	}

	probeNames := make(map[string]int)
	for name := range probes {
		probeNames[name]++
	}

	for _, name := range oracle.AllNames() {
		count, ok := probeNames[name]
		if !ok {
			fmt.Fprintf(os.Stderr, "error: oracle behavior %s has no registered probe\n", name)
			os.Exit(1)
		}
		if count > 1 {
			fmt.Fprintf(os.Stderr, "error: oracle behavior %s is registered %d times (expected exactly one)\n", name, count)
			os.Exit(1)
		}
	}

	for name, count := range probeNames {
		if _, ok := oracle.Behaviors[name]; !ok {
			fmt.Fprintf(os.Stderr, "error: probe %s is registered but not in oracle\n", name)
			os.Exit(1)
		}
		_ = count
	}

	baseURL := os.Getenv("CC_BASE_URL")
	if baseURL == "" {
		port := os.Getenv("CC_HTTP_PORT")
		if port == "" {
			port = "8080"
		}
		baseURL = fmt.Sprintf("http://127.0.0.1:%s", port)
	}

	verificationCID, err := captureLiveVerification(baseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: runner verification failed: %v\n", err)
		os.Exit(1)
	}

	names := oracle.BehaviorsForPhase(phase)
	if len(names) == 0 {
		fmt.Fprintf(os.Stderr, "error: no behaviors selected for phase %d\n", phase)
		os.Exit(1)
	}

	results := make([]ProbeResult, 0, len(names))
	start := time.Now()

	for _, name := range names {
		probeFn := probes[name]
		group := phaseGroup[name]
		if group == "" {
			group = "unknown"
		}

		probeStart := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		err := probeFn(ctx, baseURL)
		cancel()
		elapsed := time.Since(probeStart)

		var result ProbeResult
		result.Name = name
		result.Group = group
		result.Duration = elapsed

		if err != nil {
			result.Status = "fail"
			result.Error = err.Error()
			result.Output = err.Error()
			fmt.Fprintf(os.Stderr, "FAIL: %s (%s)\n", name, err)
		} else {
			result.Status = "pass"
			result.Output = "ok"
			fmt.Fprintf(os.Stderr, "PASS: %s\n", name)
		}
		results = append(results, result)
	}

	totalElapsed := time.Since(start)

	integrityProps := []JUnitProp{
		{Name: "oracle-sha256", Value: oracleHash},
		{Name: "verification-correlation-id", Value: verificationCID},
		{Name: "probe-count", Value: fmt.Sprintf("%d", len(names))},
		{Name: "started-at", Value: start.UTC().Format(time.RFC3339)},
		{Name: "total-elapsed-ms", Value: fmt.Sprintf("%d", totalElapsed.Milliseconds())},
	}

	suites := generateJUnit(phase, results, start, integrityProps)
	if err := writeJUnit(suites); err != nil {
		fmt.Fprintf(os.Stderr, "error writing JUnit: %v\n", err)
		os.Exit(1)
	}

	failures := 0
	for _, r := range results {
		if r.Status == "fail" || r.Status == "error" {
			failures++
		}
	}
	if failures > 0 {
		fmt.Fprintf(os.Stderr, "\n%d acceptance probe(s) failed\n", failures)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "\nall %d acceptance probes passed for phase %d\n", len(results), phase)
}

func findRepoRoot() (string, error) {
	dir, err := filepath.Abs(".")
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("cannot find repository root (no go.mod found)")
		}
		dir = parent
	}
}

func computeOracleHash(root string) (string, error) {
	p := filepath.Join(root, "contracts", "acceptance", "oracle.yaml")
	data, err := os.ReadFile(p)
	if err != nil {
		return "", fmt.Errorf("read oracle: %w", err)
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), nil
}

func captureLiveVerification(baseURL string) (string, error) {
	url := strings.TrimRight(baseURL, "/") + "/health/live"
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("cannot reach API at %s: %w", baseURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned %d from %s", resp.StatusCode, url)
	}

	var liveResp struct {
		Status string `json:"status"`
		Time   string `json:"time"`
	}
	if err := json.Unmarshal(body, &liveResp); err != nil {
		return "", fmt.Errorf("malformed live response: %w", err)
	}
	if liveResp.Status != "ok" {
		return "", fmt.Errorf("API not live: status=%s", liveResp.Status)
	}

	cid := resp.Header.Get("X-Correlation-ID")
	if cid == "" {
		return "missing", nil
	}
	return cid, nil
}
