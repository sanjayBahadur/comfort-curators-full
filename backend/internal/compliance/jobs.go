package compliance

import (
	"context"
	"encoding/json"

	"comfort-curators-backend/internal/platform/jobs"
	"comfort-curators-backend/internal/platform/logging"
)

const (
	JobTypeScanExpiry = "compliance.scan_expiry"
)

type ScanExpiryPayload struct {
	TenantID string `json:"tenant_id,omitempty"`
}

func RegisterScanExpiryJob(registry *jobs.Registry, svc *ComplianceService) {
	registry.Register(JobTypeScanExpiry, func(ctx context.Context, job *jobs.Job) (json.RawMessage, error) {
		logging.Info(ctx, "running compliance expiry scan",
			"job_id", job.ID,
			"attempt", job.Attempt,
		)

		result, err := svc.ScanExpired(ctx, "system")
		if err != nil {
			logging.Error(ctx, "compliance expiry scan failed",
				"job_id", job.ID,
				"error", err,
			)
			return nil, err
		}

		logging.Info(ctx, "compliance expiry scan complete",
			"job_id", job.ID,
			"scanned", result.Scanned,
			"expired", result.Expired,
			"holds_created", result.HoldsCreated,
			"holds_maintained", result.HoldsMaintained,
			"warnings_issued", result.WarningsIssued,
		)

		payload, _ := json.Marshal(result)
		return payload, nil
	})
}
