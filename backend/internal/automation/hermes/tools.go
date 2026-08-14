package hermes

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrToolNotAllowlisted  = errors.New("hermes: tool not in allowlist")
	ErrToolProhibited      = errors.New("hermes: tool exercises prohibited authority")
	ErrDirectMutation      = errors.New("hermes: direct mutation tools do not exist")
	ErrToolVersionMismatch = errors.New("hermes: tool schema version not recognized")
	ErrToolScopeMismatch   = errors.New("hermes: tool tenant/property must derive from run context")
)

const AgentKindHermes = "hermes"

const ToolSchemaVersionCurrent = "v1"

type ToolKind string

const (
	ToolKindRead     ToolKind = "read"
	ToolKindPropose  ToolKind = "propose"
	ToolKindRequest  ToolKind = "request"
	ToolKindRestrict ToolKind = "restricted"
)

type ToolAudience string

const (
	ToolAudienceInternal ToolAudience = "internal"
	ToolAudienceOwner    ToolAudience = "owner"
	ToolAudienceGuest    ToolAudience = "guest"
)

type ToolDefinition struct {
	Name             string       `json:"name"`
	SchemaVersion    string       `json:"schema_version"`
	Kind             ToolKind     `json:"kind"`
	Audience         ToolAudience `json:"audience"`
	Description      string       `json:"description"`
	RequiresApproval bool         `json:"requires_approval"`
	ApprovalKind     string       `json:"approval_kind,omitempty"`
	Idempotent       bool         `json:"idempotent"`
}

func (d ToolDefinition) IsMutation() bool {
	switch d.Kind {
	case ToolKindPropose, ToolKindRequest, ToolKindRestrict:
		return true
	default:
		return false
	}
}

// hermesToolRegistry is the narrow communication and service-recovery tool
// allowlist. Hermes exposes no tool that decides liability, spends money,
// mutates operational truth, or reaches outside approved communication.
var hermesToolRegistry = func() map[string]ToolDefinition {
	reg := []ToolDefinition{
		{
			Name:             "get_communication_context",
			SchemaVersion:    ToolSchemaVersionCurrent,
			Kind:             ToolKindRead,
			Audience:         ToolAudienceInternal,
			Description:      "Read-only narrow communication context assembled from approved facts",
			RequiresApproval: false,
			Idempotent:       true,
		},
		{
			Name:             "draft_approved_template_message",
			SchemaVersion:    ToolSchemaVersionCurrent,
			Kind:             ToolKindPropose,
			Audience:         ToolAudienceInternal,
			Description:      "Draft a message from an approved, localized, single-audience template",
			RequiresApproval: false,
			Idempotent:       true,
		},
		{
			Name:             "draft_free_form_message",
			SchemaVersion:    ToolSchemaVersionCurrent,
			Kind:             ToolKindPropose,
			Audience:         ToolAudienceInternal,
			Description:      "Draft a free-form message that requires human review before delivery",
			RequiresApproval: true,
			ApprovalKind:     "human_review",
			Idempotent:       true,
		},
		{
			Name:             "submit_delivery",
			SchemaVersion:    ToolSchemaVersionCurrent,
			Kind:             ToolKindPropose,
			Audience:         ToolAudienceInternal,
			Description:      "Submit a reviewed, idempotent delivery application action",
			RequiresApproval: true,
			ApprovalKind:     "delivery_review",
			Idempotent:       true,
		},
	}

	m := make(map[string]ToolDefinition, len(reg))
	for _, td := range reg {
		m[td.Name] = td
	}
	return m
}()

// prohibitedAuthorityKeywords are authorities Hermes must never exercise.
// A tool name containing any of these is rejected before allowlist lookup so
// the boundary fails closed even if a registry entry were ever misconfigured.
var prohibitedAuthorityKeywords = []string{
	"liability",
	"adjudicate",
	"settle",
	"waive",
	"refund",
	"reimburse",
	"disburse",
	"pay",
	"charge",
	"transfer",
	"order",
	"spend",
	"sign",
	"certify",
	"file_legal",
	"disclose_access",
	"read_secret",
	"terminate_worker",
	"suspend_worker",
	"reject_worker",
	"delete",
	"hard_delete",
	"purge",
	"wipe",
	"erase",
	"mutate",
	"insert",
	"upsert",
	"update",
	"write",
	"set",
	"put",
	"patch",
}

// IsToolProhibited reports whether a tool name exercises prohibited authority.
// This is how "Hermes cannot decide liability" fails closed: any attempt to
// name a liability, payment or mutation tool is denied regardless of the
// registry contents.
func IsToolProhibited(name string) bool {
	lower := strings.ToLower(name)
	for _, kw := range prohibitedAuthorityKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func LookupTool(name string) (ToolDefinition, error) {
	if IsToolProhibited(name) {
		return ToolDefinition{}, fmt.Errorf("%w: %s", ErrToolProhibited, name)
	}
	def, ok := hermesToolRegistry[name]
	if !ok {
		return ToolDefinition{}, fmt.Errorf("%w: %s", ErrToolNotAllowlisted, name)
	}
	if def.Kind == ToolKindRestrict {
		return ToolDefinition{}, fmt.Errorf("%w: %s", ErrDirectMutation, name)
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
	names := make([]string, 0, len(hermesToolRegistry))
	for name := range hermesToolRegistry {
		names = append(names, name)
	}
	return names
}
