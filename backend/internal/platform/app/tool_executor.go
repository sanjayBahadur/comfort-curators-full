package app

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"comfort-curators-backend/internal/automation"
	"comfort-curators-backend/internal/automation/superhost"
	"comfort-curators-backend/internal/iam"
	"comfort-curators-backend/internal/operations"

	"github.com/jackc/pgx/v5/pgxpool"
)

type superhostToolExecutor struct {
	pool         *pgxpool.Pool
	policy       *superhost.PolicyEngine
	store        *superhost.ToolCallStore
	assembler    *superhost.ContextAssembler
	tickets      *operations.TicketService
	accountTasks *superhost.AccountTaskStore
	identity     *iam.IdentityService
}

func newSuperhostToolExecutor(pool *pgxpool.Pool) *superhostToolExecutor {
	return &superhostToolExecutor{
		pool:         pool,
		policy:       superhost.NewPolicyEngine(),
		store:        superhost.NewToolCallStore(pool),
		assembler:    superhost.NewContextAssembler(pool),
		tickets:      operations.NewTicketService(pool),
		accountTasks: superhost.NewAccountTaskStore(pool),
		identity:     iam.NewIdentityService(pool, nil),
	}
}

// approvedTicketType maps a "propose_*" superhost tool -- one that requires
// human approval before anything happens -- to the real operations.Ticket
// type it should create once a human actually approves it. Kept as a plain
// map so extending "propose the model can call" to "a real ticket a
// curator sees in their queue" is a one-line addition, not new plumbing.
var approvedTicketType = map[string]string{
	"propose_restock":             operations.TypeRestock,
	"propose_inspection_ticket":   operations.TypePreArrivalInspection,
	"propose_maintenance_request": operations.TypeRoutineMaintenance,
	"propose_incident_report":     operations.TypeIncident,
}

// ExecuteApproved implements automation.ApprovedToolExecutor: it's called
// once, after a human has genuinely approved a tool call (see
// runner.go's resumeRun), to carry out the tool's real effect. Before this
// existed, an approval only ever produced a canned "Approved by human
// reviewer." string with no actual side effect -- the operator would see
// Superhost say a restock was approved, but nothing was ever created
// anywhere a curator could see it. Tools with no mapped real effect still
// get an honest, non-fabricated acknowledgement instead of a fake result.
func (e *superhostToolExecutor) ExecuteApproved(ctx context.Context, run *automation.AgentRun, toolName string, arguments json.RawMessage) (string, error) {
	ticketType, ok := approvedTicketType[toolName]
	if !ok {
		return fmt.Sprintf("Approved by human reviewer. %s has no automated execution configured yet -- no ticket or record was created.", toolName), nil
	}

	var args struct {
		Reason string `json:"reason"`
	}
	_ = json.Unmarshal(arguments, &args)
	reason := args.Reason
	if reason == "" {
		reason = fmt.Sprintf("Superhost-proposed %s, approved by a human reviewer.", toolName)
	}

	ticket, err := e.tickets.CreateTicket(ctx, operations.CreateTicketParams{
		TenantID:   run.TenantID,
		PropertyID: run.PropertyID,
		Type:       ticketType,
		Reason:     reason,
	}, run.ActorID)
	if err != nil {
		return "", fmt.Errorf("create %s ticket: %w", ticketType, err)
	}

	return fmt.Sprintf("Approved by human reviewer. Created ticket %s (type: %s, state: %s) for operations to action.", ticket.ID, ticket.Type, ticket.Status), nil
}

type rawToolCall struct {
	ID       string        `json:"id"`
	Type     string        `json:"type"`
	Function rawToolCallFn `json:"function"`
}

type rawToolCallFn struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func (e *superhostToolExecutor) Evaluate(ctx context.Context, run *automation.AgentRun, toolCall json.RawMessage) (automation.ToolLoopOutcome, error) {
	var raw rawToolCall
	if err := json.Unmarshal(toolCall, &raw); err != nil {
		return automation.ToolLoopOutcome{
			Type:         automation.ToolLoopDenied,
			ToolName:     "unknown",
			Version:      "",
			DenialReason: fmt.Sprintf("unmarshal tool call: %v", err),
		}, nil
	}

	input := superhost.ToolCallInput{
		ToolName:       raw.Function.Name,
		Version:        "v1",
		Arguments:      automation.NormalizeToolArguments(raw.Function.Arguments),
		CallID:         raw.ID,
		IdempotencyKey: raw.ID,
	}

	// run.PropertyID == "" is a portfolio-scoped run (see
	// ContextAssembler.AssemblePortfolio) -- superhost.ValidateScope
	// deliberately skips its property_id match check in that case (any
	// property_id is expected, the run isn't locked to one), so this is
	// the real check: does the property_id the model named actually
	// belong to this run's tenant? Pure ValidateScope has no DB access to
	// verify that; this executor does.
	if run.PropertyID == "" {
		var args map[string]any
		if err := json.Unmarshal(input.Arguments, &args); err == nil {
			if p, ok := args["property_id"]; ok {
				if propertyID, ok := p.(string); ok && propertyID != "" {
					var belongsToTenant bool
					if err := e.pool.QueryRow(ctx,
						`SELECT EXISTS(SELECT 1 FROM properties WHERE id = $1 AND tenant_id = $2)`,
						propertyID, run.TenantID,
					).Scan(&belongsToTenant); err != nil {
						return automation.ToolLoopOutcome{}, fmt.Errorf("verify portfolio property scope: %w", err)
					}
					if !belongsToTenant {
						return automation.ToolLoopOutcome{
							Type:         automation.ToolLoopDenied,
							ToolName:     input.ToolName,
							Version:      input.Version,
							DenialReason: fmt.Sprintf("property_id %q is not on this tenant", propertyID),
						}, nil
					}
				}
			}
		}
	}

	now := time.Now().UTC()

	tc := superhost.ToolCallRecord{
		CallID:         input.CallID,
		RunID:          run.RunID,
		ToolName:       input.ToolName,
		ToolVersion:    input.Version,
		ToolKind:       "",
		State:          superhost.ToolCallStatePolicyChecking,
		InputData:      input.Arguments,
		IdempotencyKey: input.IdempotencyKey,
		TenantID:       run.TenantID,
		PropertyID:     run.PropertyID,
		ActorID:        run.ActorID,
		Attempt:        1,
		MaxAttempts:    3,
		CreatedAt:      now,
	}
	if err := e.store.RecordToolCall(ctx, tc); err != nil {
		// An infrastructure failure here is not a policy decision — folding
		// it into ToolLoopDenied would tell the model "policy denied" for
		// what's actually a database outage. Return a real error so the
		// runner fails the run instead of misreporting why.
		return automation.ToolLoopOutcome{}, fmt.Errorf("record tool call: %w", err)
	}

	// Resolve the real role for this run's actor so tool-audience gating
	// (owner/operations/guest -- see policy.go's audienceAllowed) has
	// something real to check against. Looked up fresh here rather than
	// cached on the run: a role is current-truth data, and re-resolving
	// it costs one indexed lookup by primary key.
	var actorRoles []string
	if run.ActorID != "" {
		if user, err := e.identity.GetUserByID(ctx, run.ActorID); err == nil {
			actorRoles = []string{user.Role}
		}
		// A lookup failure (unknown actor, demo data gap) leaves
		// actorRoles empty -- audienceAllowed fails closed for anything
		// role-scoped rather than guessing a role.
	}

	pctx := superhost.PolicyContext{
		RunID:      run.RunID,
		TenantID:   run.TenantID,
		PropertyID: run.PropertyID,
		ActorID:    run.ActorID,
		ActorRoles: actorRoles,
	}

	decision := e.policy.Evaluate(pctx, input)

	if err := e.store.RecordPolicyDecision(ctx, decision); err != nil {
		return automation.ToolLoopOutcome{}, fmt.Errorf("record policy decision: %w", err)
	}

	switch decision.Result {
	case superhost.PolicyDenied:
		return automation.ToolLoopOutcome{
			Type:         automation.ToolLoopDenied,
			ToolName:     input.ToolName,
			Version:      input.Version,
			DenialReason: decision.Reason,
		}, nil

	case superhost.PolicyApprovalRequired:
		requestID := fmt.Sprintf("ar_%d", now.UnixNano())
		summary := fmt.Sprintf("i can call %s for %s. i have not done it yet. it needs your ok.",
			input.ToolName, run.PropertyID)

		def, _ := superhost.LookupTool(input.ToolName)
		approvalKind := def.ApprovalKind
		if approvalKind == "" {
			approvalKind = "operations"
		}

		ar := superhost.NewApprovalRequest(
			requestID, run.RunID, decision.DecisionID,
			input.ToolName, input.Version, approvalKind,
			run.ActorID, run.TenantID, run.PropertyID,
			nil, input.Arguments,
		)

		if err := e.store.RecordApprovalRequest(ctx, *ar); err != nil {
			return automation.ToolLoopOutcome{}, fmt.Errorf("record approval request: %w", err)
		}

		return automation.ToolLoopOutcome{
			Type:              automation.ToolLoopApprovalRequired,
			ToolName:          input.ToolName,
			Version:           input.Version,
			ApprovalRequestID: requestID,
			ApprovalSummary:   summary,
		}, nil

	case superhost.PolicyAllowed:
		def, _ := superhost.LookupTool(input.ToolName)
		var resultSummary string

		if def.Kind == superhost.ToolKindUIAction {
			surfaceID := "(no surface_id given)"
			var args map[string]any
			if err := json.Unmarshal(input.Arguments, &args); err == nil {
				if id, ok := args["surface_id"]; ok {
					if sid, ok := id.(string); ok && sid != "" {
						surfaceID = sid
					}
				}
			}
			resultSummary = fmt.Sprintf("ui action %s queued for surface %q; the browser's gated control-session driver executes it, this backend does not receive confirmation this turn", input.ToolName, surfaceID)
		} else {
			var err error
			resultSummary, err = e.executeReadTool(ctx, input.ToolName, run, input.Arguments)
			if err != nil {
				resultSummary = fmt.Sprintf("[STUB] read tool %s: execution error: %v", input.ToolName, err)
			}
		}

		return automation.ToolLoopOutcome{
			Type:          automation.ToolLoopAllowed,
			ToolName:      input.ToolName,
			Version:       input.Version,
			ResultSummary: resultSummary,
		}, nil

	default:
		return automation.ToolLoopOutcome{
			Type:         automation.ToolLoopDenied,
			ToolName:     input.ToolName,
			Version:      input.Version,
			DenialReason: fmt.Sprintf("unexpected policy result: %s", decision.Result),
		}, nil
	}
}

// executeReadTool runs every PolicyAllowed, non-UI-action tool -- mostly
// real read tools, but also the account task ledger's log/resolve tools
// (see account_tasks.go): those are writes, but writes to Superhost's own
// per-account scratchpad rather than real business state, which is why
// they're auto-allowed (RequiresApproval: false) rather than routed
// through the approval flow real business mutations require.
func (e *superhostToolExecutor) executeReadTool(ctx context.Context, toolName string, run *automation.AgentRun, arguments json.RawMessage) (string, error) {
	switch toolName {
	case "list_my_tasks":
		return e.listMyTasks(ctx, run)
	case "log_task":
		return e.logTask(ctx, run, arguments)
	case "resolve_task":
		return e.resolveTask(ctx, run, arguments)
	}

	// Portfolio-scoped run (run.PropertyID == ""): the model can still
	// name one property_id in a read tool's arguments to scope the
	// summary to just that property (already tenant-verified above);
	// with none given, summarize across the whole portfolio instead.
	propertyID := run.PropertyID
	if propertyID == "" {
		var args map[string]any
		if err := json.Unmarshal(arguments, &args); err == nil {
			if p, ok := args["property_id"].(string); ok {
				propertyID = p
			}
		}
	}

	if propertyID == "" && run.PropertyID == "" {
		pc, err := e.assembler.AssemblePortfolio(ctx, run.TenantID, run.ActorID)
		if err != nil {
			return "", fmt.Errorf("portfolio context assembly: %w", err)
		}
		// Every read tool gets the same portfolio overview when no single
		// property is named -- there isn't a per-tool distinction to make
		// yet at this scope (get_reservation_change/summarize_incident
		// need one property's detail, which is exactly what naming a
		// property_id argument gets you, handled above).
		return e.buildPortfolioSummary(pc), nil
	}

	pc, err := e.assembler.Assemble(ctx, run.TenantID, propertyID, run.ActorID)
	if err != nil {
		return "", fmt.Errorf("context assembly: %w", err)
	}

	switch toolName {
	case "get_property_operating_summary":
		return e.buildOperatingSummary(pc), nil
	case "get_reservation_change":
		return e.buildReservationChange(pc), nil
	case "summarize_incident":
		return e.buildIncidentSummary(pc), nil
	default:
		return fmt.Sprintf("[STUB] read tool %s executed — no dedicated result builder yet. Context assembled for %s/%s with %d reservations, %d tickets.",
			toolName, run.TenantID, propertyID, len(pc.Reservations), len(pc.Tickets)), nil
	}
}

func (e *superhostToolExecutor) listMyTasks(ctx context.Context, run *automation.AgentRun) (string, error) {
	tasks, err := e.accountTasks.ListForAccount(ctx, run.TenantID, run.ActorID, 10)
	if err != nil {
		return "", fmt.Errorf("list account tasks: %w", err)
	}
	if len(tasks) == 0 {
		return "No prior tasks logged for this account.", nil
	}
	out := fmt.Sprintf("%d task(s) on this account's ledger:\n", len(tasks))
	for _, t := range tasks {
		out += fmt.Sprintf("- [%s] %s (%s)", t.ID, t.Description, t.Status)
		if t.ResolvedNote != "" {
			out += fmt.Sprintf(" -- %s", t.ResolvedNote)
		}
		out += "\n"
	}
	return out, nil
}

func (e *superhostToolExecutor) logTask(ctx context.Context, run *automation.AgentRun, arguments json.RawMessage) (string, error) {
	var args struct {
		Description string `json:"description"`
	}
	_ = json.Unmarshal(arguments, &args)
	if args.Description == "" {
		return "", fmt.Errorf("log_task requires a non-empty description argument")
	}
	t, err := e.accountTasks.Log(ctx, run.TenantID, run.ActorID, run.PropertyID, args.Description, run.RunID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Logged task %s: %q", t.ID, t.Description), nil
}

func (e *superhostToolExecutor) resolveTask(ctx context.Context, run *automation.AgentRun, arguments json.RawMessage) (string, error) {
	var args struct {
		TaskID string `json:"task_id"`
		Status string `json:"status"`
		Note   string `json:"note"`
	}
	_ = json.Unmarshal(arguments, &args)
	if args.TaskID == "" {
		return "", fmt.Errorf("resolve_task requires a task_id argument")
	}
	if args.Status == "" {
		args.Status = superhost.AccountTaskStatusDone
	}
	t, err := e.accountTasks.Resolve(ctx, run.TenantID, run.ActorID, args.TaskID, args.Status, args.Note, run.RunID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Task %s marked %s.", t.ID, t.Status), nil
}

// buildPortfolioSummary gives a portfolio-scoped run (see
// ContextAssembler.AssemblePortfolio) a one-line-per-property overview,
// so the model can see every property on the tenant at once and decide
// which one(s) to act on -- the real mechanism behind "manage individual
// properties in parallel" in one thread, rather than requiring a
// separate thread per property.
func (e *superhostToolExecutor) buildPortfolioSummary(pc *superhost.PortfolioContext) string {
	if len(pc.Properties) == 0 {
		return "No properties on this tenant."
	}
	summary := fmt.Sprintf("%d properties on this tenant:\n", len(pc.Properties))
	for _, p := range pc.Properties {
		openTickets := 0
		for _, t := range p.Tickets {
			if t.Status == "open" {
				openTickets++
			}
		}
		activeReservations := 0
		for _, r := range p.Reservations {
			if r.Status == "active" {
				activeReservations++
			}
		}
		summary += fmt.Sprintf("- %s: %s, %d open tickets, %d active reservations\n",
			p.Property.ID, p.Property.State, openTickets, activeReservations)
	}
	summary += "\nCall a read or propose tool with a property_id argument to act on a specific one."
	return summary
}

func (e *superhostToolExecutor) buildOperatingSummary(pc *superhost.PropertyContext) string {
	var summary string
	summary += fmt.Sprintf("Property %s: %s, timezone %s.",
		pc.Property.ID, pc.Property.State, pc.Property.Timezone)

	if len(pc.Reservations) > 0 {
		active := 0
		for _, r := range pc.Reservations {
			if r.Status == "active" {
				active++
			}
		}
		summary += fmt.Sprintf(" %d active reservations.", active)
	} else {
		summary += " No active reservations."
	}

	if len(pc.Tickets) > 0 {
		open := 0
		for _, t := range pc.Tickets {
			if t.Status == "open" {
				open++
			}
		}
		summary += fmt.Sprintf(" %d open tickets.", open)
	} else {
		summary += " 0 open tickets."
	}

	if len(pc.Summaries) > 0 {
		for _, s := range pc.Summaries {
			summary += fmt.Sprintf(" %s: %v.", s.Label, s.Value)
		}
	}

	return summary
}

func (e *superhostToolExecutor) buildReservationChange(pc *superhost.PropertyContext) string {
	if len(pc.Reservations) == 0 {
		return "No recent reservations for this property."
	}
	var out string
	out += fmt.Sprintf("%d reservations in recent window. ", len(pc.Reservations))
	for _, r := range pc.Reservations {
		out += fmt.Sprintf("[%s %s: %s -> %s]. ", r.ID, r.Status,
			r.StartAt.Format(time.RFC3339),
			r.EndAt.Format(time.RFC3339))
	}
	return out
}

func (e *superhostToolExecutor) buildIncidentSummary(pc *superhost.PropertyContext) string {
	if len(pc.Tickets) == 0 {
		return "No tickets/incidents for this property."
	}
	var out string
	out += fmt.Sprintf("%d tickets. ", len(pc.Tickets))
	for _, t := range pc.Tickets {
		out += fmt.Sprintf("[%s %s/%s: %s]. ", t.ID, t.Type, t.Status, t.Reason)
	}
	return out
}
