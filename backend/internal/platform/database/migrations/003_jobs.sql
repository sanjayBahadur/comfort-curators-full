CREATE TABLE IF NOT EXISTS jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'claimed', 'running', 'completed', 'failed', 'dead')),
    payload JSONB NOT NULL,
    idempotency_key TEXT,
    attempt INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 5,
    lease_owner TEXT,
    lease_expires_at TIMESTAMPTZ,
    last_heartbeat_at TIMESTAMPTZ,
    next_retry_at TIMESTAMPTZ,
    error_message TEXT,
    result JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_jobs_status_claim
    ON jobs (status, next_retry_at ASC, created_at ASC)
    WHERE status IN ('pending', 'failed');

CREATE INDEX IF NOT EXISTS idx_jobs_lease_expires
    ON jobs (lease_expires_at)
    WHERE status IN ('claimed', 'running') AND lease_expires_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_jobs_type_status
    ON jobs (job_type, status);

CREATE UNIQUE INDEX IF NOT EXISTS idx_jobs_idempotency_key
    ON jobs (idempotency_key)
    WHERE idempotency_key IS NOT NULL;
