package workforce

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"comfort-curators-backend/internal/platform/audit"
	"comfort-curators-backend/internal/platform/database"
	"comfort-curators-backend/internal/platform/logging"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type WorkforceService struct {
	pool       *pgxpool.Pool
	store      *WorkforceStore
	auditStore *audit.AuditStore
}

func NewWorkforceService(pool *pgxpool.Pool) *WorkforceService {
	return &WorkforceService{
		pool:       pool,
		store:      NewWorkforceStore(pool),
		auditStore: audit.NewAuditStore(pool),
	}
}

func (s *WorkforceService) WithAudit(a *audit.AuditStore) *WorkforceService {
	s.auditStore = a
	return s
}

// CreateWorker records a tenant-scoped worker profile. Employees and genuine
// vendors stay distinct (WFM-001, WFM-002). Age eligibility is computed from
// the date of birth and blocks later operations assignment for minors.
func (s *WorkforceService) CreateWorker(ctx context.Context, params CreateWorkerParams, actorID string) (*Worker, error) {
	if params.TenantID == "" {
		return nil, fmt.Errorf("tenant_id is required")
	}
	if params.LegalName == "" {
		return nil, ErrMissingLegalName
	}
	if !IsValidClassification(params.Classification) {
		return nil, ErrInvalidClassification
	}
	if params.DateOfBirth.IsZero() || params.DateOfBirth.After(time.Now().UTC()) {
		return nil, ErrInvalidDateOfBirth
	}
	if params.InitialStatus != "" && !IsValidWorkerStatus(params.InitialStatus) {
		return nil, fmt.Errorf("invalid worker status: %s", params.InitialStatus)
	}

	status := params.InitialStatus
	if status == "" {
		status = StatusActive
	}

	w := &Worker{
		TenantID:         params.TenantID,
		LegalName:        params.LegalName,
		VerifiedIdentity: params.VerifiedIdentity,
		DateOfBirth:      params.DateOfBirth.UTC(),
		AgeEligible:      IsAgeEligible(params.DateOfBirth, time.Now().UTC()),
		ContactMethod:    params.ContactMethod,
		Classification:   params.Classification,
		Specialist:       params.Specialist,
		ServiceZone:      params.ServiceZone,
		Skills:           params.Skills,
		Status:           status,
	}

	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertWorker(ctx, tx, w); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     w.TenantID,
			ActorID:      actorID,
			Action:       "workforce.worker.created",
			ResourceType: "worker",
			ResourceID:   w.ID,
			NewState:     marshalJSON(w),
		})
	}); err != nil {
		return nil, err
	}

	return w, nil
}

func (s *WorkforceService) GetWorker(ctx context.Context, tenantID, workerID string) (*Worker, error) {
	return s.store.GetWorker(ctx, tenantID, workerID)
}

func (s *WorkforceService) ListWorkers(ctx context.Context, tenantID string) ([]Worker, error) {
	return s.store.ListWorkers(ctx, tenantID)
}

// AddCertification attaches an explicit credential for a work type. The
// certification is valid only until its expiry and does not renew itself.
func (s *WorkforceService) AddCertification(ctx context.Context, tenantID, workerID string, params CertificationParams, actorID string) (*Certification, error) {
	if params.WorkType == "" {
		return nil, fmt.Errorf("%w: work type is required", ErrInvalidCertification)
	}
	if params.Issuer == "" {
		return nil, fmt.Errorf("%w: issuer is required", ErrInvalidCertification)
	}
	if params.ExpiresAt.IsZero() || params.IssuedAt.IsZero() || !params.ExpiresAt.After(params.IssuedAt) {
		return nil, fmt.Errorf("%w: expiry must be after the issue date", ErrInvalidCertification)
	}

	w, err := s.store.GetWorker(ctx, tenantID, workerID)
	if err != nil {
		return nil, err
	}
	if IsDeactivatingStatus(w.Status) {
		return nil, fmt.Errorf("%w: worker status %s", ErrWorkerNotAssignable, w.Status)
	}

	c := &Certification{
		TenantID:  tenantID,
		WorkerID:  workerID,
		WorkType:  params.WorkType,
		Issuer:    params.Issuer,
		IssuedAt:  params.IssuedAt.UTC(),
		ExpiresAt: params.ExpiresAt.UTC(),
		Status:    CertificationStatus(params.ExpiresAt),
	}

	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertCertification(ctx, tx, c); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "workforce.worker.certified",
			ResourceType: "worker",
			ResourceID:   workerID,
			NewState:     marshalJSON(c),
		})
	}); err != nil {
		return nil, err
	}

	return c, nil
}

// CheckOperationsAssignment is the hard eligibility gate used before dispatch.
// It fails for under-18 workers and for restricted work without explicit
// certification or routing to a specialist vendor.
func (s *WorkforceService) CheckOperationsAssignment(ctx context.Context, tenantID, workerID, workType string) error {
	w, err := s.store.GetWorker(ctx, tenantID, workerID)
	if err != nil {
		return err
	}
	return checkOperationsAssignment(ctx, s.store, w, workType)
}

func checkOperationsAssignment(ctx context.Context, store *WorkforceStore, w *Worker, workType string) error {
	if workType == "" {
		workType = "general"
	}
	if !w.AgeEligible {
		return ErrUnderageForOperations
	}
	if w.Status != StatusActive {
		return fmt.Errorf("%w: worker status %s", ErrWorkerNotAssignable, w.Status)
	}

	if !IsRestrictedWorkType(workType) {
		return nil
	}

	certs, err := store.ListCertifications(ctx, w.TenantID, w.ID)
	if err != nil {
		return err
	}
	for _, c := range certs {
		if c.WorkType == workType && c.Status == CertStatusValid {
			return nil
		}
	}

	if w.Classification == ClassificationVendor && w.Specialist {
		return nil
	}

	return ErrRestrictedWorkRequiresCert
}

// AssignOperations records a durable operations assignment only after the hard
// eligibility checks pass. Under-18 assignment fails before any record is
// written.
func (s *WorkforceService) AssignOperations(ctx context.Context, tenantID, workerID, workType string, actorID string) (*WorkforceAssignment, error) {
	w, err := s.store.GetWorker(ctx, tenantID, workerID)
	if err != nil {
		return nil, err
	}
	if err := checkOperationsAssignment(ctx, s.store, w, workType); err != nil {
		return nil, err
	}

	assignment := &WorkforceAssignment{
		TenantID:   tenantID,
		WorkerID:   workerID,
		WorkType:   workType,
		AssignedBy: actorID,
	}
	if assignment.WorkType == "" {
		assignment.WorkType = "general"
	}
	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertAssignment(ctx, tx, assignment); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "workforce.worker.assigned",
			ResourceType: "workforce_assignment",
			ResourceID:   assignment.ID,
			NewState:     marshalJSON(assignment),
		})
	}); err != nil {
		return nil, err
	}

	return assignment, nil
}

func (s *WorkforceService) ListAssignments(ctx context.Context, tenantID, workerID string) ([]WorkforceAssignment, error) {
	return s.store.ListAssignments(ctx, tenantID, workerID)
}

// RecordRating stores an advisory human or AI score. It can never reject,
// suspend, or terminate the worker: a deactivating desired status is rejected
// and the worker status is never modified here (WFM-011).
func (s *WorkforceService) RecordRating(ctx context.Context, tenantID, workerID string, params RatingParams, actorID string) (*Rating, error) {
	if params.Score < 0 || params.Score > 100 {
		return nil, ErrInvalidRatingScore
	}
	if params.Source != RatingSourceHuman && params.Source != RatingSourceAI {
		return nil, ErrInvalidRatingSource
	}
	if params.DesiredStatus != "" && IsDeactivatingStatus(params.DesiredStatus) {
		return nil, ErrRatingCannotDeactivate
	}

	if _, err := s.store.GetWorker(ctx, tenantID, workerID); err != nil {
		return nil, err
	}

	rating := &Rating{
		TenantID:   tenantID,
		WorkerID:   workerID,
		Score:      params.Score,
		Source:     params.Source,
		Comment:    params.Comment,
		RecordedBy: actorID,
	}
	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertRating(ctx, tx, rating); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "workforce.worker.rated",
			ResourceType: "worker",
			ResourceID:   workerID,
			NewState:     marshalJSON(rating),
		})
	}); err != nil {
		return nil, err
	}

	// The worker status is untouched by the rating; it is re-read only to
	// prove that no status mutation happened.
	after, err := s.store.GetWorker(ctx, tenantID, workerID)
	if err != nil {
		return nil, err
	}
	rating.WorkerStatusAfter = after.Status

	return rating, nil
}

func (s *WorkforceService) ListRatings(ctx context.Context, tenantID, workerID string) ([]Rating, error) {
	return s.store.ListRatings(ctx, tenantID, workerID)
}

// ReviewAdverseAction is the only path that rejects, suspends, or terminates a
// worker. It requires the evidence considered, a reason, and a distinct human
// reviewer (WFM-011, WFM-012).
func (s *WorkforceService) ReviewAdverseAction(ctx context.Context, tenantID, workerID string, params AdverseActionParams, actorID string) (*Worker, error) {
	if !IsValidAdverseAction(params.Action) {
		return nil, ErrInvalidAdverseAction
	}
	if params.ReviewerID == "" {
		return nil, ErrAdverseActionRequiresReviewer
	}
	if params.ReviewerID == workerID {
		return nil, ErrAdverseActionSelfReview
	}
	if len(params.EvidenceRefs) == 0 {
		return nil, ErrAdverseActionRequiresEvidence
	}
	if params.Reason == "" {
		return nil, ErrAdverseActionRequiresReason
	}

	var result *Worker

	err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		w, err := s.store.GetWorkerForUpdate(ctx, tx, tenantID, workerID)
		if err != nil {
			return err
		}

		review := &AdverseActionReview{
			TenantID:      tenantID,
			WorkerID:      workerID,
			Action:        params.Action,
			EvidenceRefs:  params.EvidenceRefs,
			ReviewerID:    params.ReviewerID,
			Reason:        params.Reason,
			WorkerVersion: w.Version,
		}
		if err := s.store.InsertAdverseAction(ctx, tx, review); err != nil {
			return err
		}

		switch params.Action {
		case AdverseActionReject:
			w.Status = StatusRejected
		case AdverseActionSuspend:
			w.Status = StatusSuspended
		case AdverseActionTerminate:
			w.Status = StatusTerminated
		}
		w.Version++
		w.UpdatedAt = time.Now().UTC()

		if err := s.store.UpdateWorkerStatus(ctx, tx, w); err != nil {
			return err
		}

		result = w
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "workforce.worker.adverse_action_reviewed",
			ResourceType: "worker",
			ResourceID:   workerID,
			NewState:     marshalJSON(result),
		})
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (s *WorkforceService) ListAdverseActions(ctx context.Context, tenantID, workerID string) ([]AdverseActionReview, error) {
	return s.store.ListAdverseActions(ctx, tenantID, workerID)
}

// CreateAvailabilityWindow records a recurring availability window for a
// worker. Days 0 (Sunday) through 6 (Saturday) with 0-1439 minute ranges.
func (s *WorkforceService) CreateAvailabilityWindow(ctx context.Context, tenantID, workerID string, params AvailabilityWindowParams, actorID string) (*AvailabilityWindow, error) {
	if params.DayOfWeek < 0 || params.DayOfWeek > 6 {
		return nil, fmt.Errorf("%w: day_of_week must be 0-6", ErrInvalidAvailabilityWindow)
	}
	if params.StartMinute < 0 || params.EndMinute > 1439 || params.StartMinute >= params.EndMinute {
		return nil, fmt.Errorf("%w: start_minute must be before end_minute, both 0-1439", ErrInvalidAvailabilityWindow)
	}

	if _, err := s.store.GetWorker(ctx, tenantID, workerID); err != nil {
		return nil, err
	}

	window := &AvailabilityWindow{
		TenantID:    tenantID,
		WorkerID:    workerID,
		DayOfWeek:   params.DayOfWeek,
		StartMinute: params.StartMinute,
		EndMinute:   params.EndMinute,
		EffectiveAt: params.EffectiveAt,
	}

	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertAvailabilityWindow(ctx, tx, window); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "workforce.worker.availability.created",
			ResourceType: "availability_window",
			ResourceID:   window.ID,
			NewState:     marshalJSON(window),
		})
	}); err != nil {
		return nil, err
	}

	return window, nil
}

func (s *WorkforceService) ListAvailabilityWindows(ctx context.Context, tenantID, workerID string) ([]AvailabilityWindow, error) {
	return s.store.ListAvailabilityWindows(ctx, tenantID, workerID)
}

// RecordTimeEntry records a discrete block of work, travel, or overtime for
// a worker. Overtime is flagged separately and always attributed to a
// human-approved ticket or dispatch.
func (s *WorkforceService) RecordTimeEntry(ctx context.Context, tenantID, workerID string, params TimeEntryParams, actorID string) (*TimeEntry, error) {
	if params.WorkMinutes < 0 || params.TravelMinutes < 0 {
		return nil, fmt.Errorf("%w: minutes must be non-negative", ErrInvalidTimeEntry)
	}
	if params.WorkMinutes == 0 && params.TravelMinutes == 0 {
		return nil, fmt.Errorf("%w: at least one of work_minutes or travel_minutes must be positive", ErrInvalidTimeEntry)
	}

	w, err := s.store.GetWorker(ctx, tenantID, workerID)
	if err != nil {
		return nil, err
	}
	if IsDeactivatingStatus(w.Status) {
		return nil, fmt.Errorf("%w: worker status %s", ErrWorkerNotAssignable, w.Status)
	}

	entry := &TimeEntry{
		TenantID:      tenantID,
		WorkerID:      workerID,
		TicketID:      params.TicketID,
		WorkMinutes:   params.WorkMinutes,
		TravelMinutes: params.TravelMinutes,
		OvertimeFlag:  params.OvertimeFlag,
		RecordedBy:    actorID,
	}

	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertTimeEntry(ctx, tx, entry); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "workforce.worker.time_recorded",
			ResourceType: "time_entry",
			ResourceID:   entry.ID,
			NewState:     marshalJSON(entry),
		})
	}); err != nil {
		return nil, err
	}

	return entry, nil
}

func (s *WorkforceService) ListTimeEntries(ctx context.Context, tenantID, workerID string) ([]TimeEntry, error) {
	return s.store.ListTimeEntries(ctx, tenantID, workerID)
}

// RecordExpense records an out-of-pocket or reimbursable expense for a
// worker. Money is in integer minor units with ISO 4217 currency.
func (s *WorkforceService) RecordExpense(ctx context.Context, tenantID, workerID string, params ExpenseParams, actorID string) (*Expense, error) {
	if params.MinorUnits <= 0 {
		return nil, fmt.Errorf("%w: minor_units must be positive", ErrInvalidExpense)
	}
	if params.Currency == "" {
		return nil, fmt.Errorf("%w: currency is required", ErrInvalidExpense)
	}

	if _, err := s.store.GetWorker(ctx, tenantID, workerID); err != nil {
		return nil, err
	}

	expense := &Expense{
		TenantID:   tenantID,
		WorkerID:   workerID,
		TicketID:   params.TicketID,
		MinorUnits: params.MinorUnits,
		Currency:   params.Currency,
		Category:   params.Category,
		ReceiptRef: params.ReceiptRef,
		RecordedBy: actorID,
	}

	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertExpense(ctx, tx, expense); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "workforce.worker.expense_recorded",
			ResourceType: "expense",
			ResourceID:   expense.ID,
			NewState:     marshalJSON(expense),
		})
	}); err != nil {
		return nil, err
	}

	return expense, nil
}

func (s *WorkforceService) ListExpenses(ctx context.Context, tenantID, workerID string) ([]Expense, error) {
	return s.store.ListExpenses(ctx, tenantID, workerID)
}

// SubmitGrievance records a worker grievance with mandatory kind, reason, and
// reference evidence.
func (s *WorkforceService) SubmitGrievance(ctx context.Context, tenantID, workerID string, params GrievanceParams, actorID string) (*Grievance, error) {
	if params.Kind == "" || params.Reason == "" {
		return nil, ErrInvalidGrievance
	}

	if _, err := s.store.GetWorker(ctx, tenantID, workerID); err != nil {
		return nil, err
	}

	grievance := &Grievance{
		TenantID:     tenantID,
		WorkerID:     workerID,
		Kind:         params.Kind,
		Reason:       params.Reason,
		EvidenceRefs: params.EvidenceRefs,
		Status:       GrievanceStatusPending,
	}

	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertGrievance(ctx, tx, grievance); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "workforce.worker.grievance_submitted",
			ResourceType: "grievance",
			ResourceID:   grievance.ID,
			NewState:     marshalJSON(grievance),
		})
	}); err != nil {
		return nil, err
	}

	return grievance, nil
}

func (s *WorkforceService) ListGrievances(ctx context.Context, tenantID, workerID string) ([]Grievance, error) {
	return s.store.ListGrievances(ctx, tenantID, workerID)
}

func (s *WorkforceService) GetGrievance(ctx context.Context, tenantID, grievanceID string) (*Grievance, error) {
	return s.store.GetGrievance(ctx, tenantID, grievanceID)
}

// TriggerSOS records a worker-triggered safety alert. It freezes the worker
// by setting status to suspended and queues an immediate human review.
func (s *WorkforceService) TriggerSOS(ctx context.Context, tenantID, workerID string, params SOSEventParams, actorID string) (*SOSEvent, error) {
	w, err := s.store.GetWorker(ctx, tenantID, workerID)
	if err != nil {
		return nil, err
	}

	sosEvent := &SOSEvent{
		TenantID:    tenantID,
		WorkerID:    workerID,
		TicketID:    params.TicketID,
		Location:    params.Location,
		TriggeredAt: time.Now().UTC(),
	}

	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertSOSEvent(ctx, tx, sosEvent); err != nil {
			return err
		}
		if err := s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "workforce.worker.sos_triggered",
			ResourceType: "sos_event",
			ResourceID:   sosEvent.ID,
			NewState:     marshalJSON(sosEvent),
		}); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "workforce.worker.sos_frozen",
			ResourceType: "worker",
			ResourceID:   workerID,
			NewState:     marshalJSON(w),
		})
	}); err != nil {
		return nil, err
	}

	return sosEvent, nil
}

func (s *WorkforceService) ListSOSEvents(ctx context.Context, tenantID, workerID string) ([]SOSEvent, error) {
	return s.store.ListSOSEvents(ctx, tenantID, workerID)
}

// CreateEmploymentTerm records a versioned worker agreement capturing role,
// compensation band, effective dates, and an optional signed document ref.
func (s *WorkforceService) CreateEmploymentTerm(ctx context.Context, tenantID, workerID string, params EmploymentTermParams, actorID string) (*EmploymentTerm, error) {
	if params.Role == "" {
		return nil, fmt.Errorf("%w: role is required", ErrInvalidEmploymentTerm)
	}
	if params.EffectiveDate.IsZero() {
		return nil, fmt.Errorf("%w: effective_date is required", ErrInvalidEmploymentTerm)
	}

	if _, err := s.store.GetWorker(ctx, tenantID, workerID); err != nil {
		return nil, err
	}

	term := &EmploymentTerm{
		TenantID:         tenantID,
		WorkerID:         workerID,
		Role:             params.Role,
		CompensationBand: params.CompensationBand,
		EffectiveDate:    params.EffectiveDate.UTC(),
		EndDate:          params.EndDate,
		AgreementRef:     params.AgreementRef,
	}

	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertEmploymentTerm(ctx, tx, term); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "workforce.worker.term_created",
			ResourceType: "employment_term",
			ResourceID:   term.ID,
			NewState:     marshalJSON(term),
		})
	}); err != nil {
		return nil, err
	}

	return term, nil
}

func (s *WorkforceService) ListEmploymentTerms(ctx context.Context, tenantID, workerID string) ([]EmploymentTerm, error) {
	return s.store.ListEmploymentTerms(ctx, tenantID, workerID)
}

func (s *WorkforceService) appendAudit(ctx context.Context, event audit.AuditEvent) {
	if s.auditStore == nil {
		return
	}
	if event.ID == "" {
		event.ID = newID("aud")
	}
	if err := s.auditStore.Append(ctx, event); err != nil {
		logging.Error(ctx, "failed to append audit event", "error", err)
	}
}

func marshalJSON(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("{}")
	}
	return data
}
