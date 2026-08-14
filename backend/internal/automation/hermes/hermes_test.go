package hermes_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"comfort-curators-backend/internal/automation/hermes"
)

const (
	tenantA = "tenant-A"
	propA   = "prop-A"
)

func approvedFact(audience, kind string) hermes.ApprovedFact {
	return hermes.ApprovedFact{
		Source:      "tickets",
		RecordID:    "rec-" + kind,
		RecordKind:  kind,
		Audience:    audience,
		EffectiveAt: time.Now().UTC().Add(-time.Hour),
	}
}

func newMemService() *hermes.HermesService {
	return hermes.NewService(hermes.NewMemStore())
}

func baseDraftParams() hermes.DraftParams {
	return hermes.DraftParams{
		RunID:      "run-hermes",
		TenantID:   tenantA,
		PropertyID: propA,
		ActorID:    "actor-hermes",
		Audience:   hermes.AudienceOwner,
		Purpose:    "owner exception notice",
		Language:   "en",
		Channel:    "push",
	}
}

func TestCCHER001CommunicationAuthorityIsNarrow(t *testing.T) {
	expected := []string{
		"get_communication_context",
		"draft_approved_template_message",
		"draft_free_form_message",
		"submit_delivery",
	}

	allowed := hermes.AllowedToolNames()
	if len(allowed) != len(expected) {
		t.Fatalf("expected %d tools, got %d: %v", len(expected), len(allowed), allowed)
	}
	allowedMap := make(map[string]bool)
	for _, name := range allowed {
		allowedMap[name] = true
	}
	for _, name := range expected {
		if !allowedMap[name] {
			t.Errorf("missing required tool: %s", name)
		}
	}

	// Hermes must never decide liability, spend, or mutate operational truth:
	// such tools do not exist in the narrow registry.
	absent := []string{
		"determine_liability",
		"adjudicate_liability",
		"assign_liability",
		"waive_owner_fee",
		"refund_guest",
		"pay_vendor",
		"approve_order",
		"update_ticket_status",
		"delete_delivery",
		"disclose_access_secret",
		"sign_contract",
	}
	for _, name := range absent {
		if _, err := hermes.LookupTool(name); err == nil {
			t.Errorf("prohibited tool %q must not be in the Hermes registry", name)
		}
	}

	// Policy fails closed: model text proposing a liability, payment or direct
	// mutation action is denied even before the allowlist is consulted.
	pe := hermes.NewPolicyEngine()
	ctx := hermes.PolicyContext{
		RunID:      "run-narrow",
		TenantID:   tenantA,
		PropertyID: propA,
		ActorID:    "actor-hermes",
		ActorRoles: []string{"hermes"},
	}

	for _, name := range absent {
		input := hermes.ToolCallInput{
			ToolName:  name,
			Version:   "v1",
			CallID:    "narrow-" + name,
			Arguments: json.RawMessage(`{}`),
		}
		dec := pe.Evaluate(ctx, input)
		if dec.Result != hermes.PolicyDenied {
			t.Errorf("tool %q must be denied, got %s", name, dec.Result)
		}
	}

	if !hermes.IsToolProhibited("determine_liability") {
		t.Error("determine_liability must be classified as prohibited authority")
	}
	if hermes.IsToolProhibited("draft_approved_template_message") {
		t.Error("legitimate communication tool must not be classified as prohibited")
	}
}

func TestCCHER001ApprovalPolicyIsEnforced(t *testing.T) {
	svc := newMemService()
	ctx := context.Background()

	t.Run("free-form draft starts under review and cannot deliver unreviewed", func(t *testing.T) {
		params := baseDraftParams()
		params.ReviewPolicy = hermes.ReviewPolicyHumanReview
		params.Subject = "Follow-up on your issue"
		params.Body = "We are resolving the reported water pressure problem."
		params.Facts = []hermes.ApprovedFact{approvedFact(hermes.AudienceOwner, "ticket")}

		draft, err := svc.Draft(ctx, params)
		if err != nil {
			t.Fatalf("draft free-form: %v", err)
		}
		if draft.State != hermes.DraftStateUnderReview {
			t.Fatalf("free-form draft must start under review, got %s", draft.State)
		}
		if draft.ReviewPolicy != hermes.ReviewPolicyHumanReview {
			t.Fatalf("free-form draft must carry human_review policy, got %s", draft.ReviewPolicy)
		}

		_, err = svc.Deliver(ctx, hermes.DeliveryParams{
			TenantID:    tenantA,
			DraftID:     draft.DraftID,
			RecipientID: "owner-1",
			ActorID:     "actor-hermes",
		})
		if !errors.Is(err, hermes.ErrDraftRequiresReview) {
			t.Fatalf("unreviewed free-form draft must not deliver, got %v", err)
		}

		reviewed, err := svc.Review(ctx, tenantA, draft.DraftID, hermes.ReviewParams{
			ReviewerID: "human-reviewer-1",
			Decision:   hermes.ReviewDecisionApproved,
			Reason:     "approved after review",
		})
		if err != nil {
			t.Fatalf("review approve: %v", err)
		}
		if reviewed.State != hermes.DraftStateApproved {
			t.Fatalf("reviewed draft must be approved, got %s", reviewed.State)
		}

		delivery, err := svc.Deliver(ctx, hermes.DeliveryParams{
			TenantID:    tenantA,
			DraftID:     draft.DraftID,
			RecipientID: "owner-1",
			ActorID:     "actor-hermes",
		})
		if err != nil {
			t.Fatalf("deliver after review: %v", err)
		}
		if delivery.Status != hermes.DeliveryStateQueued {
			t.Fatalf("expected queued delivery, got %s", delivery.Status)
		}
	})

	t.Run("approved template draft starts approved", func(t *testing.T) {
		params := baseDraftParams()
		params.ReviewPolicy = hermes.ReviewPolicyApprovedTemplate
		params.TemplateKey = "owner_exception_notice"
		params.Facts = []hermes.ApprovedFact{approvedFact(hermes.AudienceOwner, "ticket")}

		draft, err := svc.Draft(ctx, params)
		if err != nil {
			t.Fatalf("draft template: %v", err)
		}
		if draft.State != hermes.DraftStateApproved {
			t.Fatalf("approved template draft must start approved, got %s", draft.State)
		}
	})

	t.Run("policy requires approval for free-form draft and delivery", func(t *testing.T) {
		pe := hermes.NewPolicyEngine()
		ctx := hermes.PolicyContext{
			RunID:      "run-policy",
			TenantID:   tenantA,
			PropertyID: propA,
			ActorID:    "actor-hermes",
			ActorRoles: []string{"hermes"},
		}

		freeForm := hermes.ToolCallInput{
			ToolName:  "draft_free_form_message",
			Version:   "v1",
			CallID:    "policy-ff",
			Arguments: json.RawMessage(`{}`),
		}
		if dec := pe.Evaluate(ctx, freeForm); dec.Result != hermes.PolicyApprovalRequired {
			t.Errorf("draft_free_form_message must require approval, got %s", dec.Result)
		}

		delivery := hermes.ToolCallInput{
			ToolName:  "submit_delivery",
			Version:   "v1",
			CallID:    "policy-dlv",
			Arguments: json.RawMessage(`{}`),
		}
		if dec := pe.Evaluate(ctx, delivery); dec.Result != hermes.PolicyApprovalRequired {
			t.Errorf("submit_delivery must require approval, got %s", dec.Result)
		}

		template := hermes.ToolCallInput{
			ToolName:  "draft_approved_template_message",
			Version:   "v1",
			CallID:    "policy-tpl",
			Arguments: json.RawMessage(`{}`),
		}
		if dec := pe.Evaluate(ctx, template); dec.Result != hermes.PolicyAllowed {
			t.Errorf("draft_approved_template_message must be allowed, got %s", dec.Result)
		}
	})
}

func TestCCHER001OwnerAndGuestContextsAreSeparated(t *testing.T) {
	svc := newMemService()
	ctx := context.Background()

	t.Run("owner draft accepts owner and public facts", func(t *testing.T) {
		params := baseDraftParams()
		params.ReviewPolicy = hermes.ReviewPolicyApprovedTemplate
		params.TemplateKey = "owner_exception_notice"
		params.Facts = []hermes.ApprovedFact{
			approvedFact(hermes.AudienceOwner, "ticket"),
			approvedFact(hermes.FactAudiencePublic, "property"),
		}
		draft, err := svc.Draft(ctx, params)
		if err != nil {
			t.Fatalf("owner draft: %v", err)
		}
		if draft.Audience != hermes.AudienceOwner {
			t.Fatalf("draft audience must be owner, got %s", draft.Audience)
		}
	})

	t.Run("guest draft accepts guest facts", func(t *testing.T) {
		params := baseDraftParams()
		params.Audience = hermes.AudienceGuest
		params.ReviewPolicy = hermes.ReviewPolicyApprovedTemplate
		params.TemplateKey = "arrival_guidance"
		params.Facts = []hermes.ApprovedFact{approvedFact(hermes.AudienceGuest, "reservation")}
		draft, err := svc.Draft(ctx, params)
		if err != nil {
			t.Fatalf("guest draft: %v", err)
		}
		if draft.Audience != hermes.AudienceGuest {
			t.Fatalf("draft audience must be guest, got %s", draft.Audience)
		}
	})

	t.Run("owner fact cannot feed a guest draft", func(t *testing.T) {
		params := baseDraftParams()
		params.Audience = hermes.AudienceGuest
		params.ReviewPolicy = hermes.ReviewPolicyApprovedTemplate
		params.TemplateKey = "arrival_guidance"
		params.Facts = []hermes.ApprovedFact{approvedFact(hermes.AudienceOwner, "ticket")}
		_, err := svc.Draft(ctx, params)
		if !errors.Is(err, hermes.ErrAudienceMismatch) {
			t.Fatalf("owner fact must not feed a guest draft, got %v", err)
		}
	})

	t.Run("guest fact cannot feed an owner draft", func(t *testing.T) {
		params := baseDraftParams()
		params.ReviewPolicy = hermes.ReviewPolicyApprovedTemplate
		params.TemplateKey = "owner_exception_notice"
		params.Facts = []hermes.ApprovedFact{approvedFact(hermes.AudienceGuest, "reservation")}
		_, err := svc.Draft(ctx, params)
		if !errors.Is(err, hermes.ErrAudienceMismatch) {
			t.Fatalf("guest fact must not feed an owner draft, got %v", err)
		}
	})

	t.Run("one draft cannot mix owner and guest facts", func(t *testing.T) {
		params := baseDraftParams()
		params.ReviewPolicy = hermes.ReviewPolicyApprovedTemplate
		params.TemplateKey = "owner_exception_notice"
		params.Facts = []hermes.ApprovedFact{
			approvedFact(hermes.AudienceOwner, "ticket"),
			approvedFact(hermes.AudienceGuest, "reservation"),
		}
		_, err := svc.Draft(ctx, params)
		if !errors.Is(err, hermes.ErrAudienceMismatch) {
			t.Fatalf("mixed owner/guest facts must be rejected, got %v", err)
		}
	})

	t.Run("public facts feed both audiences", func(t *testing.T) {
		for _, audience := range []string{hermes.AudienceOwner, hermes.AudienceGuest} {
			params := baseDraftParams()
			params.Audience = audience
			params.ReviewPolicy = hermes.ReviewPolicyApprovedTemplate
			params.TemplateKey = "notice"
			params.Facts = []hermes.ApprovedFact{approvedFact(hermes.FactAudiencePublic, "property")}
			if _, err := svc.Draft(ctx, params); err != nil {
				t.Fatalf("public fact must feed %s draft: %v", audience, err)
			}
		}
	})

	t.Run("audience validation fails closed", func(t *testing.T) {
		params := baseDraftParams()
		params.Audience = "vendor"
		params.ReviewPolicy = hermes.ReviewPolicyApprovedTemplate
		params.TemplateKey = "notice"
		params.Facts = []hermes.ApprovedFact{approvedFact(hermes.FactAudiencePublic, "property")}
		_, err := svc.Draft(ctx, params)
		if !errors.Is(err, hermes.ErrInvalidAudience) {
			t.Fatalf("invalid audience must be rejected, got %v", err)
		}
	})
}

func TestCCHER001DeliveryIsIdempotent(t *testing.T) {
	svc := newMemService()
	ctx := context.Background()

	params := baseDraftParams()
	params.ReviewPolicy = hermes.ReviewPolicyApprovedTemplate
	params.TemplateKey = "owner_exception_notice"
	params.Facts = []hermes.ApprovedFact{approvedFact(hermes.AudienceOwner, "ticket")}

	draft, err := svc.Draft(ctx, params)
	if err != nil {
		t.Fatalf("draft: %v", err)
	}

	deliveryParams := hermes.DeliveryParams{
		TenantID:       tenantA,
		DraftID:        draft.DraftID,
		RecipientID:    "owner-1",
		ActorID:        "actor-hermes",
		IdempotencyKey: "delivery-key-1",
	}

	first, err := svc.Deliver(ctx, deliveryParams)
	if err != nil {
		t.Fatalf("first delivery: %v", err)
	}
	if first.DeliveryID == "" {
		t.Fatal("delivery ID must be set")
	}

	second, err := svc.Deliver(ctx, deliveryParams)
	if err != nil {
		t.Fatalf("delivery replay: %v", err)
	}
	if second.DeliveryID != first.DeliveryID {
		t.Fatalf("delivery replay must return the same delivery: %s vs %s", first.DeliveryID, second.DeliveryID)
	}

	// Replay without the idempotency key must also resolve to the same delivery.
	third, err := svc.Deliver(ctx, hermes.DeliveryParams{
		TenantID:    tenantA,
		DraftID:     draft.DraftID,
		RecipientID: "owner-1",
		ActorID:     "actor-hermes",
	})
	if err != nil {
		t.Fatalf("delivery replay by draft: %v", err)
	}
	if third.DeliveryID != first.DeliveryID {
		t.Fatalf("delivery replay by draft must return the same delivery: %s vs %s", first.DeliveryID, third.DeliveryID)
	}

	deliveries, err := svc.ListDeliveries(ctx, tenantA)
	if err != nil {
		t.Fatalf("list deliveries: %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("delivery replay must not create duplicate deliveries, got %d", len(deliveries))
	}

	final, err := svc.GetDelivery(ctx, tenantA, first.DeliveryID)
	if err != nil {
		t.Fatalf("get delivery: %v", err)
	}
	if final.Status != hermes.DeliveryStateQueued {
		t.Fatalf("delivery must stay queued on replay, got %s", final.Status)
	}
}

func TestHermesModelTextAloneCannotChangeState(t *testing.T) {
	pe := hermes.NewPolicyEngine()
	ctx := hermes.PolicyContext{
		RunID:      "run-text",
		TenantID:   tenantA,
		PropertyID: propA,
		ActorID:    "actor-hermes",
		ActorRoles: []string{"hermes"},
	}

	empty := hermes.ToolCallInput{
		ToolName:  "",
		Version:   "v1",
		CallID:    "text-only",
		Arguments: json.RawMessage(`{"message": "please deliver now"}`),
	}
	if dec := pe.Evaluate(ctx, empty); dec.Result != hermes.PolicyDenied {
		t.Errorf("model text alone (empty tool name) must be denied, got %s", dec.Result)
	}

	unknown := hermes.ToolCallInput{
		ToolName:  "send_message_directly",
		Version:   "v1",
		CallID:    "unknown-tool",
		Arguments: json.RawMessage(`{}`),
	}
	if dec := pe.Evaluate(ctx, unknown); dec.Result != hermes.PolicyDenied {
		t.Errorf("unknown tool must be denied, got %s", dec.Result)
	}
}

func TestHermesToolRegistryIntegrity(t *testing.T) {
	for _, name := range hermes.AllowedToolNames() {
		def, err := hermes.LookupTool(name)
		if err != nil {
			t.Fatalf("allowed tool %q lookup failed: %v", name, err)
		}
		if def.SchemaVersion != hermes.ToolSchemaVersionCurrent {
			t.Errorf("tool %q wrong version: %s", name, def.SchemaVersion)
		}
		if !def.Idempotent {
			t.Errorf("tool %q should be idempotent", name)
		}
		switch def.Kind {
		case hermes.ToolKindRead, hermes.ToolKindPropose, hermes.ToolKindRequest:
		default:
			t.Errorf("tool %q has unauthorized kind %q", name, def.Kind)
		}
		if def.Name != name {
			t.Errorf("tool %q name mismatch: %s", name, def.Name)
		}
	}
}

func TestHermesToolVersionValidation(t *testing.T) {
	if err := hermes.ValidateToolVersion("get_communication_context", "v1"); err != nil {
		t.Errorf("valid version should pass: %v", err)
	}
	if err := hermes.ValidateToolVersion("get_communication_context", "v99"); err == nil {
		t.Error("wrong version should be rejected")
	}
	if err := hermes.ValidateToolVersion("nonexistent_tool", "v1"); err == nil {
		t.Error("nonexistent tool should be rejected")
	}
}

func TestHermesToolScopeValidation(t *testing.T) {
	input := hermes.ToolCallInput{
		ToolName:  "draft_approved_template_message",
		Version:   "v1",
		CallID:    "scope-ok",
		Arguments: json.RawMessage(`{"property_id": "prop-A", "tenant_id": "tenant-A"}`),
	}
	if err := input.ValidateScope("tenant-A", "prop-A"); err != nil {
		t.Errorf("matching scope should pass: %v", err)
	}

	crossTenant := hermes.ToolCallInput{
		ToolName:  "draft_approved_template_message",
		Version:   "v1",
		CallID:    "scope-xt",
		Arguments: json.RawMessage(`{"tenant_id": "tenant-B"}`),
	}
	if err := crossTenant.ValidateScope("tenant-A", "prop-A"); err == nil {
		t.Error("cross-tenant should be rejected")
	}

	crossProp := hermes.ToolCallInput{
		ToolName:  "draft_approved_template_message",
		Version:   "v1",
		CallID:    "scope-xp",
		Arguments: json.RawMessage(`{"property_id": "prop-B"}`),
	}
	if err := crossProp.ValidateScope("tenant-A", "prop-A"); err == nil {
		t.Error("cross-property should be rejected")
	}
}

func TestHermesUncertaintyAndException(t *testing.T) {
	pe := hermes.NewPolicyEngine()
	ctx := hermes.PolicyContext{
		RunID:      "run-outage",
		TenantID:   tenantA,
		PropertyID: propA,
		ActorID:    "actor-hermes",
	}

	input := hermes.ToolCallInput{
		ToolName:  "draft_free_form_message",
		Version:   "v1",
		CallID:    "outage-1",
		Arguments: json.RawMessage(`{}`),
	}
	unc := pe.EvaluateUncertainty(ctx, input, "provider unavailable: all models down")
	if unc.Result != hermes.PolicyUncertainty {
		t.Errorf("outage must produce uncertainty, got %s", unc.Result)
	}
	if unc.PolicyVersion != hermes.PolicyVersion {
		t.Error("policy version must be traceable during outage")
	}

	exc := pe.EvaluateException(ctx, input, "provider call timed out after 30s")
	if exc.Result != hermes.PolicyException {
		t.Errorf("timeout must produce exception, got %s", exc.Result)
	}
	if exc.ToolVersion != "v1" {
		t.Errorf("tool version must be traceable in exception, got %s", exc.ToolVersion)
	}
}

func TestHermesReviewRequiresDistinctHuman(t *testing.T) {
	svc := newMemService()
	ctx := context.Background()

	params := baseDraftParams()
	params.ReviewPolicy = hermes.ReviewPolicyHumanReview
	params.Subject = "Subject"
	params.Body = "Body"
	params.Facts = []hermes.ApprovedFact{approvedFact(hermes.AudienceOwner, "ticket")}

	draft, err := svc.Draft(ctx, params)
	if err != nil {
		t.Fatalf("draft: %v", err)
	}

	_, err = svc.Review(ctx, tenantA, draft.DraftID, hermes.ReviewParams{
		ReviewerID: draft.ActorID,
		Decision:   hermes.ReviewDecisionApproved,
	})
	if !errors.Is(err, hermes.ErrReviewerIsRequester) {
		t.Fatalf("requester must not review own draft, got %v", err)
	}

	_, err = svc.Review(ctx, tenantA, draft.DraftID, hermes.ReviewParams{
		ReviewerID: "",
		Decision:   hermes.ReviewDecisionApproved,
	})
	if !errors.Is(err, hermes.ErrReviewerRequired) {
		t.Fatalf("empty reviewer must be rejected, got %v", err)
	}

	_, err = svc.Review(ctx, tenantA, draft.DraftID, hermes.ReviewParams{
		ReviewerID: "reviewer-1",
		Decision:   "maybe",
	})
	if !errors.Is(err, hermes.ErrReviewDecisionRequired) {
		t.Fatalf("invalid decision must be rejected, got %v", err)
	}
}

func TestHermesTemplateDraftDoesNotRequireReview(t *testing.T) {
	svc := newMemService()
	ctx := context.Background()

	params := baseDraftParams()
	params.ReviewPolicy = hermes.ReviewPolicyApprovedTemplate
	params.TemplateKey = "owner_exception_notice"
	params.Facts = []hermes.ApprovedFact{approvedFact(hermes.AudienceOwner, "ticket")}

	draft, err := svc.Draft(ctx, params)
	if err != nil {
		t.Fatalf("draft: %v", err)
	}

	_, err = svc.Review(ctx, tenantA, draft.DraftID, hermes.ReviewParams{
		ReviewerID: "reviewer-1",
		Decision:   hermes.ReviewDecisionApproved,
	})
	if !errors.Is(err, hermes.ErrReviewNotRequired) {
		t.Fatalf("template draft must not require review, got %v", err)
	}

	// Approved template draft can deliver immediately.
	delivery, err := svc.Deliver(ctx, hermes.DeliveryParams{
		TenantID:    tenantA,
		DraftID:     draft.DraftID,
		RecipientID: "owner-1",
		ActorID:     "actor-hermes",
	})
	if err != nil {
		t.Fatalf("template draft delivery: %v", err)
	}
	if delivery.DraftID != draft.DraftID {
		t.Fatalf("delivery must reference the draft, got %s", delivery.DraftID)
	}
}

func TestHermesRejectedDraftCannotDeliver(t *testing.T) {
	svc := newMemService()
	ctx := context.Background()

	params := baseDraftParams()
	params.ReviewPolicy = hermes.ReviewPolicyHumanReview
	params.Subject = "Subject"
	params.Body = "Body"
	params.Facts = []hermes.ApprovedFact{approvedFact(hermes.AudienceOwner, "ticket")}

	draft, err := svc.Draft(ctx, params)
	if err != nil {
		t.Fatalf("draft: %v", err)
	}

	rejected, err := svc.Review(ctx, tenantA, draft.DraftID, hermes.ReviewParams{
		ReviewerID: "reviewer-1",
		Decision:   hermes.ReviewDecisionRejected,
		Reason:     "contains unsupported claim",
	})
	if err != nil {
		t.Fatalf("review reject: %v", err)
	}
	if rejected.State != hermes.DraftStateRejected {
		t.Fatalf("expected rejected, got %s", rejected.State)
	}

	_, err = svc.Deliver(ctx, hermes.DeliveryParams{
		TenantID:    tenantA,
		DraftID:     draft.DraftID,
		RecipientID: "owner-1",
		ActorID:     "actor-hermes",
	})
	if !errors.Is(err, hermes.ErrDraftNotApproved) {
		t.Fatalf("rejected draft must not deliver, got %v", err)
	}
}

func TestHermesDraftRequiresApprovedFacts(t *testing.T) {
	svc := newMemService()
	ctx := context.Background()

	params := baseDraftParams()
	params.ReviewPolicy = hermes.ReviewPolicyApprovedTemplate
	params.TemplateKey = "owner_exception_notice"
	params.Facts = nil
	if _, err := svc.Draft(ctx, params); !errors.Is(err, hermes.ErrFactsRequired) {
		t.Fatalf("draft without facts must be rejected, got %v", err)
	}

	params.Facts = []hermes.ApprovedFact{{
		Source:      "",
		RecordID:    "rec-1",
		RecordKind:  "ticket",
		Audience:    hermes.AudienceOwner,
		EffectiveAt: time.Now().UTC(),
	}}
	if _, err := svc.Draft(ctx, params); !errors.Is(err, hermes.ErrUnapprovedFact) {
		t.Fatalf("incomplete fact must be rejected, got %v", err)
	}

	params.Facts = []hermes.ApprovedFact{{
		Source:      "tickets",
		RecordID:    "rec-1",
		RecordKind:  "ticket",
		Audience:    hermes.AudienceOwner,
		EffectiveAt: time.Now().UTC(),
	}}
	if _, err := svc.Draft(ctx, params); err != nil {
		t.Fatalf("complete approved fact must pass: %v", err)
	}
}
