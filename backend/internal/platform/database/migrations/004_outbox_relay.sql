ALTER TABLE outbox_events
    ADD COLUMN IF NOT EXISTS delivered_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS attempt INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS max_attempts INTEGER NOT NULL DEFAULT 5,
    ADD COLUMN IF NOT EXISTS next_retry_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_error TEXT;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'outbox_events_status_check'
    ) THEN
        ALTER TABLE outbox_events
            ADD CONSTRAINT outbox_events_status_check
            CHECK (status IN ('pending', 'delivered', 'failed'));
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_outbox_events_pending
    ON outbox_events (status, next_retry_at ASC, occurred_at ASC, created_at ASC)
    WHERE status IN ('pending', 'failed');
