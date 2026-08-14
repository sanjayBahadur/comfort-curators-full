package observability

import (
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// MetricType distinguishes counters from histograms in a snapshot.
type MetricType string

const (
	TypeCounter   MetricType = "counter"
	TypeHistogram MetricType = "histogram"
)

// Metric is one snapshot row of the process metrics registry.
type Metric struct {
	Name   string            `json:"name"`
	Type   MetricType        `json:"type"`
	Labels map[string]string `json:"labels,omitempty"`
	Value  int64             `json:"value,omitempty"`
	Count  int64             `json:"count,omitempty"`
	Sum    float64           `json:"sum,omitempty"`
}

type histogram struct {
	count int64
	sum   float64
}

// Metrics is a dependency-free process metrics registry covering the business
// surfaces the platform must observe: API, database, outbox, job, calendar,
// notification, file, AI and authorization. It is safe for concurrent use.
type Metrics struct {
	mu         sync.RWMutex
	counters   map[string]int64
	histograms map[string]*histogram
}

// NewMetrics returns an empty metrics registry.
func NewMetrics() *Metrics {
	return &Metrics{
		counters:   map[string]int64{},
		histograms: map[string]*histogram{},
	}
}

// Inc increments a counter identified by name and label pairs.
func (m *Metrics) Inc(name string, labels ...string) {
	m.Add(name, 1, labels...)
}

// Add changes a counter by delta.
func (m *Metrics) Add(name string, delta int64, labels ...string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counters[key(name, labels)] += delta
}

// Observe records a sample (for example a duration) into a histogram.
func (m *Metrics) Observe(name string, value float64, labels ...string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := key(name, labels)
	h, ok := m.histograms[k]
	if !ok {
		h = &histogram{}
		m.histograms[k] = h
	}
	h.count++
	h.sum += value
}

// CounterValue returns the current value of a counter. It is used by tests and
// dashboards to read a specific series.
func (m *Metrics) CounterValue(name string, labels ...string) int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.counters[key(name, labels)]
}

// Snapshot returns an ordered copy of every recorded metric series.
func (m *Metrics) Snapshot() []Metric {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]Metric, 0, len(m.counters)+len(m.histograms))
	for k, v := range m.counters {
		out = append(out, Metric{
			Name:   seriesName(k),
			Type:   TypeCounter,
			Labels: seriesLabels(k),
			Value:  v,
		})
	}
	for k, h := range m.histograms {
		out = append(out, Metric{
			Name:   seriesName(k),
			Type:   TypeHistogram,
			Labels: seriesLabels(k),
			Count:  h.count,
			Sum:    h.sum,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return labelString(out[i].Labels) < labelString(out[j].Labels)
	})
	return out
}

// Reset clears every recorded series.
func (m *Metrics) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counters = map[string]int64{}
	m.histograms = map[string]*histogram{}
}

// --- business category helpers ---

// APICall counts an HTTP request handled by the API.
func (m *Metrics) APICall(method, route string, status int) {
	m.Inc("api.requests", "method", method, "route", route, "status", intLabel(status))
}

// APILatency records the handling duration of an API request.
func (m *Metrics) APILatency(method, route string, d time.Duration) {
	m.Observe("api.latency", float64(d.Milliseconds()), "method", method, "route", route)
}

// DBQuery counts a database operation by outcome.
func (m *Metrics) DBQuery(operation, result string) {
	m.Inc("database.queries", "operation", operation, "result", result)
}

// DBLatency records the duration of a database operation.
func (m *Metrics) DBLatency(operation string, d time.Duration) {
	m.Observe("database.latency", float64(d.Milliseconds()), "operation", operation)
}

// OutboxAppended counts an event appended to the outbox.
func (m *Metrics) OutboxAppended(eventName string) {
	m.Inc("outbox.appended", "event", eventName)
}

// OutboxDispatched counts an outbox event successfully dispatched.
func (m *Metrics) OutboxDispatched(eventName string) {
	m.Inc("outbox.dispatched", "event", eventName)
}

// OutboxFailed counts an outbox event that reached a visible terminal failure.
func (m *Metrics) OutboxFailed(eventName string) {
	m.Inc("outbox.failed", "event", eventName)
}

// JobClaimed counts a durable job claimed by a worker.
func (m *Metrics) JobClaimed(jobType string) {
	m.Inc("job.claimed", "job_type", jobType)
}

// JobCompleted counts a durable job completed successfully.
func (m *Metrics) JobCompleted(jobType string) {
	m.Inc("job.completed", "job_type", jobType)
}

// JobFailed counts a durable job failed (retryable).
func (m *Metrics) JobFailed(jobType string) {
	m.Inc("job.failed", "job_type", jobType)
}

// JobDead counts a durable job moved to the dead-letter queue.
func (m *Metrics) JobDead(jobType string) {
	m.Inc("job.dead", "job_type", jobType)
}

// CalendarFeedSync counts a successful calendar feed synchronization.
func (m *Metrics) CalendarFeedSync(feedID string) {
	m.Inc("calendar.feed_syncs", "feed", feedID)
}

// CalendarFeedFailure counts a failed calendar feed synchronization.
func (m *Metrics) CalendarFeedFailure(feedID string) {
	m.Inc("calendar.feed_failures", "feed", feedID)
}

// NotificationSent counts a communication delivered over a channel.
func (m *Metrics) NotificationSent(channel string) {
	m.Inc("notification.sent", "channel", channel)
}

// NotificationFailed counts a communication that failed delivery.
func (m *Metrics) NotificationFailed(channel string) {
	m.Inc("notification.failed", "channel", channel)
}

// FileUploaded counts a file object stored.
func (m *Metrics) FileUploaded(kind string) {
	m.Inc("file.uploaded", "kind", kind)
}

// FileDownloaded counts a file object served or downloaded.
func (m *Metrics) FileDownloaded(kind string) {
	m.Inc("file.downloaded", "kind", kind)
}

// FileFailed counts a file operation that failed.
func (m *Metrics) FileFailed(kind string) {
	m.Inc("file.failed", "kind", kind)
}

// AICall counts a model provider call.
func (m *Metrics) AICall(kind string) {
	m.Inc("ai.calls", "kind", kind)
}

// AIFailed counts a model provider call that failed.
func (m *Metrics) AIFailed(kind string) {
	m.Inc("ai.failures", "kind", kind)
}

// AITokens records model token usage.
func (m *Metrics) AITokens(kind string, tokens int64) {
	m.Add("ai.tokens", tokens, "kind", kind)
}

// AuthAllowed counts an authorization check that allowed an action.
func (m *Metrics) AuthAllowed(action string) {
	m.Inc("auth.allowed", "action", action)
}

// AuthDenied counts an authorization check that denied an action before
// disclosure.
func (m *Metrics) AuthDenied(action string) {
	m.Inc("auth.denied", "action", action)
}

// --- internal key encoding ---

func key(name string, labels []string) string {
	return name + "|" + strings.Join(labels, "|")
}

func seriesName(k string) string {
	if i := strings.IndexByte(k, '|'); i >= 0 {
		return k[:i]
	}
	return k
}

func seriesLabels(k string) map[string]string {
	if i := strings.IndexByte(k, '|'); i >= 0 {
		parts := strings.Split(k[i+1:], "|")
		if len(parts) >= 2 {
			out := map[string]string{}
			for j := 0; j+1 < len(parts); j += 2 {
				out[parts[j]] = parts[j+1]
			}
			return out
		}
	}
	return nil
}

func labelString(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(labels[k])
		sb.WriteString(";")
	}
	return sb.String()
}

func intLabel(status int) string {
	return strconv.Itoa(status)
}
