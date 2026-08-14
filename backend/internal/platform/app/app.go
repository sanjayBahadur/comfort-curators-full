package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"comfort-curators-backend/internal/access"
	"comfort-curators-backend/internal/api"
	"comfort-curators-backend/internal/automation"
	"comfort-curators-backend/internal/automation/hermes"
	"comfort-curators-backend/internal/automation/superhost"
	"comfort-curators-backend/internal/automation/superhost/prompt"
	"comfort-curators-backend/internal/billing"
	"comfort-curators-backend/internal/catalog"
	"comfort-curators-backend/internal/communications"
	"comfort-curators-backend/internal/compliance"
	"comfort-curators-backend/internal/consumer"
	"comfort-curators-backend/internal/contracts"
	"comfort-curators-backend/internal/documents"
	"comfort-curators-backend/internal/fleet"
	"comfort-curators-backend/internal/iam"
	"comfort-curators-backend/internal/inventory"
	"comfort-curators-backend/internal/maintenance"
	"comfort-curators-backend/internal/observability"
	"comfort-curators-backend/internal/onboarding"
	"comfort-curators-backend/internal/operations"
	"comfort-curators-backend/internal/platform/audit"
	"comfort-curators-backend/internal/platform/config"
	"comfort-curators-backend/internal/platform/database"
	"comfort-curators-backend/internal/platform/durability"
	"comfort-curators-backend/internal/platform/files"
	"comfort-curators-backend/internal/platform/health"
	httpplatform "comfort-curators-backend/internal/platform/http"
	"comfort-curators-backend/internal/platform/jobs"
	"comfort-curators-backend/internal/platform/logging"
	"comfort-curators-backend/internal/platform/security"
	"comfort-curators-backend/internal/privacy"
	"comfort-curators-backend/internal/procurement"
	storeprocurement "comfort-curators-backend/internal/procurement/store"
	"comfort-curators-backend/internal/property"
	"comfort-curators-backend/internal/recovery"
	"comfort-curators-backend/internal/reporting"
	"comfort-curators-backend/internal/reservations"
	secscan "comfort-curators-backend/internal/security"
	"comfort-curators-backend/internal/workforce"
)

// schemaInitLockKey is an arbitrary, fixed advisory-lock key shared by
// every process that runs schema initialization. RunMigrations tracks
// its own applied-versions table so it tolerates concurrent callers, but
// files.Migrate/security.EnsureSchema/audit.EnsureSchema each run raw
// `CREATE TABLE/INDEX IF NOT EXISTS` DDL with no locking of their own --
// when the api and worker containers start at the same moment, both can
// pass the IF NOT EXISTS check before either commits, and the loser's
// commit then fails with a duplicate-key error on the catalog (observed
// live, on a different table/index each run: encryption_keys,
// file_grants, idx_file_objects_object_key -- same root cause every
// time). Serialize the whole schema-setup sequence behind a session-held
// Postgres advisory lock so only one process ever runs it at once.
func initializeSchema(ctx context.Context, db *database.DB) error {
	conn, err := db.Pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire schema lock connection: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", schemaInitLockKey); err != nil {
		return fmt.Errorf("acquire schema lock: %w", err)
	}
	defer func() {
		if _, err := conn.Exec(ctx, "SELECT pg_advisory_unlock($1)", schemaInitLockKey); err != nil {
			logging.Error(ctx, "failed to release schema lock", "error", err)
		}
	}()

	if err := database.RunMigrations(ctx, db); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	if err := files.Migrate(ctx, db.Pool); err != nil {
		return fmt.Errorf("files migration: %w", err)
	}
	if err := security.EnsureSchema(ctx, db.Pool); err != nil {
		return fmt.Errorf("security schema: %w", err)
	}
	if err := audit.EnsureSchema(ctx, db.Pool); err != nil {
		return fmt.Errorf("audit schema: %w", err)
	}
	if err := iam.EnsureSchema(ctx, db.Pool); err != nil {
		return fmt.Errorf("iam schema: %w", err)
	}
	if err := property.EnsureSchema(ctx, db.Pool); err != nil {
		return fmt.Errorf("property schema: %w", err)
	}
	if err := onboarding.EnsureSchema(ctx, db.Pool); err != nil {
		return fmt.Errorf("onboarding schema: %w", err)
	}
	if err := contracts.EnsureSchema(ctx, db.Pool); err != nil {
		return fmt.Errorf("contracts schema: %w", err)
	}
	if err := compliance.EnsureSchema(ctx, db.Pool); err != nil {
		return fmt.Errorf("compliance schema: %w", err)
	}
	if err := reservations.EnsureSchema(ctx, db.Pool); err != nil {
		return fmt.Errorf("reservations schema: %w", err)
	}
	if err := operations.EnsureSchema(ctx, db.Pool); err != nil {
		return fmt.Errorf("operations schema: %w", err)
	}
	if err := workforce.EnsureSchema(ctx, db.Pool); err != nil {
		return fmt.Errorf("workforce schema: %w", err)
	}
	if err := communications.EnsureSchema(ctx, db.Pool); err != nil {
		return fmt.Errorf("communications schema: %w", err)
	}
	if err := access.EnsureSchema(ctx, db.Pool); err != nil {
		return fmt.Errorf("access schema: %w", err)
	}
	if err := fleet.EnsureSchema(ctx, db.Pool); err != nil {
		return fmt.Errorf("fleet schema: %w", err)
	}
	if err := catalog.EnsureSchema(ctx, db.Pool); err != nil {
		return fmt.Errorf("catalog schema: %w", err)
	}
	if err := inventory.EnsureSchema(ctx, db.Pool); err != nil {
		return fmt.Errorf("inventory schema: %w", err)
	}
	if err := procurement.EnsureSchema(ctx, db.Pool); err != nil {
		return fmt.Errorf("procurement schema: %w", err)
	}
	if err := maintenance.EnsureSchema(ctx, db.Pool); err != nil {
		return fmt.Errorf("maintenance schema: %w", err)
	}
	if err := documents.EnsureSchema(ctx, db.Pool); err != nil {
		return fmt.Errorf("documents schema: %w", err)
	}
	if err := billing.EnsureSchema(ctx, db.Pool); err != nil {
		return fmt.Errorf("billing schema: %w", err)
	}
	if err := privacy.EnsureSchema(ctx, db.Pool); err != nil {
		return fmt.Errorf("privacy schema: %w", err)
	}
	if err := consumer.EnsureSchema(ctx, db.Pool); err != nil {
		return fmt.Errorf("consumer schema: %w", err)
	}
	if err := reporting.EnsureSchema(ctx, db.Pool); err != nil {
		return fmt.Errorf("reporting schema: %w", err)
	}
	if err := automation.EnsureSchema(ctx, db.Pool); err != nil {
		return fmt.Errorf("automation schema: %w", err)
	}
	if err := superhost.EnsureToolSchema(ctx, db.Pool); err != nil {
		return fmt.Errorf("superhost tool schema: %w", err)
	}
	if err := hermes.EnsureSchema(ctx, db.Pool); err != nil {
		return fmt.Errorf("hermes schema: %w", err)
	}
	return nil
}

const schemaInitLockKey = 8823610245

func RunAPI(ctx context.Context) error {
	cfg := config.LoadFromEnv()
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("configuration: %w", err)
	}

	logging.Init(cfg.LogLevel)
	logging.Info(ctx, "starting api server", "config", cfg.SafeFields())

	var healthChecks []health.Checker
	var jobStore *jobs.JobStore
	var identitySvc *iam.IdentityService
	var propSvc *property.PropertyService
	var onboardingSvc *onboarding.Service
	var contractsSvc *contracts.Service
	var complianceSvc *compliance.ComplianceService
	var calendarSvc *reservations.CalendarService
	var ticketSvc *operations.TicketService
	var dispatchSvc *operations.DispatchService
	var workforceSvc *workforce.WorkforceService
	var accessSvc *access.Service
	var communicationsSvc *communications.CommunicationsService
	var fleetSvc *fleet.Service
	var catalogSvc *catalog.Service
	var inventorySvc *inventory.Service
	var procurementSvc *procurement.Service
	var maintenanceSvc *maintenance.Service
	var documentsSvc *documents.Service
	var billingSvc *billing.Service
	var privacySvc *privacy.PrivacyService
	var consumerSvc *consumer.ConsumerService
	var reportingSvc *reporting.ReportingService
	var hermesSvc *hermes.HermesService
	var agentRunStore *automation.AgentRunStore
	var superhostPool *pgxpool.Pool
	if !cfg.DBSkip {
		db, err := database.Connect(ctx, cfg)
		if err != nil {
			return fmt.Errorf("database: %w", err)
		}
		defer db.Close()

		if err := initializeSchema(ctx, db); err != nil {
			return err
		}

		idemStore := durability.NewIdempotencyStore(db.Pool)
		outboxStore := durability.NewOutboxStore(db.Pool)
		jobStore = jobs.NewJobStore(db.Pool)
		logging.Info(ctx, "durability services initialized",
			"idempotency_store", fmt.Sprintf("%p", idemStore),
			"outbox_store", fmt.Sprintf("%p", outboxStore),
			"job_store", fmt.Sprintf("%p", jobStore),
		)

		fileStore := files.NewFileStore(db.Pool, files.DefaultConfig())
		logging.Info(ctx, "file store initialized",
			"file_store", fmt.Sprintf("%p", fileStore),
		)

		auditStore := audit.NewAuditStore(db.Pool)
		logging.Info(ctx, "audit store initialized",
			"audit_store", fmt.Sprintf("%p", auditStore),
		)

		identitySvc = iam.NewIdentityService(db.Pool, auditStore)
		logging.Info(ctx, "identity service initialized",
			"identity_svc", fmt.Sprintf("%p", identitySvc),
		)

		propSvc = property.NewPropertyService(db.Pool, auditStore).WithAuthorizer(
			identitySvc.GetTenancyService(),
		)
		logging.Info(ctx, "property service initialized",
			"property_svc", fmt.Sprintf("%p", propSvc),
		)

		onboardingSvc = onboarding.NewService(db.Pool, auditStore).WithAuthorizer(
			identitySvc.GetTenancyService(),
		)
		logging.Info(ctx, "onboarding service initialized",
			"onboarding_svc", fmt.Sprintf("%p", onboardingSvc),
		)

		contractsSvc = contracts.NewService(db.Pool, auditStore).WithAuthorizer(
			identitySvc.GetTenancyService(),
		)
		logging.Info(ctx, "contracts service initialized",
			"contracts_svc", fmt.Sprintf("%p", contractsSvc),
		)

		complianceSvc = compliance.NewComplianceService(db.Pool, auditStore).WithAuthorizer(
			identitySvc.GetTenancyService(),
		)
		logging.Info(ctx, "compliance service initialized",
			"compliance_svc", fmt.Sprintf("%p", complianceSvc),
		)

		calendarSvc = reservations.NewCalendarService(db.Pool).WithAuthorizer(
			identitySvc.GetTenancyService(),
		)
		logging.Info(ctx, "calendar service initialized",
			"calendar_svc", fmt.Sprintf("%p", calendarSvc),
		)

		ticketSvc = operations.NewTicketService(db.Pool).WithAuthorizer(
			identitySvc.GetTenancyService(),
		).WithAudit(auditStore)
		logging.Info(ctx, "ticket service initialized",
			"ticket_svc", fmt.Sprintf("%p", ticketSvc),
		)

		dispatchSvc = operations.NewDispatchService(db.Pool)
		logging.Info(ctx, "dispatch service initialized",
			"dispatch_svc", fmt.Sprintf("%p", dispatchSvc),
		)

		workforceSvc = workforce.NewWorkforceService(db.Pool).WithAudit(auditStore)
		logging.Info(ctx, "workforce service initialized",
			"workforce_svc", fmt.Sprintf("%p", workforceSvc),
		)

		accessSvc = access.NewService(db.Pool).WithAudit(auditStore)
		logging.Info(ctx, "access service initialized",
			"access_svc", fmt.Sprintf("%p", accessSvc),
		)

		communicationsSvc = communications.NewCommunicationsService(db.Pool).WithAudit(auditStore)
		logging.Info(ctx, "communications service initialized",
			"communications_svc", fmt.Sprintf("%p", communicationsSvc),
		)

		hermesStore := hermes.NewPGStore(db.Pool)
		hermesSvc = hermes.NewService(hermesStore)
		logging.Info(ctx, "hermes service initialized",
			"hermes_svc", fmt.Sprintf("%p", hermesSvc),
		)

		fleetSvc = fleet.NewService(db.Pool).WithAudit(auditStore)
		logging.Info(ctx, "fleet service initialized",
			"fleet_svc", fmt.Sprintf("%p", fleetSvc),
		)

		catalogSvc = catalog.NewService(db.Pool).WithAuthorizer(
			identitySvc.GetTenancyService(),
		).WithAudit(auditStore)
		logging.Info(ctx, "catalog service initialized",
			"catalog_svc", fmt.Sprintf("%p", catalogSvc),
		)

		inventorySvc = inventory.NewService(db.Pool).WithAudit(auditStore)
		logging.Info(ctx, "inventory service initialized",
			"inventory_svc", fmt.Sprintf("%p", inventorySvc),
		)

		procurementSvc = procurement.NewService(db.Pool).WithAudit(auditStore)
		logging.Info(ctx, "procurement service initialized",
			"procurement_svc", fmt.Sprintf("%p", procurementSvc),
		)

		maintenanceSvc = maintenance.NewService(db.Pool).WithAudit(auditStore)
		logging.Info(ctx, "maintenance service initialized",
			"maintenance_svc", fmt.Sprintf("%p", maintenanceSvc),
		)

		documentsSvc = documents.NewService(db.Pool).WithAudit(auditStore)
		logging.Info(ctx, "documents service initialized",
			"documents_svc", fmt.Sprintf("%p", documentsSvc),
		)

		billingSvc = billing.NewService(db.Pool).WithAudit(auditStore)
		logging.Info(ctx, "billing service initialized",
			"billing_svc", fmt.Sprintf("%p", billingSvc),
		)

		privacySvc = privacy.NewPrivacyService(db.Pool, auditStore)
		logging.Info(ctx, "privacy service initialized",
			"privacy_svc", fmt.Sprintf("%p", privacySvc),
		)

		consumerSvc = consumer.NewConsumerService(db.Pool, auditStore)
		logging.Info(ctx, "consumer service initialized",
			"consumer_svc", fmt.Sprintf("%p", consumerSvc),
		)

		reportingSvc = reporting.NewReportingService(db.Pool, auditStore)
		logging.Info(ctx, "reporting service initialized",
			"reporting_svc", fmt.Sprintf("%p", reportingSvc),
		)

		agentRunStore = automation.NewAgentRunStore(db.Pool)
		superhostPool = db.Pool
		logging.Info(ctx, "agent run store initialized",
			"agent_run_store", fmt.Sprintf("%p", agentRunStore),
		)

		healthChecks = append(healthChecks, health.NamedChecker("database", func() error {
			return db.Pool.Ping(ctx)
		}))
		healthChecks = append(healthChecks, recovery.MinioChecker())
		healthChecks = append(healthChecks, recovery.ModelChecker())
	}

	healthHandler := health.NewHandler(healthChecks...)

	obsMetrics := observability.NewMetrics()
	obsTracer := observability.NewTracer()
	obsAlerts := observability.NewAlertService()

	// burst=300, rate=1200/min (20/sec sustained) per (IP, path). The
	// original burst=20/rate=100-per-minute was tuned only against the
	// package's own unit tests and never against real traffic shape: the
	// acceptance suite alone calls /auth/session/create from one IP well
	// over 20 times within a few seconds (nearly every probe creates its
	// own session), so the whole gate failed closed under its own
	// legitimate load. This still meaningfully throttles a single IP
	// sustaining more than 20 req/s against one path.
	rateLimiter := secscan.NewRateLimiter(1200, 300, time.Minute)

	mux := http.NewServeMux()
	mux.HandleFunc("/health/live", healthHandler.Liveness())
	mux.HandleFunc("/health/ready", healthHandler.Readiness())
	observability.NewHandler(obsMetrics, obsTracer, obsAlerts).RegisterRoutes(mux)
	// Both disclose operational/security internals -- dead-lettered job
	// payloads and unresolved security findings -- so, like the
	// observability routes above, they are gated to staff rather than left
	// to RequireAuthByDefault's weaker "any authenticated subject" bar.
	if jobStore != nil {
		mux.Handle("GET /jobs/dead-letter", iam.RequireRole(iam.RoleStaff)(handleDeadLetter(jobStore)))
	}

	mux.Handle("GET /security/findings", iam.RequireRole(iam.RoleStaff)(handleSecurityFindings()))

	if !cfg.DBSkip {
		recoveryHandler := recovery.NewHandler(superhostPool)
		recoveryHandler.RegisterRoutes(mux)
	}

	var handler http.Handler = mux
	handler = httpplatform.RateLimit(handler, rateLimiter, httpplatform.CombinedRateLimitKey)
	handler = httpplatform.ObservabilityTracing(handler, obsTracer)
	handler = httpplatform.ObservabilityMetrics(handler, obsMetrics)
	if identitySvc != nil {
		// RequireAuthByDefault must be applied before AuthMiddleware so that
		// AuthMiddleware -- which ends up wrapping it, and therefore runs
		// first -- has already populated the subject into context by the
		// time RequireAuthByDefault checks for one.
		handler = iam.RequireAuthByDefault(handler)
		handler = iam.AuthMiddleware(identitySvc.GetSessionStore())(handler)
		iam.RegisterAuthRoutes(mux, identitySvc)
		registerAcceptanceFixtures(mux, identitySvc)
		iam.RegisterTenancyRoutes(mux, identitySvc.GetTenancyService())
		var authorityResolver api.OwnerAuthorities
		if propSvc != nil {
			authorityResolver = newAuthorityResolver(propSvc)
			api.NewPropertySliceHandler(propSvc, authorityResolver).RegisterRoutes(mux)
			if onboardingSvc != nil {
				api.NewOnboardingSliceHandler(onboardingSvc, propSvc, authorityResolver).RegisterRoutes(mux)
				onboarding.NewOnboardingHandler(onboardingSvc).RegisterRoutes(mux)
			}
			if contractsSvc != nil {
				api.NewContractSliceHandler(contractsSvc, propSvc, authorityResolver).RegisterRoutes(mux)
				contracts.NewHandler(contractsSvc).RegisterRoutes(mux)
			}
		}
		if complianceSvc != nil {
			compliance.NewComplianceHandler(complianceSvc).RegisterRoutes(mux)
		}
		if calendarSvc != nil {
			reservations.NewCalendarHandler(calendarSvc).RegisterRoutes(mux)
		}
		if ticketSvc != nil {
			operations.NewTicketHandler(ticketSvc).RegisterRoutes(mux)
		}
		if dispatchSvc != nil {
			operations.NewDispatchHandler(dispatchSvc).RegisterRoutes(mux)
		}
		if workforceSvc != nil {
			workforce.NewWorkforceHandler(workforceSvc).RegisterRoutes(mux)
		}
		if accessSvc != nil {
			access.NewHandler(accessSvc).RegisterRoutes(mux)
		}
		if communicationsSvc != nil {
			communications.NewCommunicationsHandler(communicationsSvc).RegisterRoutes(mux)
		}
		if fleetSvc != nil {
			fleet.NewHandler(fleetSvc).RegisterRoutes(mux)
		}
		if catalogSvc != nil {
			catalog.NewHandler(catalogSvc).RegisterRoutes(mux)
		}
		if inventorySvc != nil {
			inventory.NewHandler(inventorySvc).RegisterRoutes(mux)
		}
		if procurementSvc != nil {
			procurement.NewHandler(procurementSvc).RegisterRoutes(mux)
		}
		storeprocurement.NewHandler(storeprocurement.NewMockProvider()).RegisterRoutes(mux)
		if maintenanceSvc != nil {
			maintenance.NewHandler(maintenanceSvc).RegisterRoutes(mux)
		}
		if billingSvc != nil && documentsSvc != nil {
			api.NewFinanceSliceHandler(billingSvc, documentsSvc, propSvc, authorityResolver).RegisterRoutes(mux)
		}
		if privacySvc != nil {
			privacy.NewHandler(privacySvc).RegisterRoutes(mux)
		}
		if consumerSvc != nil {
			consumer.NewHandler(consumerSvc).RegisterRoutes(mux)
		}
		if reportingSvc != nil {
			reporting.NewHandler(reportingSvc).RegisterRoutes(mux)
		}
		if agentRunStore != nil {
			automation.NewAgentRunHandler(agentRunStore).RegisterRoutes(mux)
			superhostAssembler := superhost.NewContextAssembler(superhostPool)
			threadStore := superhost.NewThreadStore(superhostPool, agentRunStore, superhostAssembler)
			toolCallStore := superhost.NewToolCallStore(superhostPool)
			superhost.NewHandlerWithApprovals(agentRunStore, superhostAssembler, threadStore, toolCallStore).RegisterRoutes(mux)
		}
		if hermesSvc != nil {
			hermes.NewHandler(hermesSvc).RegisterRoutes(mux)
		}
	}

	// These three wrap everything above, including the auth gate: every
	// response -- successful, rejected, or panicked -- gets a correlation
	// id and passes through Recovery/RequestLogging. They used to be
	// applied before the auth gate, which was more inner and therefore
	// executed *after* it -- an unauthenticated rejection never reached
	// them, so its response carried no request_id at all.
	handler = httpplatform.RequestLogging(handler)
	handler = httpplatform.Recovery(handler)
	handler = httpplatform.CorrelationID(handler)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	idleConnsClosed := make(chan struct{})
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		select {
		case <-sigCh:
		case <-ctx.Done():
		}
		signal.Stop(sigCh)

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		logging.Info(ctx, "shutting down http server")
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logging.Error(shutdownCtx, "http server shutdown error", "error", err)
		}
		close(idleConnsClosed)
	}()

	logging.Info(ctx, "http server listening", "addr", srv.Addr)

	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				rateLimiter.Cleanup(10 * time.Minute)
			}
		}
	}()

	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		return fmt.Errorf("http server: %w", err)
	}

	<-idleConnsClosed
	return nil
}

func RunWorker(ctx context.Context) error {
	cfg := config.LoadFromEnv()
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("configuration: %w", err)
	}

	logging.Init(cfg.LogLevel)
	logging.Info(ctx, "starting worker", "config", cfg.SafeFields())

	obsMetrics := observability.NewMetrics()
	obsTracer := observability.NewTracer()

	registry := jobs.NewRegistry()

	if !cfg.DBSkip {
		db, err := database.Connect(ctx, cfg)
		if err != nil {
			return fmt.Errorf("database: %w", err)
		}
		defer db.Close()

		if err := initializeSchema(ctx, db); err != nil {
			return err
		}

		idemStore := durability.NewIdempotencyStore(db.Pool)
		outboxStore := durability.NewOutboxStore(db.Pool)
		jobStore := jobs.NewJobStore(db.Pool)
		logging.Info(ctx, "durability services initialized",
			"idempotency_store", fmt.Sprintf("%p", idemStore),
			"outbox_store", fmt.Sprintf("%p", outboxStore),
			"job_store", fmt.Sprintf("%p", jobStore),
		)
		outboxSink := durability.NewLogEventSink()
		logging.Info(ctx, "outbox relay initialized",
			"outbox_store", fmt.Sprintf("%p", outboxStore),
			"event_sink", fmt.Sprintf("%T", outboxSink),
		)

		fileStore := files.NewFileStore(db.Pool, files.DefaultConfig())
		logging.Info(ctx, "file store initialized",
			"file_store", fmt.Sprintf("%p", fileStore),
		)

		complianceSvc := compliance.NewComplianceService(db.Pool, nil)
		compliance.RegisterScanExpiryJob(registry, complianceSvc)
		logging.Info(ctx, "compliance scan-expiry job registered")

		calendarSvc := reservations.NewCalendarService(db.Pool)
		reservations.RegisterPollFeedJob(registry, calendarSvc)
		reservations.RegisterScanStaleFeedsJob(registry, calendarSvc)
		logging.Info(ctx, "calendar feed jobs registered")

		privacySvc := privacy.NewPrivacyService(db.Pool, nil)
		privacy.RegisterScanRetentionJob(registry, privacySvc)
		logging.Info(ctx, "privacy scan-retention job registered")

		workerID := newWorkerID()
		logging.Info(ctx, "worker starting job processing", "worker_id", workerID)

		agentRunStore := automation.NewAgentRunStore(db.Pool)
		logging.Info(ctx, "agent run store initialized",
			"agent_run_store", fmt.Sprintf("%p", agentRunStore),
		)

		modelURL := os.Getenv("CC_MODEL_URL")
		if modelURL == "" {
			modelHost := os.Getenv("CC_MODEL_HOST")
			if modelHost == "" {
				modelHost = "model-stub"
			}
			modelPort := os.Getenv("CC_MODEL_PORT")
			if modelPort == "" {
				modelPort = "8080"
			}
			modelURL = fmt.Sprintf("http://%s:%s", modelHost, modelPort)
		}

		agentProvider := automation.NewHTTPProvider(modelURL, os.Getenv("CC_MODEL_API_KEY"), superhostTools())
		agentFactory := func(kind string) automation.Provider {
			return agentProvider
		}
		logging.Info(ctx, "agent run provider initialized",
			"model_url", modelURL,
		)

		systemPrompt, err := loadSuperhostSystemPrompt()
		if err != nil {
			return fmt.Errorf("load superhost system prompt: %w", err)
		}
		logging.Info(ctx, "superhost system prompt loaded",
			"prompt_bytes", len(systemPrompt),
		)

		toolExecutor := newSuperhostToolExecutor(db.Pool)
		agentRunner := automation.NewRunnerWithToolLoop(agentRunStore, agentFactory, workerID, systemPrompt, toolExecutor)
		logging.Info(ctx, "agent run runner initialized")

		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		var wg sync.WaitGroup

		wg.Add(1)
		go runRecoveryLoop(ctx, &wg, jobStore)

		wg.Add(1)
		go runOutboxRelayLoop(ctx, &wg, outboxStore, outboxSink)

		wg.Add(1)
		go runWorkLoop(ctx, &wg, jobStore, registry, workerID, obsMetrics, obsTracer)

		wg.Add(1)
		go agentRunner.RunRecoveryLoop(ctx, &wg)

		wg.Add(1)
		go agentRunner.RunWorkLoop(ctx, &wg, nil)

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

		select {
		case <-sigCh:
			logging.Info(ctx, "worker shutting down")
		case <-ctx.Done():
			logging.Info(ctx, "worker context cancelled")
		}

		cancel()
		wg.Wait()
		logging.Info(ctx, "worker stopped")
		return nil
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-sigCh:
		logging.Info(ctx, "worker shutting down")
	case <-ctx.Done():
		logging.Info(ctx, "worker context cancelled")
	}

	return nil
}

func runOutboxRelayLoop(ctx context.Context, wg *sync.WaitGroup, store *durability.OutboxStore, sink durability.EventSink) {
	defer wg.Done()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := store.RelayOnce(ctx, sink); err != nil && !errors.Is(err, durability.ErrNoPendingEvents) {
				logging.Error(ctx, "outbox relay failed", "error", err)
			}
		}
	}
}

func runRecoveryLoop(ctx context.Context, wg *sync.WaitGroup, store *jobs.JobStore) {
	defer wg.Done()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			recovered, err := store.RecoverExpiredLeases(ctx)
			if err != nil {
				logging.Error(ctx, "failed to recover expired leases", "error", err)
			} else if recovered > 0 {
				logging.Info(ctx, "recovered expired leases", "count", recovered)
			}
		}
	}
}

func runWorkLoop(ctx context.Context, wg *sync.WaitGroup, store *jobs.JobStore, registry *jobs.Registry, workerID string, m *observability.Metrics, tr *observability.Tracer) {
	defer wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		job, err := store.Claim(ctx, workerID, jobs.DefaultLeaseDuration)
		if err != nil {
			if err != jobs.ErrNoWork {
				logging.Error(ctx, "failed to claim job", "error", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(500 * time.Millisecond):
			}
			continue
		}

		logging.Info(ctx, "claimed job",
			"job_id", job.ID,
			"job_type", job.Type,
			"attempt", job.Attempt,
		)
		m.JobClaimed(job.Type)

		if err := store.StartRunning(ctx, job.ID, workerID); err != nil {
			logging.Error(ctx, "failed to start running job", "job_id", job.ID, "error", err)
			continue
		}

		processCtx, processCancel := context.WithCancel(ctx)

		var heartbeatWg sync.WaitGroup
		if jobs.DefaultLeaseDuration > 10*time.Second {
			heartbeatWg.Add(1)
			go runHeartbeat(processCtx, &heartbeatWg, store, job.ID, workerID, jobs.DefaultLeaseDuration/2)
		}

		jobCorr := observability.FromContextOrNew(ctx).Child(observability.SourceJob)
		processCtx = observability.WithCorrelation(processCtx, jobCorr)
		jobSpan := tr.Start(jobCorr, "process.job."+job.Type)

		result, handlerErr := registry.Dispatch(processCtx, job)
		processCancel()
		heartbeatWg.Wait()

		if handlerErr != nil {
			errMsg := handlerErr.Error()
			logging.Error(ctx, "job handler failed",
				"job_id", job.ID,
				"job_type", job.Type,
				"attempt", job.Attempt,
				"error", errMsg,
			)
			m.JobFailed(job.Type)
			tr.End(jobSpan, handlerErr)
			if failErr := store.Fail(ctx, job.ID, workerID, errMsg); failErr != nil {
				logging.Error(ctx, "failed to mark job as failed", "job_id", job.ID, "error", failErr)
			}
		} else {
			logging.Info(ctx, "job completed",
				"job_id", job.ID,
				"job_type", job.Type,
			)
			m.JobCompleted(job.Type)
			tr.End(jobSpan, nil)
			if completeErr := store.Complete(ctx, job.ID, workerID, result); completeErr != nil {
				logging.Error(ctx, "failed to mark job as complete", "job_id", job.ID, "error", completeErr)
			}
		}
	}
}

func runHeartbeat(ctx context.Context, wg *sync.WaitGroup, store *jobs.JobStore, jobID, workerID string, interval time.Duration) {
	defer wg.Done()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := store.Heartbeat(ctx, jobID, workerID, jobs.DefaultLeaseDuration); err != nil {
				logging.Error(ctx, "heartbeat failed", "job_id", jobID, "error", err)
				return
			}
		}
	}
}

func newWorkerID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("worker-%d", time.Now().UnixNano())
	}
	return "worker-" + hex.EncodeToString(b[:])
}

func handleDeadLetter(store *jobs.JobStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		deadJobs, err := store.GetDeadLetterJobs(r.Context())
		if err != nil {
			logging.Error(r.Context(), "dead-letter: failed to list dead letter jobs", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "internal server error",
			})
			return
		}

		if deadJobs == nil {
			deadJobs = []jobs.Job{}
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"jobs":  deadJobs,
			"count": len(deadJobs),
		})
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func newAuthorityResolver(propSvc *property.PropertyService) api.OwnerAuthorities {
	return func(actorID string) []string {
		return propSvc.ResolveActorAuthorities(context.Background(), actorID)
	}
}

var globalFindingsStore = secscan.NewFindingStore()

func handleSecurityFindings() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		findings := globalFindingsStore.All()
		if findings == nil {
			findings = []secscan.Finding{}
		}
		unresolved := globalFindingsStore.UnresolvedHighOrCritical()
		writeJSON(w, http.StatusOK, map[string]any{
			"findings":   findings,
			"unresolved": len(unresolved),
			"total":      len(findings),
		})
	}
}

func superhostTools() []automation.ChatToolDef {
	names := superhost.AllowedToolNames()
	tools := make([]automation.ChatToolDef, 0, len(names))
	for _, name := range names {
		def, err := superhost.LookupTool(name)
		if err != nil {
			continue
		}
		tools = append(tools, automation.ChatToolDef{
			Type: "function",
			Function: automation.ChatToolFunction{
				Name:        def.Name,
				Description: def.Description,
				Parameters:  json.RawMessage(`{"type":"object"}`),
			},
		})
	}
	log.Printf("http provider: built %d tools for request; per-tool argument schemas would make tool selection more reliable and do not exist yet", len(tools))
	return tools
}

// loadSuperhostSystemPrompt returns the governed v1 system prompt via
// prompt.V1(), which go:embeds internal/automation/superhost/prompt/v1.md
// at compile time (P3.6). This was originally a relative-path os.ReadFile
// against the working directory, which only worked when the binary ran
// from the repo root — go:embed makes the prompt part of the binary
// itself, so it works regardless of the process's working directory (a
// real concern for a containerized deployment).
func loadSuperhostSystemPrompt() (string, error) {
	text := prompt.V1()
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("superhost prompt.V1() returned empty text")
	}
	return text, nil
}
