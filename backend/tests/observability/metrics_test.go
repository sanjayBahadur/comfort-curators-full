package observability_test

import (
	"testing"
	"time"

	"comfort-curators-backend/internal/observability"
)

// TestMetricsCoverBusinessCategories verifies every category the parent
// requires is recorded and readable through the snapshot.
func TestMetricsCoverBusinessCategories(t *testing.T) {
	m := observability.NewMetrics()

	m.APICall("POST", "/v1/properties", 201)
	m.APILatency("POST", "/v1/properties", 25*time.Millisecond)
	m.DBQuery("insert", "ok")
	m.OutboxAppended("property.activated")
	m.OutboxDispatched("property.activated")
	m.JobClaimed("compliance-scan")
	m.JobCompleted("compliance-scan")
	m.JobDead("feed-poll")
	m.CalendarFeedSync("feed-1")
	m.NotificationSent("email")
	m.FileUploaded("evidence")
	m.AICall("jarvis")
	m.AITokens("jarvis", 120)
	m.AuthAllowed("property.read")
	m.AuthDenied("property.mutate")

	if got := m.CounterValue("api.requests", "method", "POST", "route", "/v1/properties", "status", "201"); got != 1 {
		t.Errorf("api.requests counter: got %d want 1", got)
	}
	if got := m.CounterValue("outbox.appended", "event", "property.activated"); got != 1 {
		t.Errorf("outbox.appended counter: got %d want 1", got)
	}
	if got := m.CounterValue("job.completed", "job_type", "compliance-scan"); got != 1 {
		t.Errorf("job.completed counter: got %d want 1", got)
	}
	if got := m.CounterValue("calendar.feed_syncs", "feed", "feed-1"); got != 1 {
		t.Errorf("calendar.feed_syncs counter: got %d want 1", got)
	}
	if got := m.CounterValue("notification.sent", "channel", "email"); got != 1 {
		t.Errorf("notification.sent counter: got %d want 1", got)
	}
	if got := m.CounterValue("file.uploaded", "kind", "evidence"); got != 1 {
		t.Errorf("file.uploaded counter: got %d want 1", got)
	}
	if got := m.CounterValue("ai.tokens", "kind", "jarvis"); got != 120 {
		t.Errorf("ai.tokens counter: got %d want 120", got)
	}
	if got := m.CounterValue("auth.denied", "action", "property.mutate"); got != 1 {
		t.Errorf("auth.denied counter: got %d want 1", got)
	}

	snap := m.Snapshot()
	byName := map[string]int{}
	for _, metric := range snap {
		byName[metric.Name]++
	}
	for _, name := range []string{
		"api.requests", "api.latency", "database.queries",
		"outbox.appended", "outbox.dispatched", "job.claimed", "job.completed", "job.dead",
		"calendar.feed_syncs", "notification.sent", "file.uploaded", "ai.calls", "ai.tokens",
		"auth.allowed", "auth.denied",
	} {
		if byName[name] == 0 {
			t.Errorf("snapshot is missing metric series %q", name)
		}
	}
}

// TestMetricsSnapshotContainsHistogramSumAndCount verifies histogram series
// report both the sample count and the summed value.
func TestMetricsSnapshotContainsHistogramSumAndCount(t *testing.T) {
	m := observability.NewMetrics()
	m.APILatency("GET", "/v1/properties", 10*time.Millisecond)
	m.APILatency("GET", "/v1/properties", 30*time.Millisecond)

	var found *observability.Metric
	for _, metric := range m.Snapshot() {
		if metric.Name == "api.latency" {
			metric := metric
			found = &metric
		}
	}
	if found == nil {
		t.Fatal("expected api.latency histogram in snapshot")
	}
	if found.Type != observability.TypeHistogram {
		t.Errorf("expected histogram type, got %s", found.Type)
	}
	if found.Count != 2 {
		t.Errorf("expected 2 samples, got %d", found.Count)
	}
	if found.Sum != 40 {
		t.Errorf("expected sum 40ms, got %v", found.Sum)
	}
}

// TestMetricsResetClearsAllSeries verifies reset returns the registry to empty.
func TestMetricsResetClearsAllSeries(t *testing.T) {
	m := observability.NewMetrics()
	m.APICall("GET", "/health/live", 200)
	m.Reset()
	if len(m.Snapshot()) != 0 {
		t.Errorf("expected empty snapshot after reset, got %d series", len(m.Snapshot()))
	}
}
