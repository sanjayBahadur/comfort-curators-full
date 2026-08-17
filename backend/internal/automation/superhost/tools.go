package superhost

import (
	"encoding/json"
	"errors"
	"fmt"
)

var (
	ErrToolNotAllowlisted  = errors.New("superhost: tool not in allowlist")
	ErrToolDirectMutation  = errors.New("superhost: direct mutation tools do not exist")
	ErrToolProhibited      = errors.New("superhost: tool exercises prohibited authority")
	ErrToolVersionMismatch = errors.New("superhost: tool schema version not recognized")
	ErrToolScopeMismatch   = errors.New("superhost: tool tenant/property must derive from run context")
)

const AgentKindSuperhost = "superhost"

type ToolKind string

const (
	ToolKindRead     ToolKind = "read"
	ToolKindPropose  ToolKind = "propose"
	ToolKindRequest  ToolKind = "request"
	ToolKindUIAction ToolKind = "ui_action"
	ToolKindRestrict ToolKind = "restricted"
)

type ToolAudience string

const (
	// ToolAudienceInternal and ToolAudienceUI are role-agnostic: every
	// account, regardless of role, can use tools carrying either of
	// these (see policy.go's audienceAllowed). Everything else is
	// role-scoped -- a tool must list the audience matching an actor's
	// real role (owner/staff/guest, resolved fresh from IAM at policy
	// evaluation time) to be usable by that actor.
	ToolAudienceInternal   ToolAudience = "internal"
	ToolAudienceOwner      ToolAudience = "owner"
	ToolAudienceOperations ToolAudience = "operations"
	ToolAudienceGuest      ToolAudience = "guest"
	ToolAudienceUI         ToolAudience = "ui"
)

const ToolSchemaVersionCurrent = "v1"

type ToolDefinition struct {
	Name          string   `json:"name"`
	SchemaVersion string   `json:"schema_version"`
	Kind          ToolKind `json:"kind"`
	// Audiences: which role(s) can use this tool. A tool naming more than
	// one audience (e.g. propose_restock naming both operations and
	// guest) is intentional -- the same real action can be a legitimate
	// ask from more than one role.
	Audiences        []ToolAudience `json:"audiences"`
	Description      string         `json:"description"`
	RequiresApproval bool           `json:"requires_approval"`
	ApprovalKind     string         `json:"approval_kind,omitempty"`
	Idempotent       bool           `json:"idempotent"`
	// Parameters is the real JSON Schema sent to the model as this tool's
	// function-calling signature. Every tool previously shared one bare
	// `{"type":"object"}` schema with no declared properties or required
	// fields (see app.go's superhostTools, which used to hardcode that
	// for every entry) -- nothing structurally stopped the model from
	// calling propose_inspection_ticket with `{}` and no property_id.
	// Confirmed live: a portfolio-scoped proposal was approved by a real
	// human reviewer and still failed to create a ticket, twice, because
	// the tool call itself never carried a property_id for
	// ExecuteApproved to use. A real, tool-specific schema with
	// `required` lets the model's own provider enforce this before the
	// call is even made, instead of catching it after an approval was
	// already spent on a call that could never succeed.
	Parameters json.RawMessage `json:"-"`
}

func (d ToolDefinition) IsMutation() bool {
	switch d.Kind {
	case ToolKindPropose, ToolKindRequest, ToolKindRestrict:
		return true
	default:
		return false
	}
}

// Real function-calling schemas, one per shape actually consumed server
// side (see tool_executor.go). property_id is always the real id from the
// assembled context (context.property_id for a property-scoped thread,
// context.properties[].property_id per entry for a portfolio-scoped one)
// -- never a name or address, and never omitted, so ExecuteApproved
// always has one to create the real record against.
var (
	readToolSchema = json.RawMessage(`{
		"type": "object",
		"properties": {
			"property_id": {
				"type": "string",
				"description": "The real property_id to scope this read to. Optional only in a portfolio-scoped thread with no argument at all, which returns a one-line-per-property overview instead of one property's detail."
			}
		}
	}`)

	proposeTicketSchema = json.RawMessage(`{
		"type": "object",
		"properties": {
			"property_id": {
				"type": "string",
				"description": "The real property_id this ticket is for, taken from the assembled context. Required, always -- never omit it, even in a portfolio-scoped thread."
			},
			"reason": {
				"type": "string",
				"description": "A short, specific, real reason for this ticket -- what's happening and why it needs this work."
			}
		},
		"required": ["property_id", "reason"]
	}`)

	requestApprovalSchema = json.RawMessage(`{
		"type": "object",
		"properties": {
			"property_id": {
				"type": "string",
				"description": "The real property_id this request concerns, if it's about one specific property."
			},
			"summary": {
				"type": "string",
				"description": "A short, specific summary of what needs approval and why."
			}
		},
		"required": ["summary"]
	}`)

	sendNotificationSchema = json.RawMessage(`{
		"type": "object",
		"properties": {
			"property_id": { "type": "string", "description": "The real property_id this notification concerns, if any." },
			"recipient": { "type": "string", "description": "Who the notification is for, described plainly (their role or name), not an internal id." },
			"message": { "type": "string", "description": "The real, specific content of the notification." }
		},
		"required": ["recipient", "message"]
	}`)

	assembleDocumentPacketSchema = json.RawMessage(`{
		"type": "object",
		"properties": {
			"property_id": { "type": "string", "description": "The real property_id these records belong to." },
			"description": { "type": "string", "description": "Which real records this packet assembles and why." }
		},
		"required": ["property_id", "description"]
	}`)

	escalateExceptionSchema = json.RawMessage(`{
		"type": "object",
		"properties": {
			"property_id": { "type": "string", "description": "The real property_id this exception concerns, if any." },
			"reason": { "type": "string", "description": "A short, specific reason this needs manual review." }
		},
		"required": ["reason"]
	}`)

	uiSurfaceOnlySchema = json.RawMessage(`{
		"type": "object",
		"properties": {
			"surface_id": {
				"type": "string",
				"description": "The exact id from \"Available UI surfaces\" in this turn's message. Never invent one."
			}
		},
		"required": ["surface_id"]
	}`)

	uiSetValueSchema = json.RawMessage(`{
		"type": "object",
		"properties": {
			"surface_id": {
				"type": "string",
				"description": "The exact id from \"Available UI surfaces\" in this turn's message. Never invent one."
			},
			"value": {
				"type": "string",
				"description": "The real, complete value to set -- typed into the field one character at a time in the browser."
			}
		},
		"required": ["surface_id", "value"]
	}`)

	noArgsSchema = json.RawMessage(`{"type": "object", "properties": {}}`)

	logTaskSchema = json.RawMessage(`{
		"type": "object",
		"properties": {
			"description": { "type": "string", "description": "What still needs doing, in your own words." }
		},
		"required": ["description"]
	}`)

	resolveTaskSchema = json.RawMessage(`{
		"type": "object",
		"properties": {
			"task_id": { "type": "string", "description": "The id of one of this account's own logged tasks, from list_my_tasks." },
			"status": { "type": "string", "enum": ["done", "blocked"], "description": "Defaults to done if omitted." },
			"note": { "type": "string", "description": "A brief note on how it landed." }
		},
		"required": ["task_id"]
	}`)
)

var superhostToolRegistry = func() map[string]ToolDefinition {
	reg := []ToolDefinition{
		{
			Name:             "get_property_operating_summary",
			SchemaVersion:    ToolSchemaVersionCurrent,
			Kind:             ToolKindRead,
			Audiences:        []ToolAudience{ToolAudienceInternal},
			Description:      "Read-only property operating summary from assembled context",
			RequiresApproval: false,
			Idempotent:       true,
			Parameters:       readToolSchema,
		},
		{
			Name:             "get_reservation_change",
			SchemaVersion:    ToolSchemaVersionCurrent,
			Kind:             ToolKindRead,
			Audiences:        []ToolAudience{ToolAudienceInternal},
			Description:      "Read reservation changes from assembled context",
			RequiresApproval: false,
			Idempotent:       true,
			Parameters:       readToolSchema,
		},
		{
			Name:             "propose_turnover_ticket",
			SchemaVersion:    ToolSchemaVersionCurrent,
			Kind:             ToolKindPropose,
			Audiences:        []ToolAudience{ToolAudienceOperations},
			Description:      "Propose a new turnover ticket for operations review",
			RequiresApproval: true,
			ApprovalKind:     "operations",
			Idempotent:       true,
			Parameters:       proposeTicketSchema,
		},
		{
			Name:             "propose_inspection_ticket",
			SchemaVersion:    ToolSchemaVersionCurrent,
			Kind:             ToolKindPropose,
			Audiences:        []ToolAudience{ToolAudienceOperations},
			Description:      "Propose an inspection ticket for operations review",
			RequiresApproval: true,
			ApprovalKind:     "operations",
			Idempotent:       true,
			Parameters:       proposeTicketSchema,
		},
		{
			Name:          "propose_restock",
			SchemaVersion: ToolSchemaVersionCurrent,
			Kind:          ToolKindPropose,
			// Also guest: matches the real "Restock an essential" option
			// already on the guest Stay page's own direct ticket form
			// (see GUEST_TICKET_TYPES in stay.tsx) -- a guest asking
			// Superhost for the same thing it can already ask for
			// directly is a legitimate, real use.
			Audiences:        []ToolAudience{ToolAudienceOperations, ToolAudienceGuest},
			Description:      "Propose a stock restock action for inventory review",
			RequiresApproval: true,
			ApprovalKind:     "operations",
			Idempotent:       true,
			Parameters:       proposeTicketSchema,
		},
		{
			Name:          "propose_maintenance_request",
			SchemaVersion: ToolSchemaVersionCurrent,
			Kind:          ToolKindPropose,
			// Also guest: matches GUEST_TICKET_TYPES' "Maintenance
			// request" option, same reasoning as propose_restock above.
			Audiences:        []ToolAudience{ToolAudienceOperations, ToolAudienceGuest},
			Description:      "Propose a maintenance request for triage",
			RequiresApproval: true,
			ApprovalKind:     "operations",
			Idempotent:       true,
			Parameters:       proposeTicketSchema,
		},
		{
			Name:          "propose_incident_report",
			SchemaVersion: ToolSchemaVersionCurrent,
			Kind:          ToolKindPropose,
			// Matches GUEST_TICKET_TYPES' "Something needs attention"
			// option -- a real guest concern with no prior Superhost-
			// callable equivalent before this.
			Audiences:        []ToolAudience{ToolAudienceOperations, ToolAudienceGuest},
			Description:      "Propose an incident report for operations triage",
			RequiresApproval: true,
			ApprovalKind:     "operations",
			Idempotent:       true,
			Parameters:       proposeTicketSchema,
		},
		{
			Name:             "request_owner_approval",
			SchemaVersion:    ToolSchemaVersionCurrent,
			Kind:             ToolKindRequest,
			Audiences:        []ToolAudience{ToolAudienceOwner},
			Description:      "Request owner approval for a proposed action",
			RequiresApproval: true,
			ApprovalKind:     "owner",
			Idempotent:       true,
			Parameters:       requestApprovalSchema,
		},
		{
			Name:             "request_operations_approval",
			SchemaVersion:    ToolSchemaVersionCurrent,
			Kind:             ToolKindRequest,
			Audiences:        []ToolAudience{ToolAudienceOperations},
			Description:      "Request operations team approval for a proposed action",
			RequiresApproval: true,
			ApprovalKind:     "operations",
			Idempotent:       true,
			Parameters:       requestApprovalSchema,
		},
		{
			Name:             "send_approved_notification",
			SchemaVersion:    ToolSchemaVersionCurrent,
			Kind:             ToolKindPropose,
			Audiences:        []ToolAudience{ToolAudienceInternal},
			Description:      "Send an approved notification to a recipient",
			RequiresApproval: true,
			ApprovalKind:     "operations",
			Idempotent:       true,
			Parameters:       sendNotificationSchema,
		},
		{
			Name:             "assemble_document_packet",
			SchemaVersion:    ToolSchemaVersionCurrent,
			Kind:             ToolKindPropose,
			Audiences:        []ToolAudience{ToolAudienceInternal},
			Description:      "Assemble a document packet from specified records",
			RequiresApproval: true,
			ApprovalKind:     "operations",
			Idempotent:       true,
			Parameters:       assembleDocumentPacketSchema,
		},
		{
			Name:             "summarize_incident",
			SchemaVersion:    ToolSchemaVersionCurrent,
			Kind:             ToolKindRead,
			Audiences:        []ToolAudience{ToolAudienceInternal},
			Description:      "Read-only incident summary from assembled context",
			RequiresApproval: false,
			Idempotent:       true,
			Parameters:       readToolSchema,
		},
		{
			Name:             "escalate_exception",
			SchemaVersion:    ToolSchemaVersionCurrent,
			Kind:             ToolKindPropose,
			Audiences:        []ToolAudience{ToolAudienceOperations},
			Description:      "Escalate an exception for manual review",
			RequiresApproval: true,
			ApprovalKind:     "operations",
			Idempotent:       true,
			Parameters:       escalateExceptionSchema,
		},
		{
			Name:             "ui_focus",
			SchemaVersion:    ToolSchemaVersionCurrent,
			Kind:             ToolKindUIAction,
			Audiences:        []ToolAudience{ToolAudienceUI},
			Description:      "Focus a registered UI element in the user's current browser view",
			RequiresApproval: false,
			Idempotent:       false,
			Parameters:       uiSurfaceOnlySchema,
		},
		{
			Name:             "ui_set_value",
			SchemaVersion:    ToolSchemaVersionCurrent,
			Kind:             ToolKindUIAction,
			Audiences:        []ToolAudience{ToolAudienceUI},
			Description:      "Set the value of a registered UI form element in the user's current browser view",
			RequiresApproval: false,
			Idempotent:       false,
			Parameters:       uiSetValueSchema,
		},
		{
			Name:             "ui_click",
			SchemaVersion:    ToolSchemaVersionCurrent,
			Kind:             ToolKindUIAction,
			Audiences:        []ToolAudience{ToolAudienceUI},
			Description:      "Click a registered UI element in the user's current browser view",
			RequiresApproval: false,
			Idempotent:       false,
			Parameters:       uiSurfaceOnlySchema,
		},
		{
			Name:             "ui_scroll_to",
			SchemaVersion:    ToolSchemaVersionCurrent,
			Kind:             ToolKindUIAction,
			Audiences:        []ToolAudience{ToolAudienceUI},
			Description:      "Scroll a registered UI element into view in the user's current browser view",
			RequiresApproval: false,
			Idempotent:       false,
			Parameters:       uiSurfaceOnlySchema,
		},
		{
			Name:             "ui_open_panel",
			SchemaVersion:    ToolSchemaVersionCurrent,
			Kind:             ToolKindUIAction,
			Audiences:        []ToolAudience{ToolAudienceUI},
			Description:      "Open a registered UI panel in the user's current browser view",
			RequiresApproval: false,
			Idempotent:       false,
			Parameters:       uiSurfaceOnlySchema,
		},
		{
			Name:             "list_my_tasks",
			SchemaVersion:    ToolSchemaVersionCurrent,
			Kind:             ToolKindRead,
			Audiences:        []ToolAudience{ToolAudienceInternal},
			Description:      "List this account's own Superhost task ledger: open items and recently resolved ones",
			RequiresApproval: false,
			Idempotent:       true,
			Parameters:       noArgsSchema,
		},
		{
			Name:          "log_task",
			SchemaVersion: ToolSchemaVersionCurrent,
			Kind:          ToolKindPropose,
			Audiences:     []ToolAudience{ToolAudienceInternal},
			Description:   "Note something this account's Superhost still needs to do -- its own scratchpad, not a real business record",
			// Not gated behind human approval: this only writes to
			// Superhost's own per-account memory (see
			// migrations/005_superhost_account_tasks.sql), never real
			// business state, so there's nothing here for a human to
			// approve or deny.
			RequiresApproval: false,
			Idempotent:       false,
			Parameters:       logTaskSchema,
		},
		{
			Name:             "resolve_task",
			SchemaVersion:    ToolSchemaVersionCurrent,
			Kind:             ToolKindPropose,
			Audiences:        []ToolAudience{ToolAudienceInternal},
			Description:      "Mark one of this account's own logged tasks done or blocked",
			RequiresApproval: false,
			// Genuinely idempotent: it's an UPDATE keyed by task_id, so
			// calling it again with the same status just re-sets the same
			// state rather than creating anything new.
			Idempotent: true,
			Parameters: resolveTaskSchema,
		},
	}

	m := make(map[string]ToolDefinition, len(reg))
	for _, td := range reg {
		m[td.Name] = td
	}
	return m
}()

var prohibitedAuthorityKeywords = []string{
	"direct storage mutation",
	"unapproved purchase or order",
	"self approval",
	"payment, refund, journal, or bank detail mutation",
	"legal signature, certification, or filing",
	"unrestricted access-secret disclosure",
	"worker rejection, suspension, termination, wage, or contract change",
	"high-risk work verification",
	"hard deletion or evidence rewrite",
}

var prohibitedToolNamePrefixes = []string{
	"delete_", "hard_delete_", "purge_", "wipe_", "erase_",
	"pay_", "refund_", "charge_", "transfer_", "disburse_",
	"sign_", "certify_", "file_legal_",
	"disclose_access_", "read_secret_", "export_secret_",
	"terminate_worker_", "suspend_worker_", "reject_worker_",
	"create_order_", "approve_order_", "place_order_",
	"mutate_", "write_", "update_", "insert_", "upsert_",
	"set_", "put_", "patch_",
}

func IsToolProhibited(name string) bool {
	for _, prefix := range prohibitedToolNamePrefixes {
		if len(name) >= len(prefix) && name[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

func LookupTool(name string) (ToolDefinition, error) {
	if IsToolProhibited(name) {
		return ToolDefinition{}, fmt.Errorf("%w: %s", ErrToolProhibited, name)
	}
	def, ok := superhostToolRegistry[name]
	if !ok {
		return ToolDefinition{}, fmt.Errorf("%w: %s", ErrToolNotAllowlisted, name)
	}
	if def.Kind == ToolKindRestrict {
		return ToolDefinition{}, fmt.Errorf("%w: %s", ErrToolDirectMutation, name)
	}
	return def, nil
}

func ValidateToolVersion(name, version string) error {
	def, err := LookupTool(name)
	if err != nil {
		return err
	}
	if def.SchemaVersion != version {
		return fmt.Errorf("%w: %s expects %s, got %s", ErrToolVersionMismatch, name, def.SchemaVersion, version)
	}
	return nil
}

func AllowedToolNames() []string {
	names := make([]string, 0, len(superhostToolRegistry))
	for name := range superhostToolRegistry {
		names = append(names, name)
	}
	return names
}

type ToolCallInput struct {
	ToolName       string          `json:"tool_name"`
	Version        string          `json:"version"`
	Arguments      json.RawMessage `json:"arguments"`
	CallID         string          `json:"call_id"`
	IdempotencyKey string          `json:"idempotency_key,omitempty"`
}

func (in ToolCallInput) ValidateScope(tenantID, propertyID string) error {
	var args map[string]any
	if err := json.Unmarshal(in.Arguments, &args); err != nil {
		return fmt.Errorf("superhost: invalid tool arguments: %w", err)
	}

	if t, ok := args["tenant_id"]; ok {
		if ts, ok := t.(string); ok && ts != "" && ts != tenantID {
			return fmt.Errorf("%w: tenant_id in arguments (%s) must match run context (%s)", ErrToolScopeMismatch, ts, tenantID)
		}
	}
	// propertyID == "" means this run is portfolio-scoped (see
	// ContextAssembler.AssemblePortfolio), not locked to one property --
	// a tool call naming any property_id is expected there, not a scope
	// violation. It still isn't a blank check: the tool executor (which
	// has real DB access, unlike this pure function) verifies the named
	// property actually belongs to this tenant before anything executes.
	if propertyID != "" {
		if p, ok := args["property_id"]; ok {
			if ps, ok := p.(string); ok && ps != "" && ps != propertyID {
				return fmt.Errorf("%w: property_id in arguments (%s) must match run context (%s)", ErrToolScopeMismatch, ps, propertyID)
			}
		}
	}

	return nil
}
