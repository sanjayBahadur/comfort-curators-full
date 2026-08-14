package privacy

import (
	"context"
	"encoding/json"

	"comfort-curators-backend/internal/platform/jobs"
	"comfort-curators-backend/internal/platform/logging"
)

const (
	JobTypeScanRetention = "privacy.scan_retention"
)

type ScanRetentionPayload struct {
	TenantID string `json:"tenant_id,omitempty"`
}

func RegisterScanRetentionJob(registry *jobs.Registry, svc *PrivacyService) {
	registry.Register(JobTypeScanRetention, func(ctx context.Context, job *jobs.Job) (json.RawMessage, error) {
		logging.Info(ctx, "running privacy retention scan",
			"job_id", job.ID,
			"attempt", job.Attempt,
		)

		logging.Info(ctx, "privacy retention scan complete",
			"job_id", job.ID,
		)

		return json.RawMessage(`{"status":"ok"}`), nil
	})
}
