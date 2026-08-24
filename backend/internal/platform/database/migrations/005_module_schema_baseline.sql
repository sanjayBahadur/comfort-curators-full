-- 005_module_schema_baseline.sql
--
-- baseline-if-exists: properties
--
-- The 143 tables below were previously created at every application start by
-- 24 separate EnsureSchema functions using CREATE TABLE IF NOT EXISTS. That
-- made the schema unchangeable: altering an existing table was silently
-- skipped, so the only way to add a column was to destroy the volume.
--
-- This file is that schema, captured as a migration. It was not written by
-- hand. It is pg_dump of a database provisioned by the real startup path
-- (see TestGenerateSchemaBaseline in internal/platform/app), verified to be
-- structurally byte-identical to the running application's database.
--
-- idempotency_records, outbox_events and jobs are excluded: migrations 002
-- and 003 own them and run first.
--
-- The "baseline-if-exists" marker above tells the runner that a database
-- which already has a "properties" table is already at this version, so it
-- records this migration as applied instead of executing it. Fresh databases
-- execute it normally. From here on, schema changes are ordinary forward
-- migrations with ALTER TABLE.

--
-- PostgreSQL database dump
--

-- Dumped from database version 16.14
-- Dumped by pg_dump version 16.14

--
-- Name: audit_no_update_delete(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.audit_no_update_delete() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
		BEGIN
			RAISE EXCEPTION 'audit_events are immutable: UPDATE and DELETE are not allowed';
		END;
		$$;

--
-- Name: contracts_accepted_version_immutable(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.contracts_accepted_version_immutable() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
		BEGIN
			IF EXISTS (
				SELECT 1 FROM service_contracts
				WHERE id = NEW.agreement_id AND status = 'accepted'
			) THEN
				RAISE EXCEPTION 'accepted service agreement is immutable';
			END IF;
			RETURN NEW;
		END;
		$$;

--
-- Name: contracts_version_immutable(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.contracts_version_immutable() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
		BEGIN
			RAISE EXCEPTION 'service agreement versions are immutable';
		END;
		$$;

--
-- Name: inventory_movements_immutable(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.inventory_movements_immutable() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
		BEGIN
			RAISE EXCEPTION 'inventory_movements are immutable: UPDATE and DELETE are not allowed';
		END;
		$$;

--
-- Name: onboarding_inspection_immutable(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.onboarding_inspection_immutable() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
		BEGIN
			RAISE EXCEPTION 'onboarding inspection evidence is immutable';
		END;
		$$;

SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: access_custody_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.access_custody_events (
    id text NOT NULL,
    tenant_id text NOT NULL,
    property_id text NOT NULL,
    grant_id text,
    secret_id text,
    event_type text NOT NULL,
    actor_id text NOT NULL,
    grantee_id text,
    reason text DEFAULT ''::text NOT NULL,
    metadata text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: access_disclosures; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.access_disclosures (
    id text NOT NULL,
    grant_id text NOT NULL,
    tenant_id text NOT NULL,
    property_id text NOT NULL,
    secret_id text NOT NULL,
    requestor_id text NOT NULL,
    result text NOT NULL,
    denial_reason text DEFAULT ''::text NOT NULL,
    disclosed_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: access_grants; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.access_grants (
    id text NOT NULL,
    tenant_id text NOT NULL,
    property_id text NOT NULL,
    secret_id text NOT NULL,
    grantee_id text NOT NULL,
    granter_id text NOT NULL,
    window_start timestamp with time zone NOT NULL,
    window_end timestamp with time zone NOT NULL,
    reason text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    acknowledged_at timestamp with time zone,
    returned_at timestamp with time zone,
    revoked_at timestamp with time zone,
    revoked_by text DEFAULT ''::text NOT NULL,
    revoke_reason text DEFAULT ''::text NOT NULL,
    is_emergency boolean DEFAULT false NOT NULL,
    emergency_reason text DEFAULT ''::text NOT NULL,
    version integer DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: access_holds; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.access_holds (
    id text NOT NULL,
    tenant_id text NOT NULL,
    property_id text NOT NULL,
    reason text DEFAULT ''::text NOT NULL,
    placed_by text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    released_at timestamp with time zone,
    released_by text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: accounting_exports; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.accounting_exports (
    id text NOT NULL,
    tenant_id text NOT NULL,
    period_start timestamp with time zone,
    period_end timestamp with time zone,
    format text DEFAULT 'journal_csv'::text NOT NULL,
    status text DEFAULT 'requested'::text NOT NULL,
    requested_by text DEFAULT ''::text NOT NULL,
    result_ref text DEFAULT ''::text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: adverse_action_reviews; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.adverse_action_reviews (
    id text NOT NULL,
    tenant_id text NOT NULL,
    worker_id text NOT NULL,
    action text NOT NULL,
    evidence_refs jsonb DEFAULT '[]'::jsonb,
    reviewer_id text NOT NULL,
    reason text DEFAULT ''::text NOT NULL,
    worker_version integer DEFAULT 1 NOT NULL,
    decided_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: agent_run_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.agent_run_events (
    event_id uuid DEFAULT gen_random_uuid() NOT NULL,
    run_id uuid NOT NULL,
    event_name text NOT NULL,
    event_data jsonb,
    occurred_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: agent_runs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.agent_runs (
    run_id uuid DEFAULT gen_random_uuid() NOT NULL,
    run_kind text NOT NULL,
    tenant_id text NOT NULL,
    property_id text NOT NULL,
    actor_id text NOT NULL,
    trigger_type text NOT NULL,
    trigger_id text NOT NULL,
    correlation_id text NOT NULL,
    idempotency_key text,
    state text DEFAULT 'queued'::text NOT NULL,
    state_version integer DEFAULT 1 NOT NULL,
    attempt integer DEFAULT 0 NOT NULL,
    max_attempts integer DEFAULT 3 NOT NULL,
    lease_owner text,
    lease_expires_at timestamp with time zone,
    heartbeat_at timestamp with time zone,
    provider text DEFAULT ''::text NOT NULL,
    model text DEFAULT ''::text NOT NULL,
    prompt_template_version text DEFAULT ''::text NOT NULL,
    input_schema_version text DEFAULT ''::text NOT NULL,
    output_schema_version text DEFAULT ''::text NOT NULL,
    input_data jsonb,
    output_data jsonb,
    error_message text,
    usage_minor_units bigint DEFAULT 0 NOT NULL,
    usage_currency text DEFAULT 'USD'::text NOT NULL,
    usage_input_tokens bigint DEFAULT 0 NOT NULL,
    usage_output_tokens bigint DEFAULT 0 NOT NULL,
    usage_total_tokens bigint DEFAULT 0 NOT NULL,
    usage_known boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    messages_json jsonb,
    streaming_text text DEFAULT ''::text NOT NULL,
    CONSTRAINT agent_runs_state_check CHECK ((state = ANY (ARRAY['queued'::text, 'leased'::text, 'running'::text, 'waiting_for_tool'::text, 'waiting_for_approval'::text, 'retryable'::text, 'unknown'::text, 'completed'::text, 'failed'::text, 'cancelled'::text])))
);

--
-- Name: ai_tool_calls; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ai_tool_calls (
    call_id text NOT NULL,
    run_id text NOT NULL,
    tool_name text NOT NULL,
    tool_version text DEFAULT 'v1'::text NOT NULL,
    tool_kind text NOT NULL,
    state text DEFAULT 'proposed'::text NOT NULL,
    input_data jsonb,
    output_data jsonb,
    idempotency_key text DEFAULT ''::text NOT NULL,
    tenant_id text NOT NULL,
    property_id text NOT NULL,
    actor_id text NOT NULL,
    input_class text DEFAULT ''::text NOT NULL,
    output_class text DEFAULT ''::text NOT NULL,
    policy_result text DEFAULT ''::text NOT NULL,
    error_message text,
    attempt integer DEFAULT 0 NOT NULL,
    max_attempts integer DEFAULT 3 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ai_tool_calls_state_check CHECK ((state = ANY (ARRAY['proposed'::text, 'policy_checking'::text, 'approval_required'::text, 'executing'::text, 'succeeded'::text, 'denied'::text, 'retryable'::text, 'failed'::text, 'cancelled'::text])))
);

--
-- Name: approval_requests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.approval_requests (
    request_id text NOT NULL,
    run_id text NOT NULL,
    decision_id text,
    tool_name text NOT NULL,
    tool_version text DEFAULT 'v1'::text NOT NULL,
    approval_kind text DEFAULT ''::text NOT NULL,
    requester_id text NOT NULL,
    requester_roles jsonb,
    tenant_id text NOT NULL,
    property_id text NOT NULL,
    state text DEFAULT 'pending'::text NOT NULL,
    proposed_data jsonb,
    actor_id text,
    actor_role text,
    evidence text,
    reason text,
    policy_version text DEFAULT ''::text NOT NULL,
    requested_at timestamp with time zone DEFAULT now() NOT NULL,
    decided_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT approval_requests_state_check CHECK ((state = ANY (ARRAY['pending'::text, 'approved'::text, 'rejected'::text, 'expired'::text, 'cancelled'::text])))
);

--
-- Name: audit_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.audit_events (
    id text NOT NULL,
    event_type text NOT NULL,
    tenant_id text,
    actor_id text NOT NULL,
    action text NOT NULL,
    resource_type text DEFAULT ''::text NOT NULL,
    resource_id text DEFAULT ''::text NOT NULL,
    previous_state jsonb,
    new_state jsonb,
    metadata jsonb,
    correlation_id text,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: authentication_methods; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.authentication_methods (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    method text DEFAULT 'otp'::text NOT NULL,
    secret_hash text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    consumed boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: availability_windows; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.availability_windows (
    id text NOT NULL,
    tenant_id text NOT NULL,
    worker_id text NOT NULL,
    day_of_week integer NOT NULL,
    start_minute integer NOT NULL,
    end_minute integer NOT NULL,
    effective_at timestamp with time zone DEFAULT now() NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: bank_verifications; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.bank_verifications (
    id text NOT NULL,
    tenant_id text NOT NULL,
    request_id text DEFAULT ''::text NOT NULL,
    verification_token text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    verified_by text DEFAULT ''::text NOT NULL,
    verified_at timestamp with time zone,
    expires_at timestamp with time zone DEFAULT now() NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: calendar_exceptions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.calendar_exceptions (
    id text NOT NULL,
    tenant_id text NOT NULL,
    property_id text NOT NULL,
    feed_id text,
    kind text NOT NULL,
    severity text NOT NULL,
    status text DEFAULT 'open'::text NOT NULL,
    message text NOT NULL,
    dedupe_key text DEFAULT ''::text NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    resolved_at timestamp with time zone
);

--
-- Name: calendar_feeds; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.calendar_feeds (
    id text NOT NULL,
    tenant_id text NOT NULL,
    property_id text NOT NULL,
    source text NOT NULL,
    url text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    property_timezone text DEFAULT 'Asia/Kolkata'::text NOT NULL,
    stale_after_minutes integer DEFAULT 1440 NOT NULL,
    minimum_turnaround_minutes integer DEFAULT 180 NOT NULL,
    last_polled_at timestamp with time zone,
    last_success_at timestamp with time zone,
    last_content_hash text,
    last_error text,
    version integer DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: catalog_claim_evidence; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.catalog_claim_evidence (
    id text NOT NULL,
    tenant_id text NOT NULL,
    catalog_item_id text NOT NULL,
    claim_type text NOT NULL,
    claim_statement text DEFAULT ''::text NOT NULL,
    evidence_ref text NOT NULL,
    evidence_retained_at timestamp with time zone DEFAULT now() NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: catalog_items; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.catalog_items (
    id text NOT NULL,
    tenant_id text NOT NULL,
    sku text NOT NULL,
    name text NOT NULL,
    category text NOT NULL,
    brand text DEFAULT ''::text NOT NULL,
    pack_size text DEFAULT ''::text NOT NULL,
    unit_cost_minor_units bigint NOT NULL,
    unit_cost_currency text NOT NULL,
    owner_price_minor_units bigint NOT NULL,
    owner_price_currency text NOT NULL,
    tax_class text DEFAULT ''::text NOT NULL,
    supplier text DEFAULT ''::text NOT NULL,
    country_of_origin text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    shelf_life_rule text DEFAULT ''::text NOT NULL,
    substitution_group text DEFAULT ''::text NOT NULL,
    operational_suitability text DEFAULT ''::text NOT NULL,
    label text NOT NULL,
    version integer DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: charges; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.charges (
    id text NOT NULL,
    tenant_id text NOT NULL,
    property_id text DEFAULT ''::text NOT NULL,
    charge_type text DEFAULT ''::text NOT NULL,
    amount_minor_units bigint DEFAULT 0 NOT NULL,
    currency text DEFAULT 'INR'::text NOT NULL,
    reason text DEFAULT ''::text NOT NULL,
    data jsonb DEFAULT '{}'::jsonb NOT NULL,
    contract_rule_id text DEFAULT ''::text NOT NULL,
    evidence_id text DEFAULT ''::text NOT NULL,
    ticket_id text DEFAULT ''::text NOT NULL,
    order_id text DEFAULT ''::text NOT NULL,
    approval_id text DEFAULT ''::text NOT NULL,
    idempotency_key text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: checklist_sync_conflicts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.checklist_sync_conflicts (
    id text NOT NULL,
    tenant_id text NOT NULL,
    ticket_id text NOT NULL,
    checklist_item_id text,
    template_item_index integer DEFAULT 0 NOT NULL,
    server_label text DEFAULT ''::text NOT NULL,
    server_status text DEFAULT ''::text NOT NULL,
    server_version integer DEFAULT 0 NOT NULL,
    client_label text DEFAULT ''::text NOT NULL,
    client_status text DEFAULT ''::text NOT NULL,
    client_version integer DEFAULT 0 NOT NULL,
    resolved boolean DEFAULT false NOT NULL,
    resolution text,
    resolved_by text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    resolved_at timestamp with time zone
);

--
-- Name: checklist_sync_records; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.checklist_sync_records (
    id text NOT NULL,
    sync_key text NOT NULL,
    tenant_id text NOT NULL,
    ticket_id text NOT NULL,
    payload_hash text NOT NULL,
    result text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: checklist_template_versions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.checklist_template_versions (
    id text NOT NULL,
    template_id text NOT NULL,
    version integer NOT NULL,
    items jsonb DEFAULT '[]'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: checklist_templates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.checklist_templates (
    id text NOT NULL,
    name text NOT NULL,
    ticket_type text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: communication_drafts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.communication_drafts (
    id text NOT NULL,
    tenant_id text NOT NULL,
    audience text NOT NULL,
    recipient_id text NOT NULL,
    source text NOT NULL,
    template_key text,
    consent_class text NOT NULL,
    channel text DEFAULT 'push'::text NOT NULL,
    severity text DEFAULT 'normal'::text NOT NULL,
    subject text DEFAULT ''::text NOT NULL,
    body text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'draft'::text NOT NULL,
    requires_review boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: communication_preferences; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.communication_preferences (
    id text NOT NULL,
    tenant_id text NOT NULL,
    recipient_id text NOT NULL,
    audience text NOT NULL,
    consent_transactional boolean DEFAULT true NOT NULL,
    consent_urgent boolean DEFAULT true NOT NULL,
    consent_marketing boolean DEFAULT false NOT NULL,
    consent_sponsored boolean DEFAULT false NOT NULL,
    channel text DEFAULT 'push'::text NOT NULL,
    severity text DEFAULT 'normal'::text NOT NULL,
    quiet_hours_start_minute integer DEFAULT 0 NOT NULL,
    quiet_hours_end_minute integer DEFAULT 0 NOT NULL,
    escalation_contacts jsonb DEFAULT '[]'::jsonb,
    version integer DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: communication_reviews; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.communication_reviews (
    id text NOT NULL,
    tenant_id text NOT NULL,
    draft_id text NOT NULL,
    reviewer_id text NOT NULL,
    decision text NOT NULL,
    reason text DEFAULT ''::text NOT NULL,
    reviewed_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: compliance_items; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.compliance_items (
    id text NOT NULL,
    property_id text NOT NULL,
    tenant_id text NOT NULL,
    kind text NOT NULL,
    severity text NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    effective_date timestamp with time zone NOT NULL,
    expiry_date timestamp with time zone NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    evidence_ids jsonb DEFAULT '[]'::jsonb NOT NULL,
    renewed_from_id text,
    hold_id text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: compliance_renewal_warnings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.compliance_renewal_warnings (
    id text NOT NULL,
    item_id text NOT NULL,
    property_id text NOT NULL,
    tenant_id text NOT NULL,
    days_before_expiry integer NOT NULL,
    issued_at timestamp with time zone NOT NULL,
    acknowledged_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: consumer_acceptances; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.consumer_acceptances (
    id text NOT NULL,
    tenant_id text NOT NULL,
    property_id text DEFAULT ''::text NOT NULL,
    disclosure_id text NOT NULL,
    resource_type text NOT NULL,
    resource_id text NOT NULL,
    accepted_by text DEFAULT ''::text NOT NULL,
    accepted_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: consumer_disclosures; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.consumer_disclosures (
    id text NOT NULL,
    tenant_id text NOT NULL,
    property_id text DEFAULT ''::text NOT NULL,
    resource_type text NOT NULL,
    resource_id text NOT NULL,
    price_minor_units bigint DEFAULT 0 NOT NULL,
    tax_minor_units bigint DEFAULT 0 NOT NULL,
    currency text NOT NULL,
    recurrence text DEFAULT 'one_time'::text NOT NULL,
    recurrence_amount_minor_units bigint,
    substitution_policy text DEFAULT ''::text NOT NULL,
    cancellation_policy text DEFAULT ''::text NOT NULL,
    refund_policy text DEFAULT ''::text NOT NULL,
    seller text DEFAULT ''::text NOT NULL,
    country_of_origin text DEFAULT ''::text NOT NULL,
    grievance_contact text DEFAULT ''::text NOT NULL,
    recurring_cost_visible boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: consumer_history_exports; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.consumer_history_exports (
    id text NOT NULL,
    tenant_id text NOT NULL,
    property_id text DEFAULT ''::text NOT NULL,
    requested_by text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'completed'::text NOT NULL,
    data jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: contract_acceptances; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.contract_acceptances (
    id text NOT NULL,
    agreement_id text NOT NULL,
    tenant_id text NOT NULL,
    version_number integer NOT NULL,
    content_hash text NOT NULL,
    accepted_by text NOT NULL,
    accepted_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: conversation_links; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.conversation_links (
    id text NOT NULL,
    tenant_id text NOT NULL,
    property_id text NOT NULL,
    audience text NOT NULL,
    recipient_id text NOT NULL,
    purpose text DEFAULT 'stay'::text NOT NULL,
    token_hash text NOT NULL,
    token_tail text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    used_at timestamp with time zone,
    revoked_at timestamp with time zone,
    status text DEFAULT 'active'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: credits; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.credits (
    id text NOT NULL,
    tenant_id text NOT NULL,
    property_id text DEFAULT ''::text NOT NULL,
    credit_type text DEFAULT ''::text NOT NULL,
    amount_minor_units bigint DEFAULT 0 NOT NULL,
    currency text DEFAULT 'INR'::text NOT NULL,
    reason text DEFAULT ''::text NOT NULL,
    original_entry_id text DEFAULT ''::text NOT NULL,
    original_entry_type text DEFAULT ''::text NOT NULL,
    data jsonb DEFAULT '{}'::jsonb NOT NULL,
    idempotency_key text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'issued'::text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: deliveries; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.deliveries (
    id text NOT NULL,
    tenant_id text NOT NULL,
    draft_id text,
    recipient_id text NOT NULL,
    audience text NOT NULL,
    consent_class text NOT NULL,
    channel text DEFAULT 'push'::text NOT NULL,
    status text DEFAULT 'queued'::text NOT NULL,
    error text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    delivered_at timestamp with time zone,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: dispatch_overrides; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.dispatch_overrides (
    id text NOT NULL,
    tenant_id text NOT NULL,
    ticket_id text NOT NULL,
    worker_id text NOT NULL,
    overridden_by text NOT NULL,
    reason text NOT NULL,
    overridden_constraint text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: document_extractions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.document_extractions (
    id text NOT NULL,
    document_version_id text NOT NULL,
    tenant_id text NOT NULL,
    field_name text NOT NULL,
    field_value text DEFAULT ''::text NOT NULL,
    field_category text DEFAULT 'general'::text NOT NULL,
    source_location text DEFAULT ''::text NOT NULL,
    confidence text DEFAULT 'high'::text NOT NULL,
    confidence_score double precision DEFAULT 0 NOT NULL,
    extracted_by text DEFAULT ''::text NOT NULL,
    extracted_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: document_reviews; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.document_reviews (
    id text NOT NULL,
    document_id text NOT NULL,
    document_version_id text NOT NULL,
    tenant_id text NOT NULL,
    reviewer_id text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    decision text DEFAULT ''::text NOT NULL,
    comments text DEFAULT ''::text NOT NULL,
    reviewed_at timestamp with time zone DEFAULT now() NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: document_versions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.document_versions (
    id text NOT NULL,
    document_id text NOT NULL,
    tenant_id text NOT NULL,
    version_number integer NOT NULL,
    content_hash text NOT NULL,
    object_key text NOT NULL,
    filename text DEFAULT ''::text NOT NULL,
    content_type text DEFAULT ''::text NOT NULL,
    size_bytes bigint DEFAULT 0 NOT NULL,
    uploaded_by text DEFAULT ''::text NOT NULL,
    metadata text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: documents; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.documents (
    id text NOT NULL,
    tenant_id text NOT NULL,
    property_id text NOT NULL,
    title text NOT NULL,
    document_type text NOT NULL,
    status text DEFAULT 'draft'::text NOT NULL,
    expires_at timestamp with time zone,
    current_version integer DEFAULT 1 NOT NULL,
    version integer DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: employment_terms; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.employment_terms (
    id text NOT NULL,
    tenant_id text NOT NULL,
    worker_id text NOT NULL,
    role text NOT NULL,
    compensation_band text DEFAULT ''::text,
    effective_date timestamp with time zone DEFAULT now() NOT NULL,
    end_date timestamp with time zone,
    agreement_ref text,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: encryption_keys; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.encryption_keys (
    id text NOT NULL,
    algorithm text DEFAULT 'aes256-gcm'::text NOT NULL,
    active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    rotated_at timestamp with time zone
);

--
-- Name: expenses; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.expenses (
    id text NOT NULL,
    tenant_id text NOT NULL,
    worker_id text NOT NULL,
    ticket_id text,
    minor_units bigint DEFAULT 0 NOT NULL,
    currency text DEFAULT 'INR'::text NOT NULL,
    category text DEFAULT ''::text NOT NULL,
    receipt_ref text,
    recorded_by text DEFAULT ''::text NOT NULL,
    recorded_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: external_calendar_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.external_calendar_events (
    id text NOT NULL,
    tenant_id text NOT NULL,
    property_id text NOT NULL,
    feed_id text NOT NULL,
    external_event_id text NOT NULL,
    source text NOT NULL,
    summary text,
    description text,
    start_at timestamp with time zone NOT NULL,
    end_at timestamp with time zone NOT NULL,
    all_day boolean DEFAULT false NOT NULL,
    timezone text,
    timezone_ambiguous boolean DEFAULT false NOT NULL,
    status text DEFAULT 'confirmed'::text NOT NULL,
    sequence integer DEFAULT 0 NOT NULL,
    raw_ical text NOT NULL,
    first_seen_at timestamp with time zone DEFAULT now() NOT NULL,
    last_seen_at timestamp with time zone DEFAULT now() NOT NULL,
    version integer DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: fee_rules; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.fee_rules (
    id text NOT NULL,
    rule_version text NOT NULL,
    currency text NOT NULL,
    service_tier text NOT NULL,
    percentage_basis_points bigint NOT NULL,
    minimum_monthly_fee_minor_units bigint DEFAULT 0 NOT NULL,
    setup_fee_minor_units bigint DEFAULT 0 NOT NULL,
    effective_from text,
    effective_to text,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: file_grants; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.file_grants (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id uuid NOT NULL,
    object_id uuid,
    grant_type text NOT NULL,
    grant_token text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    max_size_bytes bigint,
    allowed_content_types text[],
    used_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: file_objects; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.file_objects (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id uuid NOT NULL,
    object_key text NOT NULL,
    original_name text DEFAULT ''::text NOT NULL,
    content_type text DEFAULT ''::text NOT NULL,
    size_bytes bigint DEFAULT 0 NOT NULL,
    sha256_hash text DEFAULT ''::text NOT NULL,
    scan_status text DEFAULT 'unscanned'::text NOT NULL,
    retention_policy text DEFAULT 'standard'::text NOT NULL,
    retention_until timestamp with time zone,
    is_original boolean DEFAULT true NOT NULL,
    metadata jsonb,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone
);

--
-- Name: financial_approvals; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.financial_approvals (
    id text NOT NULL,
    tenant_id text NOT NULL,
    request_id text DEFAULT ''::text NOT NULL,
    approver_id text DEFAULT ''::text NOT NULL,
    decision text DEFAULT ''::text NOT NULL,
    reason text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: fleet_assets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.fleet_assets (
    id text NOT NULL,
    tenant_id text NOT NULL,
    model text NOT NULL,
    serial_number text NOT NULL,
    rated_motor_power_watts integer NOT NULL,
    maximum_design_speed_kmh integer NOT NULL,
    design_speed_evidence_ref text DEFAULT ''::text NOT NULL,
    compliance_document_ref text DEFAULT ''::text NOT NULL,
    battery_serial text DEFAULT ''::text NOT NULL,
    charger text DEFAULT ''::text NOT NULL,
    purchase_date timestamp with time zone NOT NULL,
    warranty_expires_at timestamp with time zone,
    warranty_terms text DEFAULT ''::text NOT NULL,
    assigned_custodian_id text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'available'::text NOT NULL,
    version integer DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: fleet_batteries; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.fleet_batteries (
    id text NOT NULL,
    tenant_id text NOT NULL,
    asset_id text NOT NULL,
    battery_serial text NOT NULL,
    health_status text DEFAULT 'ok'::text NOT NULL,
    cycle_count integer DEFAULT 0 NOT NULL,
    last_service_at timestamp with time zone,
    next_service_due_at timestamp with time zone,
    status text DEFAULT 'active'::text NOT NULL,
    version integer DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: fleet_custody_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.fleet_custody_events (
    id text NOT NULL,
    tenant_id text NOT NULL,
    asset_id text NOT NULL,
    event_type text NOT NULL,
    from_worker_id text DEFAULT ''::text NOT NULL,
    to_worker_id text DEFAULT ''::text NOT NULL,
    condition text DEFAULT ''::text NOT NULL,
    accessories text DEFAULT ''::text NOT NULL,
    acknowledged_by text DEFAULT ''::text NOT NULL,
    acknowledged_at timestamp with time zone,
    notes text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: fleet_incidents; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.fleet_incidents (
    id text NOT NULL,
    tenant_id text NOT NULL,
    asset_id text NOT NULL,
    kind text DEFAULT ''::text NOT NULL,
    severity text NOT NULL,
    description text NOT NULL,
    reported_by text NOT NULL,
    safety_ticket_id text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'open'::text NOT NULL,
    reviewed_by text DEFAULT ''::text NOT NULL,
    reviewed_at timestamp with time zone,
    resolution text DEFAULT ''::text NOT NULL,
    version integer DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: fleet_inspections; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.fleet_inspections (
    id text NOT NULL,
    tenant_id text NOT NULL,
    asset_id text NOT NULL,
    worker_id text NOT NULL,
    inspection_type text DEFAULT 'pre_use'::text NOT NULL,
    result text NOT NULL,
    damage_reported boolean DEFAULT false NOT NULL,
    damage_description text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: fleet_maintenance; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.fleet_maintenance (
    id text NOT NULL,
    tenant_id text NOT NULL,
    asset_id text NOT NULL,
    kind text NOT NULL,
    title text DEFAULT ''::text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    due_at timestamp with time zone,
    completed_at timestamp with time zone,
    status text DEFAULT 'open'::text NOT NULL,
    service_provider text DEFAULT ''::text NOT NULL,
    performed_by text DEFAULT ''::text NOT NULL,
    notes text DEFAULT ''::text NOT NULL,
    version integer DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: fleet_tracking_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.fleet_tracking_events (
    id text NOT NULL,
    tenant_id text NOT NULL,
    asset_id text NOT NULL,
    worker_id text NOT NULL,
    custody_event_id text DEFAULT ''::text NOT NULL,
    latitude double precision NOT NULL,
    longitude double precision NOT NULL,
    captured_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: goods_receipt_items; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.goods_receipt_items (
    id text NOT NULL,
    tenant_id text NOT NULL,
    goods_receipt_id text NOT NULL,
    purchase_order_item_id text DEFAULT ''::text NOT NULL,
    catalog_item_id text DEFAULT ''::text NOT NULL,
    quantity_ordered bigint DEFAULT 0 NOT NULL,
    quantity_received bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: goods_receipts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.goods_receipts (
    id text NOT NULL,
    tenant_id text NOT NULL,
    purchase_order_id text DEFAULT ''::text NOT NULL,
    received_by text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'draft'::text NOT NULL,
    condition text DEFAULT 'good'::text NOT NULL,
    condition_notes text DEFAULT ''::text NOT NULL,
    evidence_ref text DEFAULT ''::text NOT NULL,
    received_at timestamp with time zone DEFAULT now() NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: grievances; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.grievances (
    id text NOT NULL,
    tenant_id text NOT NULL,
    worker_id text NOT NULL,
    kind text NOT NULL,
    reason text DEFAULT ''::text NOT NULL,
    evidence_refs jsonb DEFAULT '[]'::jsonb,
    status text DEFAULT 'pending'::text NOT NULL,
    submitted_at timestamp with time zone DEFAULT now() NOT NULL,
    resolved_at timestamp with time zone
);

--
-- Name: hermes_deliveries; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.hermes_deliveries (
    delivery_id text NOT NULL,
    tenant_id text NOT NULL,
    draft_id text NOT NULL,
    audience text NOT NULL,
    recipient_id text DEFAULT ''::text NOT NULL,
    idempotency_key text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'queued'::text NOT NULL,
    error text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    delivered_at timestamp with time zone,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT hermes_deliveries_status_check CHECK ((status = ANY (ARRAY['queued'::text, 'sent'::text, 'delivered'::text, 'failed'::text])))
);

--
-- Name: hermes_drafts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.hermes_drafts (
    draft_id text NOT NULL,
    run_id text DEFAULT ''::text NOT NULL,
    tenant_id text NOT NULL,
    property_id text NOT NULL,
    actor_id text DEFAULT ''::text NOT NULL,
    audience text NOT NULL,
    purpose text NOT NULL,
    template_key text DEFAULT ''::text NOT NULL,
    language text DEFAULT 'en'::text NOT NULL,
    channel text DEFAULT 'push'::text NOT NULL,
    facts jsonb DEFAULT '[]'::jsonb NOT NULL,
    review_policy text NOT NULL,
    state text DEFAULT 'draft'::text NOT NULL,
    subject text DEFAULT ''::text NOT NULL,
    body text DEFAULT ''::text NOT NULL,
    version integer DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT hermes_drafts_audience_check CHECK ((audience = ANY (ARRAY['owner'::text, 'guest'::text]))),
    CONSTRAINT hermes_drafts_review_policy_check CHECK ((review_policy = ANY (ARRAY['approved_template'::text, 'human_review'::text]))),
    CONSTRAINT hermes_drafts_state_check CHECK ((state = ANY (ARRAY['draft'::text, 'under_review'::text, 'approved'::text, 'rejected'::text, 'delivered'::text])))
);

--
-- Name: hermes_reviews; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.hermes_reviews (
    review_id text NOT NULL,
    tenant_id text NOT NULL,
    draft_id text NOT NULL,
    reviewer_id text NOT NULL,
    decision text NOT NULL,
    reason text DEFAULT ''::text NOT NULL,
    reviewed_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT hermes_reviews_decision_check CHECK ((decision = ANY (ARRAY['approved'::text, 'rejected'::text])))
);

--
-- Name: incident_alerts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.incident_alerts (
    id text NOT NULL,
    tenant_id text NOT NULL,
    property_id text NOT NULL,
    ticket_id text NOT NULL,
    severity text DEFAULT ''::text NOT NULL,
    target text NOT NULL,
    policy text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'queued'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: inventory_count_lines; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.inventory_count_lines (
    id text NOT NULL,
    tenant_id text NOT NULL,
    count_id text NOT NULL,
    catalog_item_id text NOT NULL,
    expected_quantity bigint DEFAULT 0 NOT NULL,
    counted_quantity bigint DEFAULT 0 NOT NULL,
    variance bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: inventory_counts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.inventory_counts (
    id text NOT NULL,
    tenant_id text NOT NULL,
    location_id text NOT NULL,
    status text DEFAULT 'draft'::text NOT NULL,
    counted_by text DEFAULT ''::text NOT NULL,
    reviewed_by text DEFAULT ''::text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: inventory_movements; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.inventory_movements (
    id text NOT NULL,
    tenant_id text NOT NULL,
    location_id text NOT NULL,
    catalog_item_id text NOT NULL,
    movement_type text NOT NULL,
    quantity bigint NOT NULL,
    reference_type text DEFAULT ''::text NOT NULL,
    reference_id text DEFAULT ''::text NOT NULL,
    reason text DEFAULT ''::text NOT NULL,
    actor_id text DEFAULT ''::text NOT NULL,
    expires_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: invoice_lines; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.invoice_lines (
    id text NOT NULL,
    invoice_id text DEFAULT ''::text NOT NULL,
    tenant_id text NOT NULL,
    charge_type text DEFAULT ''::text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    amount_minor_units bigint DEFAULT 0 NOT NULL,
    currency text DEFAULT 'INR'::text NOT NULL,
    contract_rule_id text DEFAULT ''::text NOT NULL,
    ticket_id text DEFAULT ''::text NOT NULL,
    order_id text DEFAULT ''::text NOT NULL,
    adjustment_id text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: invoices; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.invoices (
    id text NOT NULL,
    tenant_id text NOT NULL,
    property_id text DEFAULT ''::text NOT NULL,
    period_start timestamp with time zone,
    period_end timestamp with time zone,
    total_minor_units bigint DEFAULT 0 NOT NULL,
    currency text DEFAULT 'INR'::text NOT NULL,
    status text DEFAULT 'draft'::text NOT NULL,
    idempotency_key text DEFAULT ''::text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: maintenance_approvals; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.maintenance_approvals (
    id text NOT NULL,
    tenant_id text NOT NULL,
    request_id text DEFAULT ''::text NOT NULL,
    estimate_id text DEFAULT ''::text NOT NULL,
    actor_id text DEFAULT ''::text NOT NULL,
    decision text DEFAULT ''::text NOT NULL,
    reason text DEFAULT ''::text NOT NULL,
    is_ai_actor boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: maintenance_estimates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.maintenance_estimates (
    id text NOT NULL,
    tenant_id text NOT NULL,
    request_id text DEFAULT ''::text NOT NULL,
    property_id text DEFAULT ''::text NOT NULL,
    prepared_by text DEFAULT ''::text NOT NULL,
    amount_minor_units bigint DEFAULT 0 NOT NULL,
    currency text DEFAULT 'INR'::text NOT NULL,
    scope text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'draft'::text NOT NULL,
    submitted_at timestamp with time zone,
    approved_by text DEFAULT ''::text NOT NULL,
    approved_at timestamp with time zone,
    rejected_by text DEFAULT ''::text NOT NULL,
    rejected_at timestamp with time zone,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: maintenance_requests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.maintenance_requests (
    id text NOT NULL,
    tenant_id text NOT NULL,
    property_id text DEFAULT ''::text NOT NULL,
    title text DEFAULT ''::text NOT NULL,
    category text DEFAULT ''::text NOT NULL,
    priority text DEFAULT 'normal'::text NOT NULL,
    risk_level text DEFAULT 'standard'::text NOT NULL,
    status text DEFAULT 'reported'::text NOT NULL,
    reported_by text DEFAULT ''::text NOT NULL,
    triaged_by text DEFAULT ''::text NOT NULL,
    triaged_at timestamp with time zone,
    estimate_id text DEFAULT ''::text NOT NULL,
    notes text DEFAULT ''::text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: maker_checker_requests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.maker_checker_requests (
    id text NOT NULL,
    tenant_id text NOT NULL,
    request_type text DEFAULT ''::text NOT NULL,
    property_id text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'draft'::text NOT NULL,
    created_by text DEFAULT ''::text NOT NULL,
    submitted_by text DEFAULT ''::text NOT NULL,
    approved_by text DEFAULT ''::text NOT NULL,
    rejected_by text DEFAULT ''::text NOT NULL,
    payload jsonb DEFAULT '{}'::jsonb NOT NULL,
    idempotency_key text DEFAULT ''::text NOT NULL,
    requires_verification boolean DEFAULT false NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: memberships; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.memberships (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id uuid NOT NULL,
    user_id uuid NOT NULL,
    role text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: message_template_versions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.message_template_versions (
    id text NOT NULL,
    tenant_id text NOT NULL,
    template_id text NOT NULL,
    version integer NOT NULL,
    language text NOT NULL,
    subject text DEFAULT ''::text NOT NULL,
    body text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: message_templates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.message_templates (
    id text NOT NULL,
    tenant_id text NOT NULL,
    template_key text NOT NULL,
    audience text NOT NULL,
    consent_class text NOT NULL,
    channel text DEFAULT 'push'::text NOT NULL,
    severity text DEFAULT 'normal'::text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: metric_observations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.metric_observations (
    id text NOT NULL,
    tenant_id text NOT NULL,
    property_id text DEFAULT ''::text NOT NULL,
    worker_id text NOT NULL,
    metric_kind text NOT NULL,
    value bigint DEFAULT 0 NOT NULL,
    unit text DEFAULT ''::text NOT NULL,
    period_start timestamp with time zone,
    period_end timestamp with time zone,
    source_ref text DEFAULT ''::text NOT NULL,
    recorded_by text DEFAULT ''::text NOT NULL,
    recorded_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: mfa_methods; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mfa_methods (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    method text DEFAULT 'totp'::text NOT NULL,
    secret text NOT NULL,
    verified boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: onboarding_cases; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.onboarding_cases (
    id text NOT NULL,
    tenant_id text NOT NULL,
    property_id text NOT NULL,
    owner_authority_id text NOT NULL,
    status text DEFAULT 'in_progress'::text NOT NULL,
    portfolio jsonb,
    goals jsonb,
    service_preferences jsonb,
    budgets jsonb,
    contacts jsonb DEFAULT '[]'::jsonb NOT NULL,
    photographs jsonb DEFAULT '[]'::jsonb NOT NULL,
    amenities jsonb DEFAULT '[]'::jsonb NOT NULL,
    safety jsonb,
    furnishing jsonb,
    remediation jsonb,
    fit_score_inputs jsonb,
    version integer DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: onboarding_evidence; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.onboarding_evidence (
    id text NOT NULL,
    case_id text NOT NULL,
    tenant_id text NOT NULL,
    kind text NOT NULL,
    content_hash text NOT NULL,
    object_ref text NOT NULL,
    captured_by text NOT NULL,
    captured_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: onboarding_inspections; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.onboarding_inspections (
    id text NOT NULL,
    case_id text NOT NULL,
    tenant_id text NOT NULL,
    property_id text NOT NULL,
    performed_at timestamp with time zone NOT NULL,
    inspected_by text NOT NULL,
    evidence_hash text NOT NULL,
    evidence_ref text NOT NULL,
    findings text NOT NULL,
    overall_status text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: operational_subledger_entries; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.operational_subledger_entries (
    id text NOT NULL,
    tenant_id text NOT NULL,
    property_id text DEFAULT ''::text NOT NULL,
    entry_type text DEFAULT ''::text NOT NULL,
    amount_minor_units bigint DEFAULT 0 NOT NULL,
    currency text DEFAULT 'INR'::text NOT NULL,
    reference_type text DEFAULT ''::text NOT NULL,
    reference_id text DEFAULT ''::text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: owner_authority_grants; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.owner_authority_grants (
    tenant_id text NOT NULL,
    actor_id text NOT NULL,
    authority_id text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: package_templates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.package_templates (
    id text NOT NULL,
    tenant_id text NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    items jsonb DEFAULT '[]'::jsonb NOT NULL,
    version integer DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: policy_decisions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.policy_decisions (
    decision_id text NOT NULL,
    run_id text NOT NULL,
    call_id text,
    tool_name text NOT NULL,
    tool_version text DEFAULT 'v1'::text NOT NULL,
    result text NOT NULL,
    reason text,
    input_class text DEFAULT ''::text NOT NULL,
    output_class text DEFAULT ''::text NOT NULL,
    actor_id text NOT NULL,
    actor_roles jsonb,
    tenant_id text NOT NULL,
    property_id text NOT NULL,
    idempotency_key text DEFAULT ''::text NOT NULL,
    policy_version text DEFAULT ''::text NOT NULL,
    decided_at timestamp with time zone DEFAULT now() NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT policy_decisions_result_check CHECK ((result = ANY (ARRAY['allowed'::text, 'denied'::text, 'approval_required'::text, 'uncertainty'::text, 'exception'::text])))
);

--
-- Name: privacy_aadhaar_preferences; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.privacy_aadhaar_preferences (
    id text NOT NULL,
    tenant_id text NOT NULL,
    actor_id text NOT NULL,
    aadhaar_provided boolean DEFAULT false NOT NULL,
    aadhaar_masked text DEFAULT ''::text NOT NULL,
    verification_result boolean DEFAULT false NOT NULL,
    alternate_id_type text DEFAULT ''::text NOT NULL,
    alternate_id_value text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: privacy_consents; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.privacy_consents (
    id text NOT NULL,
    tenant_id text NOT NULL,
    actor_id text NOT NULL,
    purpose_id text NOT NULL,
    notice_id text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    lawful_basis text DEFAULT 'consent'::text NOT NULL,
    granted_at timestamp with time zone NOT NULL,
    withdrawn_at timestamp with time zone,
    expires_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: privacy_evaluation_exports; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.privacy_evaluation_exports (
    id text NOT NULL,
    tenant_id text NOT NULL,
    actor_id text DEFAULT ''::text NOT NULL,
    dataset_name text DEFAULT ''::text NOT NULL,
    dataset_scope text DEFAULT ''::text NOT NULL,
    is_deidentified boolean DEFAULT false NOT NULL,
    deidentification_method text DEFAULT ''::text NOT NULL,
    approved_by text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'created'::text NOT NULL,
    denial_reason text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: privacy_identity_alternatives; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.privacy_identity_alternatives (
    id text NOT NULL,
    tenant_id text NOT NULL,
    actor_id text NOT NULL,
    identity_type text NOT NULL,
    identity_value text DEFAULT ''::text NOT NULL,
    masked_value text DEFAULT ''::text NOT NULL,
    verification_hash text DEFAULT ''::text NOT NULL,
    verified boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: privacy_notices; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.privacy_notices (
    id text NOT NULL,
    tenant_id text NOT NULL,
    actor_id text NOT NULL,
    purpose_id text DEFAULT ''::text NOT NULL,
    notice_text text DEFAULT ''::text NOT NULL,
    version text DEFAULT '1.0'::text NOT NULL,
    language text DEFAULT 'en'::text NOT NULL,
    delivered_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: privacy_processor_contracts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.privacy_processor_contracts (
    id text NOT NULL,
    tenant_id text DEFAULT ''::text NOT NULL,
    vendor_name text NOT NULL,
    vendor_contact text DEFAULT ''::text NOT NULL,
    contract_reference text DEFAULT ''::text NOT NULL,
    processing_scope text DEFAULT ''::text NOT NULL,
    data_categories jsonb DEFAULT '[]'::jsonb NOT NULL,
    security_review_status text DEFAULT 'pending_review'::text NOT NULL,
    security_review_date timestamp with time zone,
    reviewer_id text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'pending_review'::text NOT NULL,
    expires_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: privacy_purposes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.privacy_purposes (
    id text NOT NULL,
    tenant_id text NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    data_categories jsonb DEFAULT '[]'::jsonb NOT NULL,
    lawful_basis text DEFAULT ''::text NOT NULL,
    retention_period_days integer DEFAULT 0 NOT NULL,
    active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: privacy_retention_records; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.privacy_retention_records (
    id text NOT NULL,
    tenant_id text NOT NULL,
    actor_id text DEFAULT ''::text NOT NULL,
    record_type text DEFAULT ''::text NOT NULL,
    record_description text DEFAULT ''::text NOT NULL,
    lawful_basis text DEFAULT ''::text NOT NULL,
    retain_until timestamp with time zone NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    reason text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: privacy_rights_requests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.privacy_rights_requests (
    id text NOT NULL,
    tenant_id text NOT NULL,
    actor_id text NOT NULL,
    request_type text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    related_data text DEFAULT ''::text NOT NULL,
    correction_data text DEFAULT ''::text NOT NULL,
    response_data text DEFAULT ''::text NOT NULL,
    block_reason text DEFAULT ''::text NOT NULL,
    reviewed_by text DEFAULT ''::text NOT NULL,
    reviewed_at timestamp with time zone,
    completed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: privacy_security_log_settings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.privacy_security_log_settings (
    id text NOT NULL,
    tenant_id text NOT NULL,
    region text DEFAULT 'IN'::text NOT NULL,
    retention_years integer DEFAULT 5 NOT NULL,
    incident_report_process text DEFAULT ''::text NOT NULL,
    active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: privileged_access_log; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.privileged_access_log (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    actor_id text NOT NULL,
    tenant_id text,
    action text NOT NULL,
    resource_type text DEFAULT ''::text NOT NULL,
    resource_id text DEFAULT ''::text NOT NULL,
    mfa_used boolean DEFAULT false NOT NULL,
    success boolean DEFAULT false NOT NULL,
    details jsonb,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: properties; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.properties (
    id text NOT NULL,
    tenant_id text NOT NULL,
    owner_authority_id text NOT NULL,
    service_address jsonb NOT NULL,
    geolocation_zone text NOT NULL,
    timezone text DEFAULT 'Asia/Kolkata'::text NOT NULL,
    emergency_contacts jsonb DEFAULT '[]'::jsonb NOT NULL,
    access_method text NOT NULL,
    maximum_occupancy integer NOT NULL,
    state text NOT NULL,
    owner_contract_accepted boolean DEFAULT false NOT NULL,
    compliance_complete boolean DEFAULT false NOT NULL,
    mandatory_fields_set boolean DEFAULT false NOT NULL,
    version integer DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: property_access_secrets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.property_access_secrets (
    id text NOT NULL,
    tenant_id text NOT NULL,
    property_id text NOT NULL,
    secret_type text NOT NULL,
    label text DEFAULT ''::text NOT NULL,
    encrypted_value text NOT NULL,
    encryption_key_id text DEFAULT ''::text NOT NULL,
    metadata text DEFAULT ''::text NOT NULL,
    version integer DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: property_compliance_holds; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.property_compliance_holds (
    id text NOT NULL,
    property_id text NOT NULL,
    tenant_id text NOT NULL,
    kind text NOT NULL,
    severity text NOT NULL,
    status text DEFAULT 'open'::text NOT NULL,
    reason text NOT NULL,
    expires_at timestamp with time zone,
    exception_by text,
    exception_at timestamp with time zone,
    exception_expires_at timestamp with time zone,
    resolved_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: property_package_items; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.property_package_items (
    id text NOT NULL,
    tenant_id text NOT NULL,
    package_version_id text NOT NULL,
    catalog_item_id text NOT NULL,
    sku text NOT NULL,
    name text DEFAULT ''::text NOT NULL,
    label text DEFAULT ''::text NOT NULL,
    substitution_group text DEFAULT ''::text NOT NULL,
    quantity integer NOT NULL,
    order_index integer DEFAULT 0 NOT NULL,
    expected_monthly_consumption integer DEFAULT 0 NOT NULL,
    setup_cost_minor_units bigint DEFAULT 0 NOT NULL,
    monthly_cost_minor_units bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: property_package_versions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.property_package_versions (
    id text NOT NULL,
    tenant_id text NOT NULL,
    property_id text NOT NULL,
    version_number integer NOT NULL,
    status text DEFAULT 'draft'::text NOT NULL,
    effective_date timestamp with time zone NOT NULL,
    monthly_budget_limit_minor_units bigint,
    substitution_policy text DEFAULT 'owner_approval'::text NOT NULL,
    require_approval_for_price_increase boolean DEFAULT false NOT NULL,
    require_approval_for_new_sku boolean DEFAULT false NOT NULL,
    setup_cost_minor_units bigint DEFAULT 0 NOT NULL,
    monthly_cost_minor_units bigint DEFAULT 0 NOT NULL,
    monthly_consumption_units bigint DEFAULT 0 NOT NULL,
    currency text NOT NULL,
    review_summary jsonb NOT NULL,
    created_by text DEFAULT ''::text NOT NULL,
    activated_at timestamp with time zone,
    version integer DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: property_transitions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.property_transitions (
    id text NOT NULL,
    property_id text NOT NULL,
    tenant_id text NOT NULL,
    from_state text NOT NULL,
    to_state text NOT NULL,
    actor_id text NOT NULL,
    reason text NOT NULL,
    from_version integer NOT NULL,
    to_version integer NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: purchase_order_items; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.purchase_order_items (
    id text NOT NULL,
    tenant_id text NOT NULL,
    purchase_order_id text NOT NULL,
    requisition_item_id text DEFAULT ''::text NOT NULL,
    catalog_item_id text DEFAULT ''::text NOT NULL,
    quantity bigint DEFAULT 0 NOT NULL,
    unit_cost_minor_units bigint DEFAULT 0 NOT NULL,
    currency text DEFAULT 'INR'::text NOT NULL,
    line_total_minor_units bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: purchase_orders; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.purchase_orders (
    id text NOT NULL,
    tenant_id text NOT NULL,
    requisition_id text DEFAULT ''::text NOT NULL,
    supplier_id text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'draft'::text NOT NULL,
    ordered_by text DEFAULT ''::text NOT NULL,
    total_minor_units bigint DEFAULT 0 NOT NULL,
    currency text DEFAULT 'INR'::text NOT NULL,
    order_date timestamp with time zone DEFAULT now() NOT NULL,
    expected_delivery timestamp with time zone,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: queued_offline_evidence; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.queued_offline_evidence (
    id text NOT NULL,
    tenant_id text NOT NULL,
    ticket_id text NOT NULL,
    checklist_item_id text,
    content_hash text NOT NULL,
    file_name text,
    content_type text,
    size_bytes bigint DEFAULT 0 NOT NULL,
    status text DEFAULT 'queued'::text NOT NULL,
    captured_by text DEFAULT ''::text NOT NULL,
    captured_at timestamp with time zone DEFAULT now() NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: reconciliation_exceptions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reconciliation_exceptions (
    id text NOT NULL,
    tenant_id text NOT NULL,
    property_id text DEFAULT ''::text NOT NULL,
    entry_id text DEFAULT ''::text NOT NULL,
    entry_type text DEFAULT ''::text NOT NULL,
    exception_type text DEFAULT ''::text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'open'::text NOT NULL,
    recorded_by text DEFAULT ''::text NOT NULL,
    resolved_by text DEFAULT ''::text NOT NULL,
    resolved_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: report_snapshots; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.report_snapshots (
    id text NOT NULL,
    tenant_id text NOT NULL,
    property_id text DEFAULT ''::text NOT NULL,
    kind text NOT NULL,
    period_start timestamp with time zone,
    period_end timestamp with time zone,
    source_count bigint DEFAULT 0 NOT NULL,
    source_hash text DEFAULT ''::text NOT NULL,
    data jsonb DEFAULT '{}'::jsonb NOT NULL,
    built_at timestamp with time zone DEFAULT now() NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: requisition_approvals; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.requisition_approvals (
    id text NOT NULL,
    tenant_id text NOT NULL,
    requisition_id text NOT NULL,
    actor_id text DEFAULT ''::text NOT NULL,
    decision text DEFAULT ''::text NOT NULL,
    reason text DEFAULT ''::text NOT NULL,
    is_ai_actor boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: requisition_items; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.requisition_items (
    id text NOT NULL,
    tenant_id text NOT NULL,
    requisition_id text NOT NULL,
    catalog_item_id text DEFAULT ''::text NOT NULL,
    supplier_item_id text DEFAULT ''::text NOT NULL,
    quantity bigint DEFAULT 0 NOT NULL,
    unit_cost_minor_units bigint DEFAULT 0 NOT NULL,
    unit_cost_currency text DEFAULT 'INR'::text NOT NULL,
    line_total_minor_units bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: requisitions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.requisitions (
    id text NOT NULL,
    tenant_id text NOT NULL,
    property_id text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'draft'::text NOT NULL,
    created_by text DEFAULT ''::text NOT NULL,
    approved_by text DEFAULT ''::text NOT NULL,
    rejected_by text DEFAULT ''::text NOT NULL,
    total_cost_minor_units bigint DEFAULT 0 NOT NULL,
    currency text DEFAULT 'INR'::text NOT NULL,
    notes text DEFAULT ''::text NOT NULL,
    new_supplier_ids jsonb DEFAULT '[]'::jsonb NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: reservation_conflict_resolutions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reservation_conflict_resolutions (
    id text NOT NULL,
    tenant_id text NOT NULL,
    conflict_id text NOT NULL,
    actor_id text NOT NULL,
    actor_type text DEFAULT 'operator'::text NOT NULL,
    outcome text NOT NULL,
    note text,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: reservation_conflicts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reservation_conflicts (
    id text NOT NULL,
    tenant_id text NOT NULL,
    property_id text NOT NULL,
    kind text NOT NULL,
    severity text NOT NULL,
    status text DEFAULT 'open'::text NOT NULL,
    message text NOT NULL,
    reservation_ids text[] DEFAULT '{}'::text[] NOT NULL,
    exception_id text,
    dedupe_key text DEFAULT ''::text NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    resolved_at timestamp with time zone
);

--
-- Name: reservations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reservations (
    id text NOT NULL,
    tenant_id text NOT NULL,
    property_id text NOT NULL,
    feed_id text NOT NULL,
    external_event_id text NOT NULL,
    source text NOT NULL,
    guest_summary text,
    status text DEFAULT 'active'::text NOT NULL,
    start_at timestamp with time zone NOT NULL,
    end_at timestamp with time zone NOT NULL,
    all_day boolean DEFAULT false NOT NULL,
    timezone text,
    sequence integer DEFAULT 0 NOT NULL,
    version integer DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: route_plans; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.route_plans (
    id text NOT NULL,
    tenant_id text NOT NULL,
    worker_id text NOT NULL,
    planned_date date NOT NULL,
    total_travel_minutes integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: route_stops; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.route_stops (
    id text NOT NULL,
    tenant_id text NOT NULL,
    route_plan_id text NOT NULL,
    ticket_id text NOT NULL,
    property_id text NOT NULL,
    sequence integer DEFAULT 0 NOT NULL,
    estimated_arrival timestamp with time zone,
    estimated_departure timestamp with time zone,
    travel_from_previous_minutes integer DEFAULT 0 NOT NULL
);

--
-- Name: service_contract_versions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.service_contract_versions (
    id text NOT NULL,
    agreement_id text NOT NULL,
    tenant_id text NOT NULL,
    version_number integer NOT NULL,
    content_hash text NOT NULL,
    terms jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: service_contracts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.service_contracts (
    id text NOT NULL,
    tenant_id text NOT NULL,
    property_id text NOT NULL,
    status text DEFAULT 'draft'::text NOT NULL,
    current_version integer DEFAULT 0 NOT NULL,
    version integer DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: service_recoveries; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.service_recoveries (
    id text NOT NULL,
    tenant_id text NOT NULL,
    property_id text NOT NULL,
    incident_ticket_id text NOT NULL,
    follow_up_ticket_id text,
    severity text DEFAULT 'low'::text NOT NULL,
    original_reason text DEFAULT ''::text NOT NULL,
    original_evidence_hashes jsonb DEFAULT '[]'::jsonb,
    responsibility text DEFAULT ''::text NOT NULL,
    rework_cost_minor bigint DEFAULT 0 NOT NULL,
    currency text,
    status text DEFAULT 'open'::text NOT NULL,
    created_by text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: session_revocations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.session_revocations (
    session_id text NOT NULL,
    reason text DEFAULT ''::text NOT NULL,
    revoked_at timestamp with time zone DEFAULT now() NOT NULL,
    revoked_by text DEFAULT ''::text NOT NULL
);

--
-- Name: sessions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sessions (
    id text NOT NULL,
    user_id uuid NOT NULL,
    tenant_id text NOT NULL,
    actor_id text NOT NULL,
    roles jsonb DEFAULT '[]'::jsonb NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: sos_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sos_events (
    id text NOT NULL,
    tenant_id text NOT NULL,
    worker_id text NOT NULL,
    ticket_id text,
    location text DEFAULT ''::text,
    triggered_at timestamp with time zone DEFAULT now() NOT NULL,
    acknowledged_by text,
    acknowledged_at timestamp with time zone,
    resolution text,
    resolved_at timestamp with time zone
);

--
-- Name: stock_locations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.stock_locations (
    id text NOT NULL,
    tenant_id text NOT NULL,
    property_id text DEFAULT ''::text NOT NULL,
    name text NOT NULL,
    location_type text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: submission_packets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.submission_packets (
    id text NOT NULL,
    tenant_id text NOT NULL,
    property_id text NOT NULL,
    status text DEFAULT 'draft'::text NOT NULL,
    document_ids jsonb DEFAULT '[]'::jsonb NOT NULL,
    created_by text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    submitted_at timestamp with time zone,
    version integer DEFAULT 1 NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: submission_receipts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.submission_receipts (
    id text NOT NULL,
    packet_id text NOT NULL,
    tenant_id text NOT NULL,
    confirmed_by text NOT NULL,
    receipt_hash text NOT NULL,
    document_version_refs jsonb DEFAULT '[]'::jsonb NOT NULL,
    confirmed_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: superhost_account_tasks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.superhost_account_tasks (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id text NOT NULL,
    actor_id text NOT NULL,
    property_id text,
    description text NOT NULL,
    status text DEFAULT 'open'::text NOT NULL,
    resolved_note text,
    origin_run_id text,
    resolved_run_id text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT superhost_account_tasks_status_check CHECK ((status = ANY (ARRAY['open'::text, 'done'::text, 'blocked'::text])))
);

--
-- Name: superhost_threads; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.superhost_threads (
    thread_id text NOT NULL,
    run_id text NOT NULL,
    tenant_id text NOT NULL,
    property_id text NOT NULL,
    purpose text NOT NULL,
    idempotency_key text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    actor_id text DEFAULT ''::text NOT NULL
);

--
-- Name: supplier_items; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.supplier_items (
    id text NOT NULL,
    tenant_id text NOT NULL,
    supplier_id text NOT NULL,
    catalog_item_id text NOT NULL,
    supplier_sku text DEFAULT ''::text NOT NULL,
    unit_cost_minor_units bigint DEFAULT 0 NOT NULL,
    unit_cost_currency text DEFAULT 'INR'::text NOT NULL,
    lead_time_days integer DEFAULT 0 NOT NULL,
    minimum_order_quantity bigint DEFAULT 1 NOT NULL,
    is_preferred boolean DEFAULT false NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: supplier_rebates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.supplier_rebates (
    id text NOT NULL,
    tenant_id text NOT NULL,
    supplier_id text DEFAULT ''::text NOT NULL,
    purchase_order_id text DEFAULT ''::text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    amount_minor_units bigint DEFAULT 0 NOT NULL,
    currency text DEFAULT 'INR'::text NOT NULL,
    status text DEFAULT 'offered'::text NOT NULL,
    offered_at timestamp with time zone DEFAULT now() NOT NULL,
    settled_at timestamp with time zone,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: suppliers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.suppliers (
    id text NOT NULL,
    tenant_id text NOT NULL,
    name text NOT NULL,
    contact_info text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'pending_approval'::text NOT NULL,
    created_by text DEFAULT ''::text NOT NULL,
    approved_by text DEFAULT ''::text NOT NULL,
    approved_at timestamp with time zone,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: support_access_grants; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.support_access_grants (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id text NOT NULL,
    granted_by_user_id text NOT NULL,
    granted_to_user_id text NOT NULL,
    reason text NOT NULL,
    scope text DEFAULT 'tenant'::text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: tenants; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.tenants (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    state text DEFAULT 'active'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: ticket_assignments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ticket_assignments (
    id text NOT NULL,
    tenant_id text NOT NULL,
    ticket_id text NOT NULL,
    worker_id text NOT NULL,
    assigned_by text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'offered'::text NOT NULL,
    accept_until timestamp with time zone,
    accepted_at timestamp with time zone,
    version integer DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: ticket_checklist_items; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ticket_checklist_items (
    id text NOT NULL,
    ticket_id text NOT NULL,
    tenant_id text NOT NULL,
    template_item_index integer DEFAULT 0 NOT NULL,
    label text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    completed_by text,
    completed_at timestamp with time zone,
    evidence_ids jsonb DEFAULT '[]'::jsonb,
    notes text,
    version integer DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    evidence_required boolean DEFAULT false NOT NULL
);

--
-- Name: ticket_evidence; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ticket_evidence (
    id text NOT NULL,
    tenant_id text NOT NULL,
    ticket_id text NOT NULL,
    checklist_item_id text,
    object_id text,
    content_hash text NOT NULL,
    file_name text,
    content_type text,
    size_bytes bigint DEFAULT 0 NOT NULL,
    status text DEFAULT 'accepted'::text NOT NULL,
    captured_by text DEFAULT ''::text NOT NULL,
    captured_at timestamp with time zone DEFAULT now() NOT NULL,
    version integer DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: ticket_state_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ticket_state_events (
    id text NOT NULL,
    ticket_id text NOT NULL,
    tenant_id text NOT NULL,
    from_state text NOT NULL,
    to_state text NOT NULL,
    actor_id text NOT NULL,
    reason text DEFAULT ''::text NOT NULL,
    evidence jsonb DEFAULT '[]'::jsonb,
    version integer DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: tickets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.tickets (
    id text NOT NULL,
    tenant_id text NOT NULL,
    property_id text NOT NULL,
    type text NOT NULL,
    status text DEFAULT 'draft'::text NOT NULL,
    reason text DEFAULT ''::text NOT NULL,
    requested_window jsonb DEFAULT '{}'::jsonb,
    checklist_version_id text,
    created_by text DEFAULT ''::text NOT NULL,
    assigned_to text,
    verified_by text,
    verifier_note text,
    blocker jsonb,
    follow_up_ticket_id text,
    reopen_reason text,
    notification_intent text,
    version integer DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    severity text
);

--
-- Name: time_entries; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.time_entries (
    id text NOT NULL,
    tenant_id text NOT NULL,
    worker_id text NOT NULL,
    ticket_id text,
    work_minutes integer DEFAULT 0 NOT NULL,
    travel_minutes integer DEFAULT 0 NOT NULL,
    overtime_flag boolean DEFAULT false NOT NULL,
    recorded_by text DEFAULT ''::text NOT NULL,
    recorded_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: turnover_proposals; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.turnover_proposals (
    id text NOT NULL,
    tenant_id text NOT NULL,
    property_id text NOT NULL,
    reservation_id text NOT NULL,
    kind text NOT NULL,
    status text DEFAULT 'proposed'::text NOT NULL,
    scheduled_at timestamp with time zone NOT NULL,
    checklist_hint text DEFAULT ''::text NOT NULL,
    version integer DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.users (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id text NOT NULL,
    contact text NOT NULL,
    role text NOT NULL,
    state text DEFAULT 'active'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: vendor_work_orders; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.vendor_work_orders (
    id text NOT NULL,
    tenant_id text NOT NULL,
    request_id text DEFAULT ''::text NOT NULL,
    estimate_id text DEFAULT ''::text NOT NULL,
    property_id text DEFAULT ''::text NOT NULL,
    vendor_id text DEFAULT ''::text NOT NULL,
    scope text DEFAULT ''::text NOT NULL,
    risk_level text DEFAULT 'standard'::text NOT NULL,
    status text DEFAULT 'assigned'::text NOT NULL,
    assigned_by text DEFAULT ''::text NOT NULL,
    assigned_at timestamp with time zone DEFAULT now() NOT NULL,
    started_at timestamp with time zone,
    completed_by text DEFAULT ''::text NOT NULL,
    completed_at timestamp with time zone,
    completion_evidence_ref text DEFAULT ''::text NOT NULL,
    verified_by text DEFAULT ''::text NOT NULL,
    verified_at timestamp with time zone,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: warranty_records; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.warranty_records (
    id text NOT NULL,
    tenant_id text NOT NULL,
    work_order_id text DEFAULT ''::text NOT NULL,
    property_id text DEFAULT ''::text NOT NULL,
    vendor_id text DEFAULT ''::text NOT NULL,
    provider text DEFAULT ''::text NOT NULL,
    coverage text DEFAULT ''::text NOT NULL,
    expires_at timestamp with time zone,
    status text DEFAULT 'active'::text NOT NULL,
    recorded_by text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: worker_certifications; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.worker_certifications (
    id text NOT NULL,
    tenant_id text NOT NULL,
    worker_id text NOT NULL,
    work_type text NOT NULL,
    issuer text DEFAULT ''::text NOT NULL,
    issued_at timestamp with time zone NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    status text DEFAULT 'valid'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: worker_ratings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.worker_ratings (
    id text NOT NULL,
    tenant_id text NOT NULL,
    worker_id text NOT NULL,
    score integer NOT NULL,
    source text NOT NULL,
    comment text,
    recorded_by text DEFAULT ''::text NOT NULL,
    recorded_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: workers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.workers (
    id text NOT NULL,
    tenant_id text NOT NULL,
    legal_name text NOT NULL,
    verified_identity boolean DEFAULT false NOT NULL,
    date_of_birth timestamp with time zone NOT NULL,
    age_eligible boolean DEFAULT false NOT NULL,
    contact_method text DEFAULT ''::text NOT NULL,
    classification text NOT NULL,
    specialist boolean DEFAULT false NOT NULL,
    service_zone text DEFAULT ''::text NOT NULL,
    skills jsonb DEFAULT '[]'::jsonb,
    status text DEFAULT 'active'::text NOT NULL,
    version integer DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: workforce_assignments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.workforce_assignments (
    id text NOT NULL,
    tenant_id text NOT NULL,
    worker_id text NOT NULL,
    work_type text DEFAULT 'general'::text NOT NULL,
    assigned_by text DEFAULT ''::text NOT NULL,
    assigned_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: access_custody_events access_custody_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.access_custody_events
    ADD CONSTRAINT access_custody_events_pkey PRIMARY KEY (id);

--
-- Name: access_disclosures access_disclosures_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.access_disclosures
    ADD CONSTRAINT access_disclosures_pkey PRIMARY KEY (id);

--
-- Name: access_grants access_grants_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.access_grants
    ADD CONSTRAINT access_grants_pkey PRIMARY KEY (id);

--
-- Name: access_holds access_holds_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.access_holds
    ADD CONSTRAINT access_holds_pkey PRIMARY KEY (id);

--
-- Name: accounting_exports accounting_exports_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.accounting_exports
    ADD CONSTRAINT accounting_exports_pkey PRIMARY KEY (id);

--
-- Name: adverse_action_reviews adverse_action_reviews_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.adverse_action_reviews
    ADD CONSTRAINT adverse_action_reviews_pkey PRIMARY KEY (id);

--
-- Name: agent_run_events agent_run_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_run_events
    ADD CONSTRAINT agent_run_events_pkey PRIMARY KEY (event_id);

--
-- Name: agent_runs agent_runs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_runs
    ADD CONSTRAINT agent_runs_pkey PRIMARY KEY (run_id);

--
-- Name: ai_tool_calls ai_tool_calls_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_tool_calls
    ADD CONSTRAINT ai_tool_calls_pkey PRIMARY KEY (call_id);

--
-- Name: approval_requests approval_requests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_requests
    ADD CONSTRAINT approval_requests_pkey PRIMARY KEY (request_id);

--
-- Name: audit_events audit_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.audit_events
    ADD CONSTRAINT audit_events_pkey PRIMARY KEY (id);

--
-- Name: authentication_methods authentication_methods_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.authentication_methods
    ADD CONSTRAINT authentication_methods_pkey PRIMARY KEY (id);

--
-- Name: availability_windows availability_windows_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.availability_windows
    ADD CONSTRAINT availability_windows_pkey PRIMARY KEY (id);

--
-- Name: bank_verifications bank_verifications_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bank_verifications
    ADD CONSTRAINT bank_verifications_pkey PRIMARY KEY (id);

--
-- Name: calendar_exceptions calendar_exceptions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.calendar_exceptions
    ADD CONSTRAINT calendar_exceptions_pkey PRIMARY KEY (id);

--
-- Name: calendar_feeds calendar_feeds_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.calendar_feeds
    ADD CONSTRAINT calendar_feeds_pkey PRIMARY KEY (id);

--
-- Name: catalog_claim_evidence catalog_claim_evidence_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.catalog_claim_evidence
    ADD CONSTRAINT catalog_claim_evidence_pkey PRIMARY KEY (id);

--
-- Name: catalog_items catalog_items_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.catalog_items
    ADD CONSTRAINT catalog_items_pkey PRIMARY KEY (id);

--
-- Name: catalog_items catalog_items_tenant_id_sku_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.catalog_items
    ADD CONSTRAINT catalog_items_tenant_id_sku_key UNIQUE (tenant_id, sku);

--
-- Name: charges charges_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.charges
    ADD CONSTRAINT charges_pkey PRIMARY KEY (id);

--
-- Name: checklist_sync_conflicts checklist_sync_conflicts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.checklist_sync_conflicts
    ADD CONSTRAINT checklist_sync_conflicts_pkey PRIMARY KEY (id);

--
-- Name: checklist_sync_records checklist_sync_records_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.checklist_sync_records
    ADD CONSTRAINT checklist_sync_records_pkey PRIMARY KEY (id);

--
-- Name: checklist_template_versions checklist_template_versions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.checklist_template_versions
    ADD CONSTRAINT checklist_template_versions_pkey PRIMARY KEY (id);

--
-- Name: checklist_templates checklist_templates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.checklist_templates
    ADD CONSTRAINT checklist_templates_pkey PRIMARY KEY (id);

--
-- Name: communication_drafts communication_drafts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.communication_drafts
    ADD CONSTRAINT communication_drafts_pkey PRIMARY KEY (id);

--
-- Name: communication_preferences communication_preferences_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.communication_preferences
    ADD CONSTRAINT communication_preferences_pkey PRIMARY KEY (id);

--
-- Name: communication_preferences communication_preferences_tenant_id_recipient_id_audience_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.communication_preferences
    ADD CONSTRAINT communication_preferences_tenant_id_recipient_id_audience_key UNIQUE (tenant_id, recipient_id, audience);

--
-- Name: communication_reviews communication_reviews_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.communication_reviews
    ADD CONSTRAINT communication_reviews_pkey PRIMARY KEY (id);

--
-- Name: compliance_items compliance_items_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.compliance_items
    ADD CONSTRAINT compliance_items_pkey PRIMARY KEY (id);

--
-- Name: compliance_renewal_warnings compliance_renewal_warnings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.compliance_renewal_warnings
    ADD CONSTRAINT compliance_renewal_warnings_pkey PRIMARY KEY (id);

--
-- Name: consumer_acceptances consumer_acceptances_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.consumer_acceptances
    ADD CONSTRAINT consumer_acceptances_pkey PRIMARY KEY (id);

--
-- Name: consumer_disclosures consumer_disclosures_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.consumer_disclosures
    ADD CONSTRAINT consumer_disclosures_pkey PRIMARY KEY (id);

--
-- Name: consumer_history_exports consumer_history_exports_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.consumer_history_exports
    ADD CONSTRAINT consumer_history_exports_pkey PRIMARY KEY (id);

--
-- Name: contract_acceptances contract_acceptances_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contract_acceptances
    ADD CONSTRAINT contract_acceptances_pkey PRIMARY KEY (id);

--
-- Name: conversation_links conversation_links_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.conversation_links
    ADD CONSTRAINT conversation_links_pkey PRIMARY KEY (id);

--
-- Name: conversation_links conversation_links_token_hash_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.conversation_links
    ADD CONSTRAINT conversation_links_token_hash_key UNIQUE (token_hash);

--
-- Name: credits credits_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.credits
    ADD CONSTRAINT credits_pkey PRIMARY KEY (id);

--
-- Name: deliveries deliveries_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deliveries
    ADD CONSTRAINT deliveries_pkey PRIMARY KEY (id);

--
-- Name: dispatch_overrides dispatch_overrides_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dispatch_overrides
    ADD CONSTRAINT dispatch_overrides_pkey PRIMARY KEY (id);

--
-- Name: document_extractions document_extractions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.document_extractions
    ADD CONSTRAINT document_extractions_pkey PRIMARY KEY (id);

--
-- Name: document_reviews document_reviews_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.document_reviews
    ADD CONSTRAINT document_reviews_pkey PRIMARY KEY (id);

--
-- Name: document_versions document_versions_document_id_content_hash_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.document_versions
    ADD CONSTRAINT document_versions_document_id_content_hash_key UNIQUE (document_id, content_hash);

--
-- Name: document_versions document_versions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.document_versions
    ADD CONSTRAINT document_versions_pkey PRIMARY KEY (id);

--
-- Name: documents documents_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.documents
    ADD CONSTRAINT documents_pkey PRIMARY KEY (id);

--
-- Name: employment_terms employment_terms_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.employment_terms
    ADD CONSTRAINT employment_terms_pkey PRIMARY KEY (id);

--
-- Name: encryption_keys encryption_keys_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.encryption_keys
    ADD CONSTRAINT encryption_keys_pkey PRIMARY KEY (id);

--
-- Name: expenses expenses_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.expenses
    ADD CONSTRAINT expenses_pkey PRIMARY KEY (id);

--
-- Name: external_calendar_events external_calendar_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.external_calendar_events
    ADD CONSTRAINT external_calendar_events_pkey PRIMARY KEY (id);

--
-- Name: fee_rules fee_rules_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.fee_rules
    ADD CONSTRAINT fee_rules_pkey PRIMARY KEY (id);

--
-- Name: fee_rules fee_rules_rule_version_currency_service_tier_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.fee_rules
    ADD CONSTRAINT fee_rules_rule_version_currency_service_tier_key UNIQUE (rule_version, currency, service_tier);

--
-- Name: file_grants file_grants_grant_token_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.file_grants
    ADD CONSTRAINT file_grants_grant_token_key UNIQUE (grant_token);

--
-- Name: file_grants file_grants_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.file_grants
    ADD CONSTRAINT file_grants_pkey PRIMARY KEY (id);

--
-- Name: file_objects file_objects_object_key_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.file_objects
    ADD CONSTRAINT file_objects_object_key_key UNIQUE (object_key);

--
-- Name: file_objects file_objects_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.file_objects
    ADD CONSTRAINT file_objects_pkey PRIMARY KEY (id);

--
-- Name: financial_approvals financial_approvals_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.financial_approvals
    ADD CONSTRAINT financial_approvals_pkey PRIMARY KEY (id);

--
-- Name: fleet_assets fleet_assets_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.fleet_assets
    ADD CONSTRAINT fleet_assets_pkey PRIMARY KEY (id);

--
-- Name: fleet_batteries fleet_batteries_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.fleet_batteries
    ADD CONSTRAINT fleet_batteries_pkey PRIMARY KEY (id);

--
-- Name: fleet_custody_events fleet_custody_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.fleet_custody_events
    ADD CONSTRAINT fleet_custody_events_pkey PRIMARY KEY (id);

--
-- Name: fleet_incidents fleet_incidents_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.fleet_incidents
    ADD CONSTRAINT fleet_incidents_pkey PRIMARY KEY (id);

--
-- Name: fleet_inspections fleet_inspections_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.fleet_inspections
    ADD CONSTRAINT fleet_inspections_pkey PRIMARY KEY (id);

--
-- Name: fleet_maintenance fleet_maintenance_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.fleet_maintenance
    ADD CONSTRAINT fleet_maintenance_pkey PRIMARY KEY (id);

--
-- Name: fleet_tracking_events fleet_tracking_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.fleet_tracking_events
    ADD CONSTRAINT fleet_tracking_events_pkey PRIMARY KEY (id);

--
-- Name: goods_receipt_items goods_receipt_items_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.goods_receipt_items
    ADD CONSTRAINT goods_receipt_items_pkey PRIMARY KEY (id);

--
-- Name: goods_receipts goods_receipts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.goods_receipts
    ADD CONSTRAINT goods_receipts_pkey PRIMARY KEY (id);

--
-- Name: grievances grievances_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.grievances
    ADD CONSTRAINT grievances_pkey PRIMARY KEY (id);

--
-- Name: hermes_deliveries hermes_deliveries_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.hermes_deliveries
    ADD CONSTRAINT hermes_deliveries_pkey PRIMARY KEY (delivery_id);

--
-- Name: hermes_drafts hermes_drafts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.hermes_drafts
    ADD CONSTRAINT hermes_drafts_pkey PRIMARY KEY (draft_id);

--
-- Name: hermes_reviews hermes_reviews_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.hermes_reviews
    ADD CONSTRAINT hermes_reviews_pkey PRIMARY KEY (review_id);

--
-- Name: incident_alerts incident_alerts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.incident_alerts
    ADD CONSTRAINT incident_alerts_pkey PRIMARY KEY (id);

--
-- Name: inventory_count_lines inventory_count_lines_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inventory_count_lines
    ADD CONSTRAINT inventory_count_lines_pkey PRIMARY KEY (id);

--
-- Name: inventory_counts inventory_counts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inventory_counts
    ADD CONSTRAINT inventory_counts_pkey PRIMARY KEY (id);

--
-- Name: inventory_movements inventory_movements_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inventory_movements
    ADD CONSTRAINT inventory_movements_pkey PRIMARY KEY (id);

--
-- Name: invoice_lines invoice_lines_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.invoice_lines
    ADD CONSTRAINT invoice_lines_pkey PRIMARY KEY (id);

--
-- Name: invoices invoices_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.invoices
    ADD CONSTRAINT invoices_pkey PRIMARY KEY (id);

--
-- Name: maintenance_approvals maintenance_approvals_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.maintenance_approvals
    ADD CONSTRAINT maintenance_approvals_pkey PRIMARY KEY (id);

--
-- Name: maintenance_estimates maintenance_estimates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.maintenance_estimates
    ADD CONSTRAINT maintenance_estimates_pkey PRIMARY KEY (id);

--
-- Name: maintenance_requests maintenance_requests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.maintenance_requests
    ADD CONSTRAINT maintenance_requests_pkey PRIMARY KEY (id);

--
-- Name: maker_checker_requests maker_checker_requests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.maker_checker_requests
    ADD CONSTRAINT maker_checker_requests_pkey PRIMARY KEY (id);

--
-- Name: memberships memberships_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.memberships
    ADD CONSTRAINT memberships_pkey PRIMARY KEY (id);

--
-- Name: memberships memberships_tenant_id_user_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.memberships
    ADD CONSTRAINT memberships_tenant_id_user_id_key UNIQUE (tenant_id, user_id);

--
-- Name: message_template_versions message_template_versions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.message_template_versions
    ADD CONSTRAINT message_template_versions_pkey PRIMARY KEY (id);

--
-- Name: message_template_versions message_template_versions_template_id_version_language_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.message_template_versions
    ADD CONSTRAINT message_template_versions_template_id_version_language_key UNIQUE (template_id, version, language);

--
-- Name: message_templates message_templates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.message_templates
    ADD CONSTRAINT message_templates_pkey PRIMARY KEY (id);

--
-- Name: message_templates message_templates_tenant_id_template_key_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.message_templates
    ADD CONSTRAINT message_templates_tenant_id_template_key_key UNIQUE (tenant_id, template_key);

--
-- Name: metric_observations metric_observations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.metric_observations
    ADD CONSTRAINT metric_observations_pkey PRIMARY KEY (id);

--
-- Name: mfa_methods mfa_methods_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mfa_methods
    ADD CONSTRAINT mfa_methods_pkey PRIMARY KEY (id);

--
-- Name: onboarding_cases onboarding_cases_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.onboarding_cases
    ADD CONSTRAINT onboarding_cases_pkey PRIMARY KEY (id);

--
-- Name: onboarding_evidence onboarding_evidence_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.onboarding_evidence
    ADD CONSTRAINT onboarding_evidence_pkey PRIMARY KEY (id);

--
-- Name: onboarding_inspections onboarding_inspections_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.onboarding_inspections
    ADD CONSTRAINT onboarding_inspections_pkey PRIMARY KEY (id);

--
-- Name: operational_subledger_entries operational_subledger_entries_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.operational_subledger_entries
    ADD CONSTRAINT operational_subledger_entries_pkey PRIMARY KEY (id);

--
-- Name: owner_authority_grants owner_authority_grants_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.owner_authority_grants
    ADD CONSTRAINT owner_authority_grants_pkey PRIMARY KEY (actor_id, authority_id);

--
-- Name: package_templates package_templates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.package_templates
    ADD CONSTRAINT package_templates_pkey PRIMARY KEY (id);

--
-- Name: policy_decisions policy_decisions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.policy_decisions
    ADD CONSTRAINT policy_decisions_pkey PRIMARY KEY (decision_id);

--
-- Name: privacy_aadhaar_preferences privacy_aadhaar_preferences_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.privacy_aadhaar_preferences
    ADD CONSTRAINT privacy_aadhaar_preferences_pkey PRIMARY KEY (id);

--
-- Name: privacy_consents privacy_consents_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.privacy_consents
    ADD CONSTRAINT privacy_consents_pkey PRIMARY KEY (id);

--
-- Name: privacy_evaluation_exports privacy_evaluation_exports_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.privacy_evaluation_exports
    ADD CONSTRAINT privacy_evaluation_exports_pkey PRIMARY KEY (id);

--
-- Name: privacy_identity_alternatives privacy_identity_alternatives_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.privacy_identity_alternatives
    ADD CONSTRAINT privacy_identity_alternatives_pkey PRIMARY KEY (id);

--
-- Name: privacy_notices privacy_notices_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.privacy_notices
    ADD CONSTRAINT privacy_notices_pkey PRIMARY KEY (id);

--
-- Name: privacy_processor_contracts privacy_processor_contracts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.privacy_processor_contracts
    ADD CONSTRAINT privacy_processor_contracts_pkey PRIMARY KEY (id);

--
-- Name: privacy_purposes privacy_purposes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.privacy_purposes
    ADD CONSTRAINT privacy_purposes_pkey PRIMARY KEY (id);

--
-- Name: privacy_retention_records privacy_retention_records_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.privacy_retention_records
    ADD CONSTRAINT privacy_retention_records_pkey PRIMARY KEY (id);

--
-- Name: privacy_rights_requests privacy_rights_requests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.privacy_rights_requests
    ADD CONSTRAINT privacy_rights_requests_pkey PRIMARY KEY (id);

--
-- Name: privacy_security_log_settings privacy_security_log_settings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.privacy_security_log_settings
    ADD CONSTRAINT privacy_security_log_settings_pkey PRIMARY KEY (id);

--
-- Name: privileged_access_log privileged_access_log_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.privileged_access_log
    ADD CONSTRAINT privileged_access_log_pkey PRIMARY KEY (id);

--
-- Name: properties properties_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.properties
    ADD CONSTRAINT properties_pkey PRIMARY KEY (id);

--
-- Name: property_access_secrets property_access_secrets_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.property_access_secrets
    ADD CONSTRAINT property_access_secrets_pkey PRIMARY KEY (id);

--
-- Name: property_compliance_holds property_compliance_holds_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.property_compliance_holds
    ADD CONSTRAINT property_compliance_holds_pkey PRIMARY KEY (id);

--
-- Name: property_package_items property_package_items_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.property_package_items
    ADD CONSTRAINT property_package_items_pkey PRIMARY KEY (id);

--
-- Name: property_package_versions property_package_versions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.property_package_versions
    ADD CONSTRAINT property_package_versions_pkey PRIMARY KEY (id);

--
-- Name: property_package_versions property_package_versions_tenant_id_property_id_version_num_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.property_package_versions
    ADD CONSTRAINT property_package_versions_tenant_id_property_id_version_num_key UNIQUE (tenant_id, property_id, version_number);

--
-- Name: property_transitions property_transitions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.property_transitions
    ADD CONSTRAINT property_transitions_pkey PRIMARY KEY (id);

--
-- Name: purchase_order_items purchase_order_items_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_order_items
    ADD CONSTRAINT purchase_order_items_pkey PRIMARY KEY (id);

--
-- Name: purchase_orders purchase_orders_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_orders
    ADD CONSTRAINT purchase_orders_pkey PRIMARY KEY (id);

--
-- Name: queued_offline_evidence queued_offline_evidence_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.queued_offline_evidence
    ADD CONSTRAINT queued_offline_evidence_pkey PRIMARY KEY (id);

--
-- Name: reconciliation_exceptions reconciliation_exceptions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reconciliation_exceptions
    ADD CONSTRAINT reconciliation_exceptions_pkey PRIMARY KEY (id);

--
-- Name: report_snapshots report_snapshots_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.report_snapshots
    ADD CONSTRAINT report_snapshots_pkey PRIMARY KEY (id);

--
-- Name: requisition_approvals requisition_approvals_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.requisition_approvals
    ADD CONSTRAINT requisition_approvals_pkey PRIMARY KEY (id);

--
-- Name: requisition_items requisition_items_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.requisition_items
    ADD CONSTRAINT requisition_items_pkey PRIMARY KEY (id);

--
-- Name: requisitions requisitions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.requisitions
    ADD CONSTRAINT requisitions_pkey PRIMARY KEY (id);

--
-- Name: reservation_conflict_resolutions reservation_conflict_resolutions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reservation_conflict_resolutions
    ADD CONSTRAINT reservation_conflict_resolutions_pkey PRIMARY KEY (id);

--
-- Name: reservation_conflicts reservation_conflicts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reservation_conflicts
    ADD CONSTRAINT reservation_conflicts_pkey PRIMARY KEY (id);

--
-- Name: reservations reservations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reservations
    ADD CONSTRAINT reservations_pkey PRIMARY KEY (id);

--
-- Name: route_plans route_plans_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.route_plans
    ADD CONSTRAINT route_plans_pkey PRIMARY KEY (id);

--
-- Name: route_stops route_stops_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.route_stops
    ADD CONSTRAINT route_stops_pkey PRIMARY KEY (id);

--
-- Name: service_contract_versions service_contract_versions_agreement_id_version_number_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.service_contract_versions
    ADD CONSTRAINT service_contract_versions_agreement_id_version_number_key UNIQUE (agreement_id, version_number);

--
-- Name: service_contract_versions service_contract_versions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.service_contract_versions
    ADD CONSTRAINT service_contract_versions_pkey PRIMARY KEY (id);

--
-- Name: service_contracts service_contracts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.service_contracts
    ADD CONSTRAINT service_contracts_pkey PRIMARY KEY (id);

--
-- Name: service_recoveries service_recoveries_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.service_recoveries
    ADD CONSTRAINT service_recoveries_pkey PRIMARY KEY (id);

--
-- Name: session_revocations session_revocations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.session_revocations
    ADD CONSTRAINT session_revocations_pkey PRIMARY KEY (session_id);

--
-- Name: sessions sessions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sessions
    ADD CONSTRAINT sessions_pkey PRIMARY KEY (id);

--
-- Name: sos_events sos_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sos_events
    ADD CONSTRAINT sos_events_pkey PRIMARY KEY (id);

--
-- Name: stock_locations stock_locations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_locations
    ADD CONSTRAINT stock_locations_pkey PRIMARY KEY (id);

--
-- Name: submission_packets submission_packets_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.submission_packets
    ADD CONSTRAINT submission_packets_pkey PRIMARY KEY (id);

--
-- Name: submission_receipts submission_receipts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.submission_receipts
    ADD CONSTRAINT submission_receipts_pkey PRIMARY KEY (id);

--
-- Name: superhost_account_tasks superhost_account_tasks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.superhost_account_tasks
    ADD CONSTRAINT superhost_account_tasks_pkey PRIMARY KEY (id);

--
-- Name: superhost_threads superhost_threads_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.superhost_threads
    ADD CONSTRAINT superhost_threads_pkey PRIMARY KEY (thread_id);

--
-- Name: supplier_items supplier_items_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.supplier_items
    ADD CONSTRAINT supplier_items_pkey PRIMARY KEY (id);

--
-- Name: supplier_rebates supplier_rebates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.supplier_rebates
    ADD CONSTRAINT supplier_rebates_pkey PRIMARY KEY (id);

--
-- Name: suppliers suppliers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.suppliers
    ADD CONSTRAINT suppliers_pkey PRIMARY KEY (id);

--
-- Name: support_access_grants support_access_grants_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.support_access_grants
    ADD CONSTRAINT support_access_grants_pkey PRIMARY KEY (id);

--
-- Name: tenants tenants_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tenants
    ADD CONSTRAINT tenants_pkey PRIMARY KEY (id);

--
-- Name: ticket_assignments ticket_assignments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ticket_assignments
    ADD CONSTRAINT ticket_assignments_pkey PRIMARY KEY (id);

--
-- Name: ticket_checklist_items ticket_checklist_items_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ticket_checklist_items
    ADD CONSTRAINT ticket_checklist_items_pkey PRIMARY KEY (id);

--
-- Name: ticket_evidence ticket_evidence_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ticket_evidence
    ADD CONSTRAINT ticket_evidence_pkey PRIMARY KEY (id);

--
-- Name: ticket_state_events ticket_state_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ticket_state_events
    ADD CONSTRAINT ticket_state_events_pkey PRIMARY KEY (id);

--
-- Name: tickets tickets_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tickets
    ADD CONSTRAINT tickets_pkey PRIMARY KEY (id);

--
-- Name: time_entries time_entries_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.time_entries
    ADD CONSTRAINT time_entries_pkey PRIMARY KEY (id);

--
-- Name: turnover_proposals turnover_proposals_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.turnover_proposals
    ADD CONSTRAINT turnover_proposals_pkey PRIMARY KEY (id);

--
-- Name: external_calendar_events uq_external_event_source; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.external_calendar_events
    ADD CONSTRAINT uq_external_event_source UNIQUE (feed_id, external_event_id);

--
-- Name: reservations uq_reservation_source; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reservations
    ADD CONSTRAINT uq_reservation_source UNIQUE (feed_id, external_event_id);

--
-- Name: turnover_proposals uq_turnover_proposal; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.turnover_proposals
    ADD CONSTRAINT uq_turnover_proposal UNIQUE (reservation_id, kind);

--
-- Name: users users_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);

--
-- Name: vendor_work_orders vendor_work_orders_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vendor_work_orders
    ADD CONSTRAINT vendor_work_orders_pkey PRIMARY KEY (id);

--
-- Name: warranty_records warranty_records_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.warranty_records
    ADD CONSTRAINT warranty_records_pkey PRIMARY KEY (id);

--
-- Name: worker_certifications worker_certifications_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.worker_certifications
    ADD CONSTRAINT worker_certifications_pkey PRIMARY KEY (id);

--
-- Name: worker_ratings worker_ratings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.worker_ratings
    ADD CONSTRAINT worker_ratings_pkey PRIMARY KEY (id);

--
-- Name: workers workers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.workers
    ADD CONSTRAINT workers_pkey PRIMARY KEY (id);

--
-- Name: workforce_assignments workforce_assignments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.workforce_assignments
    ADD CONSTRAINT workforce_assignments_pkey PRIMARY KEY (id);

--
-- Name: idx_aadhaar_preferences_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_aadhaar_preferences_tenant ON public.privacy_aadhaar_preferences USING btree (tenant_id);

--
-- Name: idx_access_custody_events_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_access_custody_events_tenant ON public.access_custody_events USING btree (tenant_id, property_id, created_at);

--
-- Name: idx_access_disclosures_grant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_access_disclosures_grant ON public.access_disclosures USING btree (grant_id, tenant_id);

--
-- Name: idx_access_disclosures_time; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_access_disclosures_time ON public.access_disclosures USING btree (tenant_id, disclosed_at);

--
-- Name: idx_access_grants_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_access_grants_tenant ON public.access_grants USING btree (tenant_id, property_id, grantee_id, status);

--
-- Name: idx_access_grants_window; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_access_grants_window ON public.access_grants USING btree (window_start, window_end);

--
-- Name: idx_access_holds_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_access_holds_tenant ON public.access_holds USING btree (tenant_id, property_id, status);

--
-- Name: idx_accounting_exports_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_accounting_exports_tenant ON public.accounting_exports USING btree (tenant_id, status);

--
-- Name: idx_adverse_action_reviews_worker; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_adverse_action_reviews_worker ON public.adverse_action_reviews USING btree (tenant_id, worker_id, decided_at);

--
-- Name: idx_agent_run_events_run; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agent_run_events_run ON public.agent_run_events USING btree (run_id, occurred_at);

--
-- Name: idx_agent_runs_idempotency; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_agent_runs_idempotency ON public.agent_runs USING btree (run_kind, idempotency_key) WHERE ((idempotency_key IS NOT NULL) AND (state <> ALL (ARRAY['cancelled'::text, 'failed'::text])));

--
-- Name: idx_agent_runs_kind_state; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agent_runs_kind_state ON public.agent_runs USING btree (run_kind, state);

--
-- Name: idx_agent_runs_lease_expires; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agent_runs_lease_expires ON public.agent_runs USING btree (lease_expires_at) WHERE ((state = ANY (ARRAY['leased'::text, 'running'::text, 'waiting_for_tool'::text, 'waiting_for_approval'::text])) AND (lease_expires_at IS NOT NULL));

--
-- Name: idx_agent_runs_state_claim; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agent_runs_state_claim ON public.agent_runs USING btree (state, created_at) WHERE (state = 'queued'::text);

--
-- Name: idx_ai_tool_calls_idempotency; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_tool_calls_idempotency ON public.ai_tool_calls USING btree (tool_name, idempotency_key, state) WHERE ((idempotency_key <> ''::text) AND (state <> ALL (ARRAY['cancelled'::text, 'failed'::text])));

--
-- Name: idx_ai_tool_calls_run; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_tool_calls_run ON public.ai_tool_calls USING btree (run_id);

--
-- Name: idx_approval_requests_run; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_approval_requests_run ON public.approval_requests USING btree (run_id);

--
-- Name: idx_audit_events_actor; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_audit_events_actor ON public.audit_events USING btree (actor_id);

--
-- Name: idx_audit_events_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_audit_events_created_at ON public.audit_events USING btree (created_at);

--
-- Name: idx_audit_events_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_audit_events_tenant ON public.audit_events USING btree (tenant_id);

--
-- Name: idx_audit_events_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_audit_events_type ON public.audit_events USING btree (event_type);

--
-- Name: idx_auth_methods_user_expires; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_auth_methods_user_expires ON public.authentication_methods USING btree (user_id, expires_at);

--
-- Name: idx_availability_windows_worker; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_availability_windows_worker ON public.availability_windows USING btree (tenant_id, worker_id, day_of_week);

--
-- Name: idx_bank_verifications_request; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_bank_verifications_request ON public.bank_verifications USING btree (tenant_id, request_id);

--
-- Name: idx_bank_verifications_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bank_verifications_tenant ON public.bank_verifications USING btree (tenant_id, request_id);

--
-- Name: idx_calendar_exceptions_property; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_calendar_exceptions_property ON public.calendar_exceptions USING btree (tenant_id, property_id, status);

--
-- Name: idx_calendar_feeds_property; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_calendar_feeds_property ON public.calendar_feeds USING btree (tenant_id, property_id);

--
-- Name: idx_catalog_claim_evidence_item; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_catalog_claim_evidence_item ON public.catalog_claim_evidence USING btree (tenant_id, catalog_item_id);

--
-- Name: idx_catalog_items_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_catalog_items_tenant ON public.catalog_items USING btree (tenant_id, status);

--
-- Name: idx_charges_idempotency; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_charges_idempotency ON public.charges USING btree (tenant_id, idempotency_key);

--
-- Name: idx_charges_property; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_charges_property ON public.charges USING btree (tenant_id, property_id);

--
-- Name: idx_charges_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_charges_tenant ON public.charges USING btree (tenant_id, status);

--
-- Name: idx_checklist_sync_conflicts_open; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_checklist_sync_conflicts_open ON public.checklist_sync_conflicts USING btree (tenant_id, ticket_id, resolved);

--
-- Name: idx_checklist_sync_conflicts_ticket; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_checklist_sync_conflicts_ticket ON public.checklist_sync_conflicts USING btree (tenant_id, ticket_id);

--
-- Name: idx_checklist_sync_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_checklist_sync_key ON public.checklist_sync_records USING btree (sync_key);

--
-- Name: idx_checklist_sync_records_ticket; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_checklist_sync_records_ticket ON public.checklist_sync_records USING btree (tenant_id, ticket_id);

--
-- Name: idx_checklist_template_versions; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_checklist_template_versions ON public.checklist_template_versions USING btree (template_id, version);

--
-- Name: idx_communication_drafts_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_communication_drafts_tenant ON public.communication_drafts USING btree (tenant_id, recipient_id, status);

--
-- Name: idx_communication_preferences_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_communication_preferences_tenant ON public.communication_preferences USING btree (tenant_id, recipient_id, audience);

--
-- Name: idx_communication_reviews_draft; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_communication_reviews_draft ON public.communication_reviews USING btree (tenant_id, draft_id, reviewed_at);

--
-- Name: idx_compliance_items_expiry; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_compliance_items_expiry ON public.compliance_items USING btree (status, expiry_date);

--
-- Name: idx_compliance_items_property; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_compliance_items_property ON public.compliance_items USING btree (property_id);

--
-- Name: idx_compliance_items_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_compliance_items_tenant ON public.compliance_items USING btree (tenant_id);

--
-- Name: idx_consumer_acceptances_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_consumer_acceptances_tenant ON public.consumer_acceptances USING btree (tenant_id, resource_type, resource_id);

--
-- Name: idx_consumer_disclosures_resource; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_consumer_disclosures_resource ON public.consumer_disclosures USING btree (tenant_id, resource_type, resource_id);

--
-- Name: idx_consumer_disclosures_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_consumer_disclosures_tenant ON public.consumer_disclosures USING btree (tenant_id, resource_type);

--
-- Name: idx_consumer_history_exports_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_consumer_history_exports_tenant ON public.consumer_history_exports USING btree (tenant_id);

--
-- Name: idx_contract_acceptances_agreement; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_contract_acceptances_agreement ON public.contract_acceptances USING btree (agreement_id);

--
-- Name: idx_conversation_links_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_conversation_links_tenant ON public.conversation_links USING btree (tenant_id, property_id, audience);

--
-- Name: idx_credits_idempotency; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_credits_idempotency ON public.credits USING btree (tenant_id, idempotency_key);

--
-- Name: idx_credits_property; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_credits_property ON public.credits USING btree (tenant_id, property_id);

--
-- Name: idx_credits_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_credits_tenant ON public.credits USING btree (tenant_id, status);

--
-- Name: idx_deliveries_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_deliveries_tenant ON public.deliveries USING btree (tenant_id, recipient_id, created_at);

--
-- Name: idx_dispatch_overrides_ticket; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_dispatch_overrides_ticket ON public.dispatch_overrides USING btree (tenant_id, ticket_id);

--
-- Name: idx_dispatch_overrides_worker; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_dispatch_overrides_worker ON public.dispatch_overrides USING btree (tenant_id, worker_id);

--
-- Name: idx_document_extractions_confidence; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_document_extractions_confidence ON public.document_extractions USING btree (tenant_id, field_name, confidence);

--
-- Name: idx_document_extractions_version; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_document_extractions_version ON public.document_extractions USING btree (document_version_id, tenant_id);

--
-- Name: idx_document_reviews_doc; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_document_reviews_doc ON public.document_reviews USING btree (document_id, tenant_id, status);

--
-- Name: idx_document_reviews_reviewer; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_document_reviews_reviewer ON public.document_reviews USING btree (reviewer_id, status);

--
-- Name: idx_document_versions_doc; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_document_versions_doc ON public.document_versions USING btree (document_id, tenant_id, version_number);

--
-- Name: idx_documents_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_documents_status ON public.documents USING btree (tenant_id, status);

--
-- Name: idx_documents_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_documents_tenant ON public.documents USING btree (tenant_id, property_id, document_type);

--
-- Name: idx_employment_terms_worker; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_employment_terms_worker ON public.employment_terms USING btree (tenant_id, worker_id, effective_date);

--
-- Name: idx_eval_exports_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_eval_exports_tenant ON public.privacy_evaluation_exports USING btree (tenant_id);

--
-- Name: idx_expenses_worker; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_expenses_worker ON public.expenses USING btree (tenant_id, worker_id, recorded_at);

--
-- Name: idx_external_events_feed; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_external_events_feed ON public.external_calendar_events USING btree (feed_id);

--
-- Name: idx_external_events_property; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_external_events_property ON public.external_calendar_events USING btree (tenant_id, property_id);

--
-- Name: idx_fee_rules_tier; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fee_rules_tier ON public.fee_rules USING btree (service_tier, currency, rule_version);

--
-- Name: idx_file_grants_expires; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_file_grants_expires ON public.file_grants USING btree (expires_at);

--
-- Name: idx_file_grants_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_file_grants_tenant ON public.file_grants USING btree (tenant_id);

--
-- Name: idx_file_grants_token; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_file_grants_token ON public.file_grants USING btree (grant_token);

--
-- Name: idx_file_objects_object_key; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_file_objects_object_key ON public.file_objects USING btree (object_key);

--
-- Name: idx_file_objects_scan_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_file_objects_scan_status ON public.file_objects USING btree (scan_status);

--
-- Name: idx_file_objects_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_file_objects_tenant ON public.file_objects USING btree (tenant_id);

--
-- Name: idx_financial_approvals_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_financial_approvals_tenant ON public.financial_approvals USING btree (tenant_id, request_id);

--
-- Name: idx_fleet_assets_custodian; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fleet_assets_custodian ON public.fleet_assets USING btree (tenant_id, assigned_custodian_id);

--
-- Name: idx_fleet_assets_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fleet_assets_tenant ON public.fleet_assets USING btree (tenant_id, status);

--
-- Name: idx_fleet_batteries_asset; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fleet_batteries_asset ON public.fleet_batteries USING btree (tenant_id, asset_id);

--
-- Name: idx_fleet_custody_events_asset; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fleet_custody_events_asset ON public.fleet_custody_events USING btree (tenant_id, asset_id, created_at);

--
-- Name: idx_fleet_custody_events_worker; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fleet_custody_events_worker ON public.fleet_custody_events USING btree (tenant_id, to_worker_id, created_at);

--
-- Name: idx_fleet_incidents_asset; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fleet_incidents_asset ON public.fleet_incidents USING btree (tenant_id, asset_id, status);

--
-- Name: idx_fleet_inspections_asset; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fleet_inspections_asset ON public.fleet_inspections USING btree (tenant_id, asset_id, created_at);

--
-- Name: idx_fleet_maintenance_asset; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fleet_maintenance_asset ON public.fleet_maintenance USING btree (tenant_id, asset_id, status);

--
-- Name: idx_fleet_maintenance_overdue; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fleet_maintenance_overdue ON public.fleet_maintenance USING btree (tenant_id, status, due_at);

--
-- Name: idx_fleet_tracking_events_asset; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fleet_tracking_events_asset ON public.fleet_tracking_events USING btree (tenant_id, asset_id, captured_at);

--
-- Name: idx_fleet_tracking_events_worker; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_fleet_tracking_events_worker ON public.fleet_tracking_events USING btree (tenant_id, worker_id, captured_at);

--
-- Name: idx_goods_receipt_items_gr; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_goods_receipt_items_gr ON public.goods_receipt_items USING btree (tenant_id, goods_receipt_id);

--
-- Name: idx_goods_receipts_po; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_goods_receipts_po ON public.goods_receipts USING btree (tenant_id, purchase_order_id);

--
-- Name: idx_grievances_worker; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_grievances_worker ON public.grievances USING btree (tenant_id, worker_id, status);

--
-- Name: idx_hermes_deliveries_draft; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_hermes_deliveries_draft ON public.hermes_deliveries USING btree (tenant_id, draft_id);

--
-- Name: idx_hermes_deliveries_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_hermes_deliveries_key ON public.hermes_deliveries USING btree (tenant_id, idempotency_key) WHERE (idempotency_key <> ''::text);

--
-- Name: idx_hermes_drafts_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_hermes_drafts_tenant ON public.hermes_drafts USING btree (tenant_id, state);

--
-- Name: idx_hermes_reviews_draft; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_hermes_reviews_draft ON public.hermes_reviews USING btree (tenant_id, draft_id);

--
-- Name: idx_identity_alternatives_actor; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_identity_alternatives_actor ON public.privacy_identity_alternatives USING btree (actor_id);

--
-- Name: idx_identity_alternatives_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_identity_alternatives_tenant ON public.privacy_identity_alternatives USING btree (tenant_id);

--
-- Name: idx_incident_alerts_queue; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_incident_alerts_queue ON public.incident_alerts USING btree (tenant_id, property_id, status, created_at);

--
-- Name: idx_incident_alerts_ticket; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_incident_alerts_ticket ON public.incident_alerts USING btree (tenant_id, ticket_id);

--
-- Name: idx_inventory_count_lines_count; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_inventory_count_lines_count ON public.inventory_count_lines USING btree (tenant_id, count_id);

--
-- Name: idx_inventory_counts_location; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_inventory_counts_location ON public.inventory_counts USING btree (tenant_id, location_id);

--
-- Name: idx_inventory_movements_balance; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_inventory_movements_balance ON public.inventory_movements USING btree (tenant_id, location_id, catalog_item_id, created_at);

--
-- Name: idx_inventory_movements_location; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_inventory_movements_location ON public.inventory_movements USING btree (tenant_id, location_id, created_at);

--
-- Name: idx_invoice_lines_invoice; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_invoice_lines_invoice ON public.invoice_lines USING btree (tenant_id, invoice_id);

--
-- Name: idx_invoices_idempotency; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_invoices_idempotency ON public.invoices USING btree (tenant_id, idempotency_key);

--
-- Name: idx_invoices_property; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_invoices_property ON public.invoices USING btree (tenant_id, property_id);

--
-- Name: idx_invoices_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_invoices_tenant ON public.invoices USING btree (tenant_id, status);

--
-- Name: idx_maintenance_approvals_estimate; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_maintenance_approvals_estimate ON public.maintenance_approvals USING btree (tenant_id, estimate_id);

--
-- Name: idx_maintenance_estimates_request; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_maintenance_estimates_request ON public.maintenance_estimates USING btree (tenant_id, request_id);

--
-- Name: idx_maintenance_requests_property; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_maintenance_requests_property ON public.maintenance_requests USING btree (tenant_id, property_id);

--
-- Name: idx_maintenance_requests_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_maintenance_requests_tenant ON public.maintenance_requests USING btree (tenant_id, status);

--
-- Name: idx_maker_checker_requests_idempotency; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_maker_checker_requests_idempotency ON public.maker_checker_requests USING btree (tenant_id, idempotency_key) WHERE (idempotency_key <> ''::text);

--
-- Name: idx_maker_checker_requests_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_maker_checker_requests_tenant ON public.maker_checker_requests USING btree (tenant_id, status);

--
-- Name: idx_maker_checker_requests_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_maker_checker_requests_type ON public.maker_checker_requests USING btree (tenant_id, request_type);

--
-- Name: idx_memberships_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_memberships_tenant ON public.memberships USING btree (tenant_id);

--
-- Name: idx_memberships_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_memberships_user ON public.memberships USING btree (user_id);

--
-- Name: idx_message_template_versions_template; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_message_template_versions_template ON public.message_template_versions USING btree (template_id, version);

--
-- Name: idx_message_templates_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_message_templates_tenant ON public.message_templates USING btree (tenant_id, audience, status);

--
-- Name: idx_metric_observations_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_metric_observations_tenant ON public.metric_observations USING btree (tenant_id, property_id, worker_id, recorded_at);

--
-- Name: idx_mfa_methods_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mfa_methods_user ON public.mfa_methods USING btree (user_id);

--
-- Name: idx_onboarding_cases_property; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_onboarding_cases_property ON public.onboarding_cases USING btree (property_id);

--
-- Name: idx_onboarding_cases_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_onboarding_cases_tenant ON public.onboarding_cases USING btree (tenant_id, status);

--
-- Name: idx_onboarding_evidence_case; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_onboarding_evidence_case ON public.onboarding_evidence USING btree (case_id);

--
-- Name: idx_onboarding_inspections_case; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_onboarding_inspections_case ON public.onboarding_inspections USING btree (case_id);

--
-- Name: idx_owner_authority_grants_actor; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_owner_authority_grants_actor ON public.owner_authority_grants USING btree (actor_id);

--
-- Name: idx_package_items_version; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_package_items_version ON public.property_package_items USING btree (tenant_id, package_version_id, order_index);

--
-- Name: idx_package_templates_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_package_templates_tenant ON public.package_templates USING btree (tenant_id, status);

--
-- Name: idx_package_versions_property; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_package_versions_property ON public.property_package_versions USING btree (tenant_id, property_id, status);

--
-- Name: idx_package_versions_property_number; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_package_versions_property_number ON public.property_package_versions USING btree (tenant_id, property_id, version_number);

--
-- Name: idx_policy_decisions_run; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_policy_decisions_run ON public.policy_decisions USING btree (run_id);

--
-- Name: idx_privacy_consents_purpose; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_privacy_consents_purpose ON public.privacy_consents USING btree (purpose_id);

--
-- Name: idx_privacy_consents_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_privacy_consents_tenant ON public.privacy_consents USING btree (tenant_id);

--
-- Name: idx_privacy_notices_purpose; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_privacy_notices_purpose ON public.privacy_notices USING btree (purpose_id);

--
-- Name: idx_privacy_notices_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_privacy_notices_tenant ON public.privacy_notices USING btree (tenant_id);

--
-- Name: idx_privacy_purposes_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_privacy_purposes_tenant ON public.privacy_purposes USING btree (tenant_id);

--
-- Name: idx_privacy_rights_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_privacy_rights_status ON public.privacy_rights_requests USING btree (status);

--
-- Name: idx_privacy_rights_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_privacy_rights_tenant ON public.privacy_rights_requests USING btree (tenant_id);

--
-- Name: idx_privileged_access_actor; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_privileged_access_actor ON public.privileged_access_log USING btree (actor_id);

--
-- Name: idx_privileged_access_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_privileged_access_tenant ON public.privileged_access_log USING btree (tenant_id);

--
-- Name: idx_processor_contracts_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_processor_contracts_tenant ON public.privacy_processor_contracts USING btree (tenant_id);

--
-- Name: idx_properties_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_properties_tenant ON public.properties USING btree (tenant_id, state);

--
-- Name: idx_property_access_secrets_property; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_property_access_secrets_property ON public.property_access_secrets USING btree (tenant_id, property_id, secret_type);

--
-- Name: idx_property_holds_property; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_property_holds_property ON public.property_compliance_holds USING btree (property_id);

--
-- Name: idx_property_holds_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_property_holds_tenant ON public.property_compliance_holds USING btree (tenant_id);

--
-- Name: idx_property_transitions_property; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_property_transitions_property ON public.property_transitions USING btree (property_id, created_at);

--
-- Name: idx_purchase_order_items_po; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_purchase_order_items_po ON public.purchase_order_items USING btree (tenant_id, purchase_order_id);

--
-- Name: idx_purchase_orders_requisition; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_purchase_orders_requisition ON public.purchase_orders USING btree (tenant_id, requisition_id);

--
-- Name: idx_purchase_orders_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_purchase_orders_tenant ON public.purchase_orders USING btree (tenant_id, status);

--
-- Name: idx_queued_offline_evidence_hash; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_queued_offline_evidence_hash ON public.queued_offline_evidence USING btree (tenant_id, content_hash);

--
-- Name: idx_queued_offline_evidence_ticket; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_queued_offline_evidence_ticket ON public.queued_offline_evidence USING btree (tenant_id, ticket_id, status);

--
-- Name: idx_reconciliation_exceptions_property; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_reconciliation_exceptions_property ON public.reconciliation_exceptions USING btree (tenant_id, property_id);

--
-- Name: idx_reconciliation_exceptions_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_reconciliation_exceptions_tenant ON public.reconciliation_exceptions USING btree (tenant_id, status);

--
-- Name: idx_renewal_warnings_item; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_renewal_warnings_item ON public.compliance_renewal_warnings USING btree (item_id);

--
-- Name: idx_renewal_warnings_property; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_renewal_warnings_property ON public.compliance_renewal_warnings USING btree (property_id);

--
-- Name: idx_report_snapshots_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_report_snapshots_key ON public.report_snapshots USING btree (tenant_id, kind, property_id, COALESCE(period_start, '-infinity'::timestamp with time zone), COALESCE(period_end, 'infinity'::timestamp with time zone));

--
-- Name: idx_report_snapshots_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_report_snapshots_tenant ON public.report_snapshots USING btree (tenant_id, kind, property_id);

--
-- Name: idx_requisition_approvals_req; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_requisition_approvals_req ON public.requisition_approvals USING btree (tenant_id, requisition_id);

--
-- Name: idx_requisition_items_req; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_requisition_items_req ON public.requisition_items USING btree (tenant_id, requisition_id);

--
-- Name: idx_requisitions_property; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_requisitions_property ON public.requisitions USING btree (tenant_id, property_id);

--
-- Name: idx_requisitions_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_requisitions_tenant ON public.requisitions USING btree (tenant_id, status);

--
-- Name: idx_reservation_conflict_resolutions_conflict; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_reservation_conflict_resolutions_conflict ON public.reservation_conflict_resolutions USING btree (conflict_id);

--
-- Name: idx_reservation_conflicts_property; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_reservation_conflicts_property ON public.reservation_conflicts USING btree (tenant_id, property_id, status);

--
-- Name: idx_reservations_property; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_reservations_property ON public.reservations USING btree (tenant_id, property_id);

--
-- Name: idx_retention_records_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_retention_records_tenant ON public.privacy_retention_records USING btree (tenant_id);

--
-- Name: idx_route_plans_worker; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_route_plans_worker ON public.route_plans USING btree (tenant_id, worker_id, planned_date);

--
-- Name: idx_route_stops_plan; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_route_stops_plan ON public.route_stops USING btree (route_plan_id, sequence);

--
-- Name: idx_service_contract_versions_agreement; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_service_contract_versions_agreement ON public.service_contract_versions USING btree (agreement_id);

--
-- Name: idx_service_contracts_property; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_service_contracts_property ON public.service_contracts USING btree (property_id);

--
-- Name: idx_service_contracts_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_service_contracts_tenant ON public.service_contracts USING btree (tenant_id, status);

--
-- Name: idx_service_recoveries_incident; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_service_recoveries_incident ON public.service_recoveries USING btree (tenant_id, incident_ticket_id);

--
-- Name: idx_sessions_expires; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sessions_expires ON public.sessions USING btree (expires_at);

--
-- Name: idx_sos_events_worker; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sos_events_worker ON public.sos_events USING btree (tenant_id, worker_id, triggered_at);

--
-- Name: idx_stock_locations_property; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_stock_locations_property ON public.stock_locations USING btree (tenant_id, property_id);

--
-- Name: idx_stock_locations_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_stock_locations_tenant ON public.stock_locations USING btree (tenant_id, location_type);

--
-- Name: idx_subledger_reference; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_subledger_reference ON public.operational_subledger_entries USING btree (tenant_id, reference_type, reference_id);

--
-- Name: idx_subledger_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_subledger_tenant ON public.operational_subledger_entries USING btree (tenant_id, property_id);

--
-- Name: idx_submission_packets_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_submission_packets_tenant ON public.submission_packets USING btree (tenant_id, property_id, status);

--
-- Name: idx_submission_receipts_packet; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_submission_receipts_packet ON public.submission_receipts USING btree (packet_id, tenant_id);

--
-- Name: idx_superhost_account_tasks_actor; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_superhost_account_tasks_actor ON public.superhost_account_tasks USING btree (tenant_id, actor_id, status, created_at DESC);

--
-- Name: idx_superhost_threads_idempotency_v2; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_superhost_threads_idempotency_v2 ON public.superhost_threads USING btree (tenant_id, actor_id, idempotency_key) WHERE (idempotency_key <> ''::text);

--
-- Name: idx_supplier_items_catalog; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_supplier_items_catalog ON public.supplier_items USING btree (tenant_id, catalog_item_id);

--
-- Name: idx_supplier_items_supplier; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_supplier_items_supplier ON public.supplier_items USING btree (tenant_id, supplier_id);

--
-- Name: idx_supplier_rebates_po; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_supplier_rebates_po ON public.supplier_rebates USING btree (tenant_id, purchase_order_id);

--
-- Name: idx_supplier_rebates_supplier; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_supplier_rebates_supplier ON public.supplier_rebates USING btree (tenant_id, supplier_id);

--
-- Name: idx_suppliers_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_suppliers_tenant ON public.suppliers USING btree (tenant_id, status);

--
-- Name: idx_support_access_grants_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_support_access_grants_tenant ON public.support_access_grants USING btree (tenant_id, granted_to_user_id);

--
-- Name: idx_ticket_assignments_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ticket_assignments_status ON public.ticket_assignments USING btree (tenant_id, status);

--
-- Name: idx_ticket_assignments_ticket; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ticket_assignments_ticket ON public.ticket_assignments USING btree (tenant_id, ticket_id);

--
-- Name: idx_ticket_assignments_worker; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ticket_assignments_worker ON public.ticket_assignments USING btree (tenant_id, worker_id);

--
-- Name: idx_ticket_checklist_items_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ticket_checklist_items_status ON public.ticket_checklist_items USING btree (tenant_id, status);

--
-- Name: idx_ticket_checklist_items_ticket; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ticket_checklist_items_ticket ON public.ticket_checklist_items USING btree (tenant_id, ticket_id, template_item_index);

--
-- Name: idx_ticket_evidence_hash; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ticket_evidence_hash ON public.ticket_evidence USING btree (tenant_id, ticket_id, content_hash);

--
-- Name: idx_ticket_evidence_ticket; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ticket_evidence_ticket ON public.ticket_evidence USING btree (tenant_id, ticket_id, captured_at);

--
-- Name: idx_ticket_state_events_ticket; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ticket_state_events_ticket ON public.ticket_state_events USING btree (tenant_id, ticket_id, created_at);

--
-- Name: idx_tickets_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tickets_status ON public.tickets USING btree (tenant_id, status);

--
-- Name: idx_tickets_tenant_property; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tickets_tenant_property ON public.tickets USING btree (tenant_id, property_id);

--
-- Name: idx_tickets_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tickets_type ON public.tickets USING btree (tenant_id, type);

--
-- Name: idx_time_entries_worker; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_time_entries_worker ON public.time_entries USING btree (tenant_id, worker_id, recorded_at);

--
-- Name: idx_turnover_proposals_property; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_turnover_proposals_property ON public.turnover_proposals USING btree (tenant_id, property_id, status);

--
-- Name: idx_users_tenant_contact; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_users_tenant_contact ON public.users USING btree (tenant_id, contact);

--
-- Name: idx_vendor_work_orders_request; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_vendor_work_orders_request ON public.vendor_work_orders USING btree (tenant_id, request_id);

--
-- Name: idx_vendor_work_orders_vendor; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_vendor_work_orders_vendor ON public.vendor_work_orders USING btree (tenant_id, vendor_id, status);

--
-- Name: idx_warranty_records_property; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_warranty_records_property ON public.warranty_records USING btree (tenant_id, property_id);

--
-- Name: idx_warranty_records_work_order; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_warranty_records_work_order ON public.warranty_records USING btree (tenant_id, work_order_id);

--
-- Name: idx_worker_certifications_worker; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_worker_certifications_worker ON public.worker_certifications USING btree (tenant_id, worker_id, work_type);

--
-- Name: idx_worker_ratings_worker; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_worker_ratings_worker ON public.worker_ratings USING btree (tenant_id, worker_id, recorded_at);

--
-- Name: idx_workers_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_workers_tenant ON public.workers USING btree (tenant_id, status);

--
-- Name: idx_workforce_assignments_worker; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_workforce_assignments_worker ON public.workforce_assignments USING btree (tenant_id, worker_id, assigned_at);

--
-- Name: uq_calendar_exceptions_open; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_calendar_exceptions_open ON public.calendar_exceptions USING btree (tenant_id, property_id, kind, dedupe_key) WHERE (status = 'open'::text);

--
-- Name: uq_reservation_conflicts_open; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_reservation_conflicts_open ON public.reservation_conflicts USING btree (tenant_id, property_id, kind, dedupe_key) WHERE (status = 'open'::text);

--
-- Name: audit_events audit_events_no_update; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER audit_events_no_update BEFORE DELETE OR UPDATE ON public.audit_events FOR EACH STATEMENT EXECUTE FUNCTION public.audit_no_update_delete();

--
-- Name: inventory_movements inventory_movements_no_update; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER inventory_movements_no_update BEFORE DELETE OR UPDATE ON public.inventory_movements FOR EACH STATEMENT EXECUTE FUNCTION public.inventory_movements_immutable();

--
-- Name: onboarding_inspections onboarding_inspections_no_update_delete; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER onboarding_inspections_no_update_delete BEFORE DELETE OR UPDATE ON public.onboarding_inspections FOR EACH ROW EXECUTE FUNCTION public.onboarding_inspection_immutable();

--
-- Name: service_contract_versions service_contract_versions_no_insert_accepted; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER service_contract_versions_no_insert_accepted BEFORE INSERT ON public.service_contract_versions FOR EACH ROW EXECUTE FUNCTION public.contracts_accepted_version_immutable();

--
-- Name: service_contract_versions service_contract_versions_no_update_delete; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER service_contract_versions_no_update_delete BEFORE DELETE OR UPDATE ON public.service_contract_versions FOR EACH ROW EXECUTE FUNCTION public.contracts_version_immutable();

--
-- Name: adverse_action_reviews adverse_action_reviews_worker_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.adverse_action_reviews
    ADD CONSTRAINT adverse_action_reviews_worker_id_fkey FOREIGN KEY (worker_id) REFERENCES public.workers(id);

--
-- Name: agent_run_events agent_run_events_run_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_run_events
    ADD CONSTRAINT agent_run_events_run_id_fkey FOREIGN KEY (run_id) REFERENCES public.agent_runs(run_id);

--
-- Name: availability_windows availability_windows_worker_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.availability_windows
    ADD CONSTRAINT availability_windows_worker_id_fkey FOREIGN KEY (worker_id) REFERENCES public.workers(id);

--
-- Name: checklist_sync_conflicts checklist_sync_conflicts_ticket_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.checklist_sync_conflicts
    ADD CONSTRAINT checklist_sync_conflicts_ticket_id_fkey FOREIGN KEY (ticket_id) REFERENCES public.tickets(id);

--
-- Name: checklist_sync_records checklist_sync_records_ticket_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.checklist_sync_records
    ADD CONSTRAINT checklist_sync_records_ticket_id_fkey FOREIGN KEY (ticket_id) REFERENCES public.tickets(id);

--
-- Name: checklist_template_versions checklist_template_versions_template_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.checklist_template_versions
    ADD CONSTRAINT checklist_template_versions_template_id_fkey FOREIGN KEY (template_id) REFERENCES public.checklist_templates(id);

--
-- Name: communication_reviews communication_reviews_draft_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.communication_reviews
    ADD CONSTRAINT communication_reviews_draft_id_fkey FOREIGN KEY (draft_id) REFERENCES public.communication_drafts(id);

--
-- Name: dispatch_overrides dispatch_overrides_ticket_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dispatch_overrides
    ADD CONSTRAINT dispatch_overrides_ticket_id_fkey FOREIGN KEY (ticket_id) REFERENCES public.tickets(id);

--
-- Name: employment_terms employment_terms_worker_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.employment_terms
    ADD CONSTRAINT employment_terms_worker_id_fkey FOREIGN KEY (worker_id) REFERENCES public.workers(id);

--
-- Name: expenses expenses_worker_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.expenses
    ADD CONSTRAINT expenses_worker_id_fkey FOREIGN KEY (worker_id) REFERENCES public.workers(id);

--
-- Name: external_calendar_events external_calendar_events_feed_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.external_calendar_events
    ADD CONSTRAINT external_calendar_events_feed_id_fkey FOREIGN KEY (feed_id) REFERENCES public.calendar_feeds(id);

--
-- Name: file_grants file_grants_object_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.file_grants
    ADD CONSTRAINT file_grants_object_id_fkey FOREIGN KEY (object_id) REFERENCES public.file_objects(id);

--
-- Name: grievances grievances_worker_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.grievances
    ADD CONSTRAINT grievances_worker_id_fkey FOREIGN KEY (worker_id) REFERENCES public.workers(id);

--
-- Name: hermes_deliveries hermes_deliveries_draft_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.hermes_deliveries
    ADD CONSTRAINT hermes_deliveries_draft_id_fkey FOREIGN KEY (draft_id) REFERENCES public.hermes_drafts(draft_id);

--
-- Name: hermes_reviews hermes_reviews_draft_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.hermes_reviews
    ADD CONSTRAINT hermes_reviews_draft_id_fkey FOREIGN KEY (draft_id) REFERENCES public.hermes_drafts(draft_id);

--
-- Name: incident_alerts incident_alerts_ticket_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.incident_alerts
    ADD CONSTRAINT incident_alerts_ticket_id_fkey FOREIGN KEY (ticket_id) REFERENCES public.tickets(id);

--
-- Name: message_template_versions message_template_versions_template_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.message_template_versions
    ADD CONSTRAINT message_template_versions_template_id_fkey FOREIGN KEY (template_id) REFERENCES public.message_templates(id);

--
-- Name: queued_offline_evidence queued_offline_evidence_ticket_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.queued_offline_evidence
    ADD CONSTRAINT queued_offline_evidence_ticket_id_fkey FOREIGN KEY (ticket_id) REFERENCES public.tickets(id);

--
-- Name: reservation_conflict_resolutions reservation_conflict_resolutions_conflict_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reservation_conflict_resolutions
    ADD CONSTRAINT reservation_conflict_resolutions_conflict_id_fkey FOREIGN KEY (conflict_id) REFERENCES public.reservation_conflicts(id);

--
-- Name: reservations reservations_feed_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reservations
    ADD CONSTRAINT reservations_feed_id_fkey FOREIGN KEY (feed_id) REFERENCES public.calendar_feeds(id);

--
-- Name: route_stops route_stops_route_plan_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.route_stops
    ADD CONSTRAINT route_stops_route_plan_id_fkey FOREIGN KEY (route_plan_id) REFERENCES public.route_plans(id);

--
-- Name: route_stops route_stops_ticket_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.route_stops
    ADD CONSTRAINT route_stops_ticket_id_fkey FOREIGN KEY (ticket_id) REFERENCES public.tickets(id);

--
-- Name: service_recoveries service_recoveries_incident_ticket_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.service_recoveries
    ADD CONSTRAINT service_recoveries_incident_ticket_id_fkey FOREIGN KEY (incident_ticket_id) REFERENCES public.tickets(id);

--
-- Name: sos_events sos_events_worker_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sos_events
    ADD CONSTRAINT sos_events_worker_id_fkey FOREIGN KEY (worker_id) REFERENCES public.workers(id);

--
-- Name: ticket_assignments ticket_assignments_ticket_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ticket_assignments
    ADD CONSTRAINT ticket_assignments_ticket_id_fkey FOREIGN KEY (ticket_id) REFERENCES public.tickets(id);

--
-- Name: ticket_checklist_items ticket_checklist_items_ticket_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ticket_checklist_items
    ADD CONSTRAINT ticket_checklist_items_ticket_id_fkey FOREIGN KEY (ticket_id) REFERENCES public.tickets(id);

--
-- Name: ticket_evidence ticket_evidence_ticket_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ticket_evidence
    ADD CONSTRAINT ticket_evidence_ticket_id_fkey FOREIGN KEY (ticket_id) REFERENCES public.tickets(id);

--
-- Name: ticket_state_events ticket_state_events_ticket_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ticket_state_events
    ADD CONSTRAINT ticket_state_events_ticket_id_fkey FOREIGN KEY (ticket_id) REFERENCES public.tickets(id);

--
-- Name: time_entries time_entries_worker_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.time_entries
    ADD CONSTRAINT time_entries_worker_id_fkey FOREIGN KEY (worker_id) REFERENCES public.workers(id);

--
-- Name: turnover_proposals turnover_proposals_reservation_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.turnover_proposals
    ADD CONSTRAINT turnover_proposals_reservation_id_fkey FOREIGN KEY (reservation_id) REFERENCES public.reservations(id);

--
-- Name: worker_certifications worker_certifications_worker_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.worker_certifications
    ADD CONSTRAINT worker_certifications_worker_id_fkey FOREIGN KEY (worker_id) REFERENCES public.workers(id);

--
-- Name: worker_ratings worker_ratings_worker_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.worker_ratings
    ADD CONSTRAINT worker_ratings_worker_id_fkey FOREIGN KEY (worker_id) REFERENCES public.workers(id);

--
-- Name: workforce_assignments workforce_assignments_worker_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.workforce_assignments
    ADD CONSTRAINT workforce_assignments_worker_id_fkey FOREIGN KEY (worker_id) REFERENCES public.workers(id);

--
-- PostgreSQL database dump complete
--
