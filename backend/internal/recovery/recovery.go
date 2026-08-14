package recovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"comfort-curators-backend/internal/iam"
	"comfort-curators-backend/internal/platform/health"
)

type StatusReport struct {
	RPO           string   `json:"rpo"`
	RTO           string   `json:"rto"`
	LastBackupAt  string   `json:"last_backup_at,omitempty"`
	BackupEnabled bool     `json:"backup_enabled"`
	MigrationOK   bool     `json:"migration_ok"`
	OutboxSafe    bool     `json:"outbox_safe"`
	Degradation   []string `json:"degradation,omitempty"`
	Status        string   `json:"status"`
}

type DependencyStatus struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Error  string `json:"error,omitempty"`
	Target string `json:"target,omitempty"`
}

func recoveryReport(pool *pgxpool.Pool) StatusReport {
	r := StatusReport{
		RPO:           "15 minutes",
		RTO:           "4 hours",
		BackupEnabled: true,
		Status:        "ok",
	}

	if pool != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := pool.Ping(ctx); err != nil {
			r.MigrationOK = false
			r.Degradation = append(r.Degradation, "postgresql connectivity lost")
		} else {
			r.MigrationOK = checkMigrationHealth(ctx, pool)
		}
		r.OutboxSafe = checkOutboxSafety(ctx, pool)
	}

	if !minioAvailable() {
		r.Degradation = append(r.Degradation, "minio/object-storage degraded")
		r.Status = "degraded"
	}

	if !modelStubAvailable() {
		r.Degradation = append(r.Degradation, "model-stub degraded")
		r.Status = "degraded"
	}

	if !r.MigrationOK {
		r.Degradation = append(r.Degradation, "migration health check failed")
		r.Status = "degraded"
	}
	if !r.OutboxSafe {
		r.Degradation = append(r.Degradation, "outbox safety check failed")
		r.Status = "degraded"
	}

	return r
}

func checkMigrationHealth(ctx context.Context, pool *pgxpool.Pool) bool {
	if pool == nil {
		return false
	}
	var exists bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'schema_migrations')`,
	).Scan(&exists); err != nil {
		return false
	}
	if !exists {
		return false
	}

	var count int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM schema_migrations`,
	).Scan(&count); err != nil {
		return false
	}
	return count >= 1
}

func checkOutboxSafety(ctx context.Context, pool *pgxpool.Pool) bool {
	if pool == nil {
		return false
	}
	var exists bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'idempotency_records')`,
	).Scan(&exists); err != nil {
		return false
	}
	if !exists {
		return false
	}

	if err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'outbox_events')`,
	).Scan(&exists); err != nil {
		return false
	}
	if !exists {
		return false
	}

	if err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'jobs')`,
	).Scan(&exists); err != nil {
		return false
	}
	return exists
}

func minioAvailable() bool {
	host := os.Getenv("CC_S3_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("CC_S3_PORT")
	if port == "" {
		port = "9000"
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func modelStubAvailable() bool {
	host := os.Getenv("CC_MODEL_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("CC_MODEL_PORT")
	if port == "" {
		port = "8081"
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

type Handler struct {
	mu           sync.RWMutex
	pool         *pgxpool.Pool
	lastBackupAt time.Time
	backupCount  int
}

func NewHandler(pool *pgxpool.Pool) *Handler {
	return &Handler{pool: pool}
}

func (h *Handler) RecordBackup(at time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lastBackupAt = at
	h.backupCount++
}

func (h *Handler) LastBackup() time.Time {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.lastBackupAt
}

func (h *Handler) RecoveryStatus() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		report := recoveryReport(h.pool)

		h.mu.RLock()
		if !h.lastBackupAt.IsZero() {
			report.LastBackupAt = h.lastBackupAt.UTC().Format(time.RFC3339)
		}
		h.mu.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(report)
	}
}

// RegisterRoutes attaches the recovery-status route, gated to staff: it
// discloses backup timing, migration state, and outbox health, which
// RequireAuthByDefault alone does not restrict from any logged-in subject.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /health/recovery", iam.RequireRole(iam.RoleStaff)(h.RecoveryStatus()))
}

func DependencyChecker() health.Checker {
	return health.NamedChecker("dependencies", func() error {
		degraded := []string{}
		if !minioAvailable() {
			degraded = append(degraded, "minio")
		}
		if !modelStubAvailable() {
			degraded = append(degraded, "model-stub")
		}
		if len(degraded) > 0 {
			return fmt.Errorf("degraded: %s", joinStrings(degraded))
		}
		return nil
	})
}

func MinioChecker() health.Checker {
	return health.NamedChecker("minio", func() error {
		if !minioAvailable() {
			return fmt.Errorf("minio not reachable")
		}
		return nil
	})
}

func ModelChecker() health.Checker {
	return health.NamedChecker("model", func() error {
		if !modelStubAvailable() {
			return fmt.Errorf("model not reachable")
		}
		return nil
	})
}

func joinStrings(ss []string) string {
	s := ""
	for i, v := range ss {
		if i > 0 {
			s += ", "
		}
		s += v
	}
	return s
}
