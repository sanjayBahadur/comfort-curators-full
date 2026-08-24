-- 006_workers_user_id.sql
--
-- Links a worker to the user account they sign in as.
--
-- Nothing could resolve a session to a worker before this. There was no
-- user_id or actor_id on workers and no /me endpoint, which is why the field
-- app shows every curator the whole tenant's ticket queue (P2-04) and stamps
-- checklist completions with the literal string "curator-demo" (P2-05).
-- Neither is a frontend fix; both were waiting on this column.
--
-- This migration is also the falsification for P1-04: it ALTERs a table that
-- already exists on every deployed database. Under the old EnsureSchema
-- arrangement a change like this was silently skipped, because CREATE TABLE
-- IF NOT EXISTS does nothing to a table that is already there. If this
-- column appears on a database that already had a workers table, forward
-- migrations work.
--
-- Nullable on purpose. Existing workers have no account yet, and a worker
-- record must be able to exist before their login does — onboarding creates
-- the worker first. Code must treat a null user_id as "cannot sign in yet",
-- never as "any session matches".
--
-- users.id is uuid while workers.id is text, so the column is uuid to match
-- the referenced key.

ALTER TABLE public.workers
    ADD COLUMN IF NOT EXISTS user_id uuid;

-- One worker per user account, per tenant. A partial index so the many
-- workers with no account yet do not collide with each other on null.
CREATE UNIQUE INDEX IF NOT EXISTS workers_tenant_user_id_key
    ON public.workers (tenant_id, user_id)
    WHERE user_id IS NOT NULL;

-- The lookup the /me endpoint and every scoped field query will do.
CREATE INDEX IF NOT EXISTS workers_user_id_idx
    ON public.workers (user_id)
    WHERE user_id IS NOT NULL;

COMMENT ON COLUMN public.workers.user_id IS
    'Account this worker signs in as. Null until the worker has a login; a null must never be treated as matching any session.';
