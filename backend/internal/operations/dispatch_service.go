package operations

import (
	"context"
	"fmt"
	"time"

	"comfort-curators-backend/internal/platform/logging"

	"github.com/jackc/pgx/v5/pgxpool"
)

type DispatchService struct {
	pool          *pgxpool.Pool
	dispatchStore *dispatchStore
	ticketStore   *TicketStore
}

func NewDispatchService(pool *pgxpool.Pool) *DispatchService {
	return &DispatchService{
		pool:          pool,
		dispatchStore: newDispatchStore(pool),
		ticketStore:   NewTicketStore(pool),
	}
}

func (s *DispatchService) EvaluateCandidates(ctx context.Context, tenantID, ticketID, workType string) (*DispatchCandidatesResponse, error) {
	workers, err := s.dispatchStore.listActiveWorkers(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list active workers: %w", err)
	}

	constraints := defaultConstraints()
	if IsHighRiskTicketType(ticketTypeFromContext(workType)) {
		constraints.RequiresTwoPerson = true
	}
	constraints.RequiredSkills = requiredSkillsForWorkType(workType)

	var candidates []WorkerEligibility
	for _, w := range workers {
		eligibility := s.evaluateWorker(ctx, &w, constraints, workType)
		candidates = append(candidates, eligibility)
	}

	return &DispatchCandidatesResponse{
		TicketID:   ticketID,
		Candidates: candidates,
	}, nil
}

func (s *DispatchService) AssignWorker(ctx context.Context, tenantID, ticketID, workerID, workType string, actorID string) (*TicketAssignment, *PayTreatment, error) {
	ticket, err := s.ticketStore.GetTicket(ctx, tenantID, ticketID)
	if err != nil {
		return nil, nil, err
	}

	if ticket.Status != StateScheduled && ticket.Status != StateAssigned {
		return nil, nil, ErrDispatchTicketNotAssignable
	}

	workers, err := s.dispatchStore.listActiveWorkers(ctx, tenantID)
	if err != nil {
		return nil, nil, fmt.Errorf("list active workers: %w", err)
	}

	var targetWorker *workforceWorker
	for _, w := range workers {
		if w.ID == workerID {
			ww := w
			targetWorker = &ww
			break
		}
	}
	if targetWorker == nil {
		return nil, nil, fmt.Errorf("worker %s is not active or does not exist", workerID)
	}

	constraints := defaultConstraints()
	if IsHighRiskTicketType(ticket.Type) {
		constraints.RequiresTwoPerson = true
	}
	constraints.RequiredSkills = requiredSkillsForWorkType(ticket.Type)

	eligibility := s.evaluateWorker(ctx, targetWorker, constraints, ticket.Type)
	if !eligibility.Eligible {
		detail := s.firstFailingConstraint(eligibility.Checks)
		return nil, nil, fmt.Errorf("%w: %s", ErrDispatchWorkerNotEligible, detail)
	}

	if constraints.RequiresTwoPerson {
		existingAssignments, err := s.dispatchStore.ListAssignmentsForTicket(ctx, tenantID, ticketID)
		if err != nil {
			return nil, nil, err
		}
		if len(existingAssignments) == 0 {
			term, err := s.dispatchStore.getWorkerEmploymentTerm(ctx, tenantID, workerID)
			if err != nil {
				return nil, nil, err
			}
			payTreatment := &PayTreatment{
				WorkerID: workerID,
			}
			if term != nil {
				payTreatment.Role = term.Role
				payTreatment.CompensationBand = term.CompensationBand
			}

			assignment := &TicketAssignment{
				TenantID:   tenantID,
				TicketID:   ticketID,
				WorkerID:   workerID,
				AssignedBy: actorID,
				Status:     AssignmentStatusOffered,
			}
			if err := s.dispatchStore.InsertAssignment(ctx, assignment); err != nil {
				return nil, nil, err
			}
			return assignment, payTreatment, nil
		}
	}

	term, err := s.dispatchStore.getWorkerEmploymentTerm(ctx, tenantID, workerID)
	if err != nil {
		return nil, nil, err
	}
	payTreatment := &PayTreatment{
		WorkerID: workerID,
	}
	if term != nil {
		payTreatment.Role = term.Role
		payTreatment.CompensationBand = term.CompensationBand
	}

	assignment := &TicketAssignment{
		TenantID:   tenantID,
		TicketID:   ticketID,
		WorkerID:   workerID,
		AssignedBy: actorID,
		Status:     AssignmentStatusOffered,
	}
	if err := s.dispatchStore.InsertAssignment(ctx, assignment); err != nil {
		return nil, nil, err
	}

	return assignment, payTreatment, nil
}

func (s *DispatchService) OverrideAssignment(ctx context.Context, tenantID, ticketID, workerID, workType string, req DispatchOverrideRequest, actorID string) (*TicketAssignment, *PayTreatment, *DispatchOverride, error) {
	if req.Reason == "" {
		return nil, nil, nil, ErrDispatchOverrideRequiresReason
	}
	if req.OverriddenConstraint == "" {
		return nil, nil, nil, ErrDispatchOverrideRequiresConstraint
	}
	if actorID == "" {
		return nil, nil, nil, ErrDispatchOverrideNotAttributed
	}

	ticket, err := s.ticketStore.GetTicket(ctx, tenantID, ticketID)
	if err != nil {
		return nil, nil, nil, err
	}

	if ticket.Status != StateScheduled && ticket.Status != StateAssigned {
		return nil, nil, nil, ErrDispatchTicketNotAssignable
	}

	workers, err := s.dispatchStore.listActiveWorkers(ctx, tenantID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("list active workers: %w", err)
	}

	var targetWorker *workforceWorker
	for _, w := range workers {
		if w.ID == workerID {
			ww := w
			targetWorker = &ww
			break
		}
	}
	if targetWorker == nil {
		return nil, nil, nil, fmt.Errorf("worker %s is not active or does not exist", workerID)
	}

	override := &DispatchOverride{
		TenantID:             tenantID,
		TicketID:             ticketID,
		WorkerID:             workerID,
		OverriddenBy:         actorID,
		Reason:               req.Reason,
		OverriddenConstraint: req.OverriddenConstraint,
	}
	if err := s.dispatchStore.InsertOverride(ctx, override); err != nil {
		return nil, nil, nil, err
	}

	term, err := s.dispatchStore.getWorkerEmploymentTerm(ctx, tenantID, workerID)
	if err != nil {
		return nil, nil, nil, err
	}
	payTreatment := &PayTreatment{
		WorkerID: workerID,
	}
	if term != nil {
		payTreatment.Role = term.Role
		payTreatment.CompensationBand = term.CompensationBand
	}

	assignment := &TicketAssignment{
		TenantID:   tenantID,
		TicketID:   ticketID,
		WorkerID:   workerID,
		AssignedBy: actorID,
		Status:     AssignmentStatusOffered,
	}
	if err := s.dispatchStore.InsertAssignment(ctx, assignment); err != nil {
		return nil, nil, nil, err
	}

	return assignment, payTreatment, override, nil
}

func (s *DispatchService) AcceptAssignment(ctx context.Context, tenantID, assignmentID, workerID string) (*TicketAssignment, *PayTreatment, error) {
	assignment, err := s.dispatchStore.GetAssignment(ctx, tenantID, assignmentID)
	if err != nil {
		return nil, nil, err
	}

	if assignment.Status != AssignmentStatusOffered {
		return nil, nil, fmt.Errorf("%w: current status is %s", ErrDispatchAssignmentNotOffered, assignment.Status)
	}

	if assignment.WorkerID != workerID {
		return nil, nil, ErrDispatchNotWorker
	}

	now := time.Now().UTC()
	assignment.Status = AssignmentStatusAccepted
	assignment.AcceptedAt = &now
	if err := s.dispatchStore.UpdateAssignmentStatus(ctx, assignment); err != nil {
		return nil, nil, err
	}

	term, err := s.dispatchStore.getWorkerEmploymentTerm(ctx, tenantID, workerID)
	if err != nil {
		return nil, nil, err
	}
	payTreatment := &PayTreatment{
		WorkerID: workerID,
	}
	if term != nil {
		payTreatment.Role = term.Role
		payTreatment.CompensationBand = term.CompensationBand
	}

	logging.Info(ctx, "worker accepted dispatch assignment",
		"assignment_id", assignmentID,
		"worker_id", workerID,
		"ticket_id", assignment.TicketID,
	)

	return assignment, payTreatment, nil
}

func (s *DispatchService) DeclineAssignment(ctx context.Context, tenantID, assignmentID, workerID string) (*TicketAssignment, error) {
	assignment, err := s.dispatchStore.GetAssignment(ctx, tenantID, assignmentID)
	if err != nil {
		return nil, err
	}

	if assignment.Status != AssignmentStatusOffered {
		return nil, fmt.Errorf("%w: current status is %s", ErrDispatchAssignmentNotOffered, assignment.Status)
	}

	if assignment.WorkerID != workerID {
		return nil, ErrDispatchNotWorker
	}

	assignment.Status = AssignmentStatusDeclined
	if err := s.dispatchStore.UpdateAssignmentStatus(ctx, assignment); err != nil {
		return nil, err
	}

	logging.Info(ctx, "worker declined dispatch assignment",
		"assignment_id", assignmentID,
		"worker_id", workerID,
		"ticket_id", assignment.TicketID,
	)

	return assignment, nil
}

func (s *DispatchService) GetAssignment(ctx context.Context, tenantID, assignmentID string) (*TicketAssignment, *PayTreatment, error) {
	assignment, err := s.dispatchStore.GetAssignment(ctx, tenantID, assignmentID)
	if err != nil {
		return nil, nil, err
	}

	term, err := s.dispatchStore.getWorkerEmploymentTerm(ctx, tenantID, assignment.WorkerID)
	if err != nil {
		return nil, nil, err
	}
	payTreatment := &PayTreatment{
		WorkerID: assignment.WorkerID,
	}
	if term != nil {
		payTreatment.Role = term.Role
		payTreatment.CompensationBand = term.CompensationBand
	}

	return assignment, payTreatment, nil
}

func (s *DispatchService) ListAssignmentsForTicket(ctx context.Context, tenantID, ticketID string) ([]TicketAssignment, error) {
	return s.dispatchStore.ListAssignmentsForTicket(ctx, tenantID, ticketID)
}

func (s *DispatchService) ListAssignmentsForWorker(ctx context.Context, tenantID, workerID string) ([]TicketAssignment, error) {
	return s.dispatchStore.ListAssignmentsForWorker(ctx, tenantID, workerID)
}

func (s *DispatchService) ListOverridesForTicket(ctx context.Context, tenantID, ticketID string) ([]DispatchOverride, error) {
	return s.dispatchStore.ListOverridesForTicket(ctx, tenantID, ticketID)
}

func (s *DispatchService) GetPayTreatment(ctx context.Context, tenantID, workerID string) (*PayTreatment, error) {
	term, err := s.dispatchStore.getWorkerEmploymentTerm(ctx, tenantID, workerID)
	if err != nil {
		return nil, err
	}
	pt := &PayTreatment{
		WorkerID: workerID,
	}
	if term != nil {
		pt.Role = term.Role
		pt.CompensationBand = term.CompensationBand
	}
	return pt, nil
}

func (s *DispatchService) evaluateWorker(ctx context.Context, w *workforceWorker, constraints DispatchConstraints, workType string) WorkerEligibility {
	var checks []ConstraintCheck

	ageCheck := constraintCheck(ConstraintAge, w.AgeEligible, "")
	if !w.AgeEligible {
		ageCheck.Detail = "worker is under 18"
	}
	checks = append(checks, ageCheck)

	activeCheck := constraintCheck("worker_status", w.Status == "active", "")
	if w.Status != "active" {
		activeCheck.Detail = fmt.Sprintf("worker status is %s", w.Status)
	}
	checks = append(checks, activeCheck)

	skillCheck := constraintCheck(ConstraintSkill, true, "")
	hasRequiredSkill := true
	if len(constraints.RequiredSkills) > 0 {
		hasRequiredSkill = false
		for _, required := range constraints.RequiredSkills {
			for _, workerSkill := range w.Skills {
				if workerSkill == required {
					hasRequiredSkill = true
					break
				}
			}
			if hasRequiredSkill {
				break
			}
		}
		if !hasRequiredSkill {
			skillCheck.Passed = false
			skillCheck.Detail = "worker does not possess any required skill"
		}
	}
	checks = append(checks, skillCheck)

	zoneCheck := constraintCheck(ConstraintZone, true, "")
	if constraints.ServiceZone != "" {
		if w.ServiceZone != constraints.ServiceZone {
			zoneCheck.Passed = false
			zoneCheck.Detail = fmt.Sprintf("worker zone %s != required zone %s", w.ServiceZone, constraints.ServiceZone)
		}
	}
	checks = append(checks, zoneCheck)

	availabilityCheck := constraintCheck(ConstraintAvailability, true, "")
	availWindows, err := s.dispatchStore.listWorkerAvailability(ctx, w.TenantID, w.ID)
	if err == nil && len(availWindows) > 0 {
		availabilityCheck.Passed = true
		availabilityCheck.Detail = "worker has availability windows configured"
	} else {
		availabilityCheck.Passed = false
		availabilityCheck.Detail = "worker has no availability windows configured"
	}
	checks = append(checks, availabilityCheck)

	hoursCheck := constraintCheck(ConstraintWorkingHours, true, "")
	todayStart := time.Now().UTC().Truncate(24 * time.Hour)
	entries, err := s.dispatchStore.listWorkerTimeEntries(ctx, w.TenantID, w.ID, todayStart)
	if err == nil {
		totalMinutes := 0
		for _, e := range entries {
			totalMinutes += e.WorkMinutes + e.TravelMinutes
		}
		if constraints.MaxDailyWorkMinutes > 0 && totalMinutes >= constraints.MaxDailyWorkMinutes {
			hoursCheck.Passed = false
			hoursCheck.Detail = fmt.Sprintf("worker has %d minutes today; limit is %d", totalMinutes, constraints.MaxDailyWorkMinutes)
		} else {
			hoursCheck.Passed = true
			hoursCheck.Detail = fmt.Sprintf("worker has %d work minutes today; limit is %d", totalMinutes, constraints.MaxDailyWorkMinutes)
		}
	} else {
		hoursCheck.Passed = true
	}
	checks = append(checks, hoursCheck)

	restCheck := constraintCheck(ConstraintRest, true, "")
	if len(entries) > 0 {
		lastEntry := entries[len(entries)-1]
		minutesSinceLast := int(time.Since(lastEntry.RecordedAt).Minutes())
		if constraints.MinRestMinutes > 0 && minutesSinceLast < constraints.MinRestMinutes {
			restCheck.Passed = false
			restCheck.Detail = fmt.Sprintf("only %d minutes since last entry; need %d minutes rest", minutesSinceLast, constraints.MinRestMinutes)
		} else {
			restCheck.Passed = true
			restCheck.Detail = fmt.Sprintf("%d minutes since last entry; minimum rest is %d", minutesSinceLast, constraints.MinRestMinutes)
		}
	} else {
		restCheck.Passed = true
	}
	checks = append(checks, restCheck)

	safetyCheck := constraintCheck(ConstraintSafety, !IsHighRiskTicketType(workType) || w.Classification == "vendor" || w.Specialist, "")
	if IsHighRiskTicketType(workType) && w.Classification != "vendor" && !w.Specialist {
		if IsRestrictedTicketWorkType(workType) {
			certs, err := s.dispatchStore.listWorkerCertifications(ctx, w.TenantID, w.ID)
			if err == nil {
				hasValidCert := false
				for _, c := range certs {
					if c.WorkType == workType && c.Status == "valid" {
						hasValidCert = true
						break
					}
				}
				if !hasValidCert {
					safetyCheck.Passed = false
					safetyCheck.Detail = fmt.Sprintf("restricted work type %s requires valid certification", workType)
				}
			}
		}
	}
	checks = append(checks, safetyCheck)

	twoPersonCheck := constraintCheck(ConstraintTwoPerson, !constraints.RequiresTwoPerson, "")
	if constraints.RequiresTwoPerson {
		twoPersonCheck.Detail = "two-person assignment is required; only first worker can be assigned through standard dispatch"
	}
	checks = append(checks, twoPersonCheck)

	score := 0
	eligible := true
	for _, c := range checks {
		if !c.Passed {
			if c.Hard {
				eligible = false
			}
		}
		if c.Hard && c.Passed {
			score += 10
		}
		if !c.Hard && c.Passed {
			score += 5
		}
	}

	return WorkerEligibility{
		WorkerID: w.ID,
		Eligible: eligible,
		Score:    score,
		Checks:   checks,
	}
}

func (s *DispatchService) firstFailingConstraint(checks []ConstraintCheck) string {
	for _, c := range checks {
		if !c.Passed && c.Hard {
			if c.Detail != "" {
				return c.Detail
			}
			return fmt.Sprintf("hard constraint %s failed", c.Constraint)
		}
	}
	return "hard constraint violation"
}

func defaultConstraints() DispatchConstraints {
	return DispatchConstraints{
		RequiredSkills:       nil,
		ServiceZone:          "",
		RequiresTwoPerson:    false,
		MaxDailyWorkMinutes:  480,
		MaxWeeklyWorkMinutes: 2400,
		MinRestMinutes:       480,
		MaxTravelMinutes:     120,
	}
}

func requiredSkillsForWorkType(workType string) []string {
	switch workType {
	case TypeRestock:
		return []string{"restock", "inventory"}
	case TypeRoutineMaintenance:
		return []string{"maintenance", "general"}
	case TypeSpecialistVendorRequest:
		return []string{"specialist"}
	case TypeTurnover:
		return []string{"cleaning", "turnover"}
	case TypePreArrivalInspection:
		return []string{"inspection"}
	case TypeIncident:
		return []string{"incident_response"}
	case TypePropertyOnboarding:
		return []string{"onboarding"}
	case TypeDocumentReview:
		return []string{"document_review"}
	case TypeInventoryCount:
		return []string{"inventory"}
	default:
		return nil
	}
}

func ticketTypeFromContext(workType string) string {
	for _, tt := range AllTicketTypes {
		if tt == workType {
			return tt
		}
	}
	return ""
}

func IsRestrictedTicketWorkType(workType string) bool {
	switch workType {
	case TypeSpecialistVendorRequest:
		return true
	default:
		return false
	}
}
