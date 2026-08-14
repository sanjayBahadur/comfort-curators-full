package reservations

import (
	"context"
	"encoding/json"
	"time"

	"comfort-curators-backend/internal/platform/jobs"
	"comfort-curators-backend/internal/platform/logging"
)

const (
	JobTypePollFeed       = "calendar.poll_feed"
	JobTypeScanStaleFeeds = "calendar.scan_stale_feeds"
)

type PollFeedPayload struct {
	FeedID string `json:"feed_id"`
}

func RegisterPollFeedJob(registry *jobs.Registry, svc *CalendarService) {
	registry.Register(JobTypePollFeed, func(ctx context.Context, job *jobs.Job) (json.RawMessage, error) {
		var payload PollFeedPayload
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			logging.Error(ctx, "invalid poll feed payload",
				"job_id", job.ID,
				"error", err,
			)
			return nil, err
		}

		logging.Info(ctx, "polling calendar feed",
			"job_id", job.ID,
			"feed_id", payload.FeedID,
			"attempt", job.Attempt,
		)

		result, err := svc.PollFeed(ctx, payload.FeedID)
		if err != nil {
			logging.Error(ctx, "calendar feed poll failed",
				"job_id", job.ID,
				"feed_id", payload.FeedID,
				"error", err,
			)
			return nil, err
		}

		logging.Info(ctx, "calendar feed poll complete",
			"job_id", job.ID,
			"feed_id", payload.FeedID,
			"created", result.EventsCreated,
			"updated", result.EventsUpdated,
			"cancelled", result.EventsCancelled,
			"exceptions", result.ExceptionsCreated,
			"unchanged", result.Unchanged,
		)

		payloadBytes, _ := json.Marshal(result)
		return payloadBytes, nil
	})
}

func RegisterScanStaleFeedsJob(registry *jobs.Registry, svc *CalendarService) {
	registry.Register(JobTypeScanStaleFeeds, func(ctx context.Context, job *jobs.Job) (json.RawMessage, error) {
		logging.Info(ctx, "scanning stale calendar feeds",
			"job_id", job.ID,
			"attempt", job.Attempt,
		)

		result, err := svc.ScanStaleFeeds(ctx, time.Now().UTC())
		if err != nil {
			logging.Error(ctx, "stale feed scan failed",
				"job_id", job.ID,
				"error", err,
			)
			return nil, err
		}

		logging.Info(ctx, "stale feed scan complete",
			"job_id", job.ID,
			"stale_feeds", result.StaleFeeds,
		)

		payloadBytes, _ := json.Marshal(result)
		return payloadBytes, nil
	})
}
