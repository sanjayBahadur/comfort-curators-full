package operations

import (
	"encoding/json"
	"errors"
	"time"
)

const (
	AssignmentStatusOffered   = "offered"
	AssignmentStatusAccepted  = "accepted"
	AssignmentStatusDeclined  = "declined"
	AssignmentStatusCompleted = "completed"
)

const (
	ConstraintSkill        = "skill"
	ConstraintAge          = "age"
	ConstraintZone         = "service_zone"
	ConstraintAvailability = "availability"
	ConstraintWorkingHours = "working_hours"
	ConstraintRest         = "rest"
	ConstraintTravel       = "travel"
	ConstraintSafety       = "safety"
	ConstraintAccess       = "access"
	ConstraintTwoPerson    = "two_person"
)

var AllHardConstraints = []string{
	ConstraintSkill,
	ConstraintAge,
	ConstraintZone,
	ConstraintAvailability,
	ConstraintWorkingHours,
	ConstraintRest,
	ConstraintTravel,
	ConstraintSafety,
	ConstraintAccess,
	ConstraintTwoPerson,
}

var (
	ErrDispatchAssignmentNotFound         = errors.New("ticket assignment not found")
	ErrDispatchWorkerNotEligible          = errors.New("worker does not meet hard dispatch constraints")
	ErrDispatchAssignmentNotOffered       = errors.New("assignment is not in offered status")
	ErrDispatchNotWorker                  = errors.New("only the assigned worker can accept or decline their assignment")
	ErrDispatchOverrideRequiresReason     = errors.New("dispatch override requires a reason")
	ErrDispatchOverrideRequiresConstraint = errors.New("dispatch override must name the overridden constraint")
	ErrDispatchOverrideNotAttributed      = errors.New("dispatch override must be attributed to an actor")
	ErrDispatchTicketNotAssignable        = errors.New("ticket is not in an assignable state")
	ErrDispatchHardConstraintViolated     = errors.New("hard constraint cannot be optimized away or overridden by score")
	ErrDispatchTwoPersonRequired          = errors.New("two-person assignment is required for this ticket type")
	ErrDispatchRestNotSatisfied           = errors.New("worker has not had adequate rest since previous assignment")
	ErrDispatchWorkingHoursExceeded       = errors.New("worker has exceeded daily or weekly working-hour limits")
	ErrDispatchOutsideAvailability        = errors.New("ticket time window falls outside worker availability")
	ErrDispatchSkillMissing               = errors.New("worker does not possess the required skill")
	ErrDispatchZoneMismatch               = errors.New("worker service zone does not match the property zone")
)

type DispatchConstraints struct {
	RequiredSkills       []string `json:"required_skills"`
	ServiceZone          string   `json:"service_zone"`
	RequiresTwoPerson    bool     `json:"requires_two_person"`
	MaxDailyWorkMinutes  int      `json:"max_daily_work_minutes"`
	MaxWeeklyWorkMinutes int      `json:"max_weekly_work_minutes"`
	MinRestMinutes       int      `json:"min_rest_minutes"`
	MaxTravelMinutes     int      `json:"max_travel_minutes"`
}

type WorkerEligibility struct {
	WorkerID string            `json:"worker_id"`
	Eligible bool              `json:"eligible"`
	Score    int               `json:"score"`
	Checks   []ConstraintCheck `json:"checks"`
}

type ConstraintCheck struct {
	Constraint string `json:"constraint"`
	Hard       bool   `json:"hard"`
	Passed     bool   `json:"passed"`
	Detail     string `json:"detail,omitempty"`
}

type PayTreatment struct {
	Role             string `json:"role"`
	CompensationBand string `json:"compensation_band,omitempty"`
	WorkerID         string `json:"worker_id"`
}

type TicketAssignment struct {
	ID          string     `json:"id"`
	TenantID    string     `json:"tenant_id"`
	TicketID    string     `json:"ticket_id"`
	WorkerID    string     `json:"worker_id"`
	AssignedBy  string     `json:"assigned_by"`
	Status      string     `json:"status"`
	AcceptUntil *time.Time `json:"accept_until,omitempty"`
	AcceptedAt  *time.Time `json:"accepted_at,omitempty"`
	Version     int        `json:"version"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type DispatchOverride struct {
	ID                   string    `json:"id"`
	TenantID             string    `json:"tenant_id"`
	TicketID             string    `json:"ticket_id"`
	WorkerID             string    `json:"worker_id"`
	OverriddenBy         string    `json:"overridden_by"`
	Reason               string    `json:"reason"`
	OverriddenConstraint string    `json:"overridden_constraint"`
	CreatedAt            time.Time `json:"created_at"`
}

type DispatchCandidatesRequest struct {
	TicketID string `json:"ticket_id"`
	WorkType string `json:"work_type,omitempty"`
}

type DispatchCandidatesResponse struct {
	TicketID   string              `json:"ticket_id"`
	Candidates []WorkerEligibility `json:"candidates"`
}

type DispatchAssignRequest struct {
	WorkerID string `json:"worker_id"`
}

type DispatchOverrideRequest struct {
	WorkerID             string `json:"worker_id"`
	Reason               string `json:"reason"`
	OverriddenConstraint string `json:"overridden_constraint"`
}

type DispatchAcceptRequest struct {
	WorkerID string `json:"worker_id"`
}

type dispatchAssignmentResource struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
	Data    any    `json:"data"`
}

type dispatchCandidatesResource struct {
	Data any `json:"data"`
}

func isHardConstraint(c string) bool {
	for _, hc := range AllHardConstraints {
		if hc == c {
			return true
		}
	}
	return false
}

func constraintCheck(constraint string, passed bool, detail string) ConstraintCheck {
	return ConstraintCheck{
		Constraint: constraint,
		Hard:       isHardConstraint(constraint),
		Passed:     passed,
		Detail:     detail,
	}
}

func marshalDispatchJSON(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("{}")
	}
	return data
}
