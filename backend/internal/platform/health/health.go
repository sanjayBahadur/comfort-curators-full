package health

import (
	"encoding/json"
	"net/http"
	"time"
)

type Status string

const (
	StatusOK       Status = "ok"
	StatusDegraded Status = "degraded"
)

type HealthResponse struct {
	Status Status            `json:"status"`
	Time   string            `json:"time"`
	Checks map[string]string `json:"checks,omitempty"`
}

type ErrorResponse struct {
	RequestID string         `json:"request_id"`
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details,omitempty"`
}

type Checker interface {
	Name() string
	Check() error
}

type CheckerFn func() error

type namedCheck struct {
	name  string
	check CheckerFn
}

func (c namedCheck) Name() string { return c.name }

func (c namedCheck) Check() error { return c.check() }

func NamedChecker(name string, fn CheckerFn) Checker {
	return namedCheck{name: name, check: fn}
}

type Handler struct {
	checks []Checker
}

func NewHandler(checks ...Checker) *Handler {
	return &Handler{checks: checks}
}

func (h *Handler) Liveness() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, HealthResponse{
			Status: StatusOK,
			Time:   time.Now().UTC().Format(time.RFC3339),
		})
	}
}

func (h *Handler) Readiness() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := HealthResponse{
			Status: StatusOK,
			Time:   time.Now().UTC().Format(time.RFC3339),
			Checks: make(map[string]string),
		}

		for _, c := range h.checks {
			if err := c.Check(); err != nil {
				resp.Checks[c.Name()] = err.Error()
				resp.Status = StatusDegraded
			} else {
				resp.Checks[c.Name()] = "ok"
			}
		}

		statusCode := http.StatusOK
		if resp.Status == StatusDegraded {
			statusCode = http.StatusServiceUnavailable
		}

		if resp.Status == StatusOK && len(resp.Checks) == 0 {
			resp.Checks = nil
		}

		writeJSON(w, statusCode, resp)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
