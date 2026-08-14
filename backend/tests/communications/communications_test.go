package communications_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"comfort-curators-backend/internal/communications"
	"comfort-curators-backend/internal/platform/audit"

	"github.com/jackc/pgx/v5/pgxpool"
)

func communicationsPostgresAvailable() bool {
	host := os.Getenv("CC_DB_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("CC_DB_PORT")
	if port == "" {
		port = "5432"
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func communicationsDBConnString() string {
	host := os.Getenv("CC_DB_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("CC_DB_PORT")
	if port == "" {
		port = "5432"
	}
	user := os.Getenv("CC_DB_USER")
	if user == "" {
		user = "ccuser"
	}
	pass := os.Getenv("CC_DB_PASS")
	if pass == "" {
		pass = "ccpass"
	}
	name := os.Getenv("CC_DB_NAME")
	if name == "" {
		name = "comfort_curators"
	}
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, pass, host, port, name)
}

func communicationsPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if !communicationsPostgresAvailable() {
		t.Skip("PostgreSQL not available for communications integration test")
	}
	pool, err := pgxpool.New(context.Background(), communicationsDBConnString())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := communications.EnsureSchema(context.Background(), pool); err != nil {
		t.Fatalf("ensure communications schema: %v", err)
	}
	if err := audit.EnsureSchema(context.Background(), pool); err != nil {
		t.Fatalf("ensure audit schema: %v", err)
	}
	for _, table := range []string{
		"conversation_links",
		"deliveries",
		"communication_reviews",
		"communication_drafts",
		"communication_preferences",
		"message_template_versions",
		"message_templates",
		"audit_events",
	} {
		if _, err := pool.Exec(context.Background(), "TRUNCATE "+table+" CASCADE"); err != nil {
			t.Fatalf("truncate %s: %v", table, err)
		}
	}
	return pool
}

func newCommunicationsService(t *testing.T) *communications.CommunicationsService {
	t.Helper()
	pool := communicationsPool(t)
	return communications.NewCommunicationsService(pool).WithAudit(audit.NewAuditStore(pool))
}

func createTemplate(t *testing.T, svc *communications.CommunicationsService, tenantID, key, audience, class string) *communications.MessageTemplate {
	t.Helper()
	tmpl, err := svc.CreateTemplate(context.Background(), tenantID, communications.TemplateParams{
		TemplateKey:  key,
		Audience:     audience,
		ConsentClass: class,
		Channel:      communications.ChannelPush,
		Severity:     communications.SeverityNormal,
	}, "actor-comm-1")
	if err != nil {
		t.Fatalf("create template %s: %v", key, err)
	}
	return tmpl
}

func addVersion(t *testing.T, svc *communications.CommunicationsService, tenantID, templateID, language, subject, body string) {
	t.Helper()
	if _, err := svc.AddTemplateVersion(context.Background(), tenantID, templateID, communications.TemplateVersionParams{
		Language: language,
		Subject:  subject,
		Body:     body,
	}, "actor-comm-1"); err != nil {
		t.Fatalf("add template version: %v", err)
	}
}

func TestCommunicationsFreeFormAIMessageRequiresReview(t *testing.T) {
	svc := newCommunicationsService(t)
	ctx := context.Background()
	tenantID := "tenant-com-ai"

	ai, err := svc.CreateDraft(ctx, tenantID, communications.DraftParams{
		Audience:     communications.AudienceGuest,
		RecipientID:  "guest-ai-1",
		Source:       communications.SourceAI,
		ConsentClass: communications.ConsentClassTransactional,
		Subject:      "About your upcoming stay",
		Body:         "We noticed your travel plans changed. Reply anytime.",
	}, "model-stub")
	if err != nil {
		t.Fatalf("create AI draft: %v", err)
	}
	if ai.Status != communications.DraftStatusUnderReview {
		t.Fatalf("free-form AI draft must start under review, got %q", ai.Status)
	}
	if !ai.RequiresReview {
		t.Fatal("free-form AI draft must be flagged requires_review")
	}

	// An unreviewed free-form AI message can never be delivered.
	if _, err := svc.Deliver(ctx, tenantID, ai.ID, "actor-comm-1"); !errors.Is(err, communications.ErrDraftRequiresReview) {
		t.Fatalf("delivering an unreviewed AI message must fail with ErrDraftRequiresReview, got %v", err)
	}

	// A rejected free-form AI message stays rejected and undeliverable.
	rejected, err := svc.CreateDraft(ctx, tenantID, communications.DraftParams{
		Audience:     communications.AudienceGuest,
		RecipientID:  "guest-ai-2",
		Source:       communications.SourceAI,
		ConsentClass: communications.ConsentClassMarketing,
		Subject:      "Limited time offer",
		Body:         "Get 20% off your next stay.",
	}, "model-stub")
	if err != nil {
		t.Fatalf("create second AI draft: %v", err)
	}
	rejectedDraft, err := svc.ReviewDraft(ctx, tenantID, rejected.ID, communications.ReviewParams{
		ReviewerID: "human-reviewer-1",
		Decision:   communications.ReviewDecisionRejected,
		Reason:     "not in brand voice",
	}, "actor-comm-1")
	if err != nil {
		t.Fatalf("reject AI draft: %v", err)
	}
	if rejectedDraft.Status != communications.DraftStatusRejected {
		t.Fatalf("rejected draft must be rejected, got %q", rejectedDraft.Status)
	}
	if _, err := svc.Deliver(ctx, tenantID, rejected.ID, "actor-comm-1"); !errors.Is(err, communications.ErrDraftNotApproved) {
		t.Fatalf("delivering a rejected AI message must fail with ErrDraftNotApproved, got %v", err)
	}

	// A human-approved free-form AI message becomes deliverable.
	approved, err := svc.ReviewDraft(ctx, tenantID, ai.ID, communications.ReviewParams{
		ReviewerID: "human-reviewer-1",
		Decision:   communications.ReviewDecisionApproved,
		Reason:     "clear and within policy",
	}, "actor-comm-1")
	if err != nil {
		t.Fatalf("approve AI draft: %v", err)
	}
	if approved.Status != communications.DraftStatusApproved {
		t.Fatalf("approved draft must be approved, got %q", approved.Status)
	}

	delivery, err := svc.Deliver(ctx, tenantID, ai.ID, "actor-comm-1")
	if err != nil {
		t.Fatalf("deliver approved AI draft: %v", err)
	}
	if delivery.Status != communications.DeliveryStatusQueued {
		t.Fatalf("delivery must start queued, got %q", delivery.Status)
	}

	// A draft can only be delivered once.
	if _, err := svc.Deliver(ctx, tenantID, ai.ID, "actor-comm-1"); !errors.Is(err, communications.ErrDraftAlreadyDelivered) {
		t.Fatalf("re-delivering a delivered draft must fail with ErrDraftAlreadyDelivered, got %v", err)
	}

	// Curated template drafts do not require review.
	tmpl := createTemplate(t, svc, tenantID, "stay_confirmation", communications.AudienceGuest, communications.ConsentClassTransactional)
	addVersion(t, svc, tenantID, tmpl.ID, communications.LanguageEnglish, "Stay confirmed", "Your stay is confirmed.")
	tmplDraft, err := svc.CreateDraft(ctx, tenantID, communications.DraftParams{
		Audience:    communications.AudienceGuest,
		RecipientID: "guest-tpl-1",
		Source:      communications.SourceTemplate,
		TemplateKey: "stay_confirmation",
	}, "actor-comm-1")
	if err != nil {
		t.Fatalf("create template draft: %v", err)
	}
	if tmplDraft.RequiresReview || tmplDraft.Status != communications.DraftStatusApproved {
		t.Fatalf("template draft must be pre-approved, got requires_review=%v status=%q", tmplDraft.RequiresReview, tmplDraft.Status)
	}
	if _, err := svc.ReviewDraft(ctx, tenantID, tmplDraft.ID, communications.ReviewParams{
		ReviewerID: "human-reviewer-1",
		Decision:   communications.ReviewDecisionApproved,
	}, "actor-comm-1"); !errors.Is(err, communications.ErrReviewNotRequired) {
		t.Fatalf("reviewing a curated template draft must fail with ErrReviewNotRequired, got %v", err)
	}
}

func TestCommunicationsSecureLinkExpiresAndRejectsReplay(t *testing.T) {
	pool := communicationsPool(t)
	svc := communications.NewCommunicationsService(pool).WithAudit(audit.NewAuditStore(pool))
	ctx := context.Background()
	tenantID := "tenant-com-link"

	link, token, err := svc.CreateSecureLink(ctx, tenantID, communications.SecureLinkParams{
		PropertyID:  "prop-link-1",
		Audience:    communications.AudienceGuest,
		RecipientID: "guest-link-1",
		Purpose:     "stay",
		ExpiresAt:   time.Now().UTC().Add(2 * time.Hour),
	}, "actor-comm-1")
	if err != nil {
		t.Fatalf("create secure link: %v", err)
	}
	if token == "" || link.TokenTail == "" || link.TokenTail == token {
		t.Fatal("secure link must return a raw token and a distinct display tail")
	}
	if link.Status != communications.LinkStatusActive {
		t.Fatalf("fresh link must be active, got %q", link.Status)
	}

	// First redemption succeeds.
	redeemed, err := svc.RedeemSecureLink(ctx, token)
	if err != nil {
		t.Fatalf("redeem secure link: %v", err)
	}
	if redeemed.UsedAt == nil || redeemed.Status != communications.LinkStatusUsed {
		t.Fatalf("redeemed link must be used, got status=%q used_at=%v", redeemed.Status, redeemed.UsedAt)
	}

	// Replaying the same token is rejected.
	if _, err := svc.RedeemSecureLink(ctx, token); !errors.Is(err, communications.ErrLinkAlreadyUsed) {
		t.Fatalf("replaying a used secure link must fail with ErrLinkAlreadyUsed, got %v", err)
	}

	// An unknown token is rejected.
	if _, err := svc.RedeemSecureLink(ctx, "not-a-real-token"); !errors.Is(err, communications.ErrLinkNotFound) {
		t.Fatalf("unknown token must fail with ErrLinkNotFound, got %v", err)
	}

	// A link cannot be created with a past expiry.
	if _, _, err := svc.CreateSecureLink(ctx, tenantID, communications.SecureLinkParams{
		PropertyID:  "prop-link-2",
		Audience:    communications.AudienceGuest,
		RecipientID: "guest-link-2",
		ExpiresAt:   time.Now().UTC().Add(-1 * time.Hour),
	}, "actor-comm-1"); !errors.Is(err, communications.ErrInvalidSecureLink) {
		t.Fatalf("creating an already-expired link must fail with ErrInvalidSecureLink, got %v", err)
	}

	// An expired link rejects redemption. Expiry is moved into the past on the
	// stored row so the classification is deterministic.
	expiring, token2, err := svc.CreateSecureLink(ctx, tenantID, communications.SecureLinkParams{
		PropertyID:  "prop-link-3",
		Audience:    communications.AudienceGuest,
		RecipientID: "guest-link-3",
		Purpose:     "stay",
		ExpiresAt:   time.Now().UTC().Add(24 * time.Hour),
	}, "actor-comm-1")
	if err != nil {
		t.Fatalf("create expiring link: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE conversation_links SET expires_at = NOW() - interval '1 hour' WHERE id=$1`, expiring.ID); err != nil {
		t.Fatalf("expire stored link: %v", err)
	}
	if _, err := svc.RedeemSecureLink(ctx, token2); !errors.Is(err, communications.ErrLinkExpired) {
		t.Fatalf("redeeming an expired secure link must fail with ErrLinkExpired, got %v", err)
	}

	// Revoked links reject redemption even before expiry.
	revokable, token3, err := svc.CreateSecureLink(ctx, tenantID, communications.SecureLinkParams{
		PropertyID:  "prop-link-4",
		Audience:    communications.AudienceOwner,
		RecipientID: "owner-link-1",
		Purpose:     "stay",
		ExpiresAt:   time.Now().UTC().Add(24 * time.Hour),
	}, "actor-comm-1")
	if err != nil {
		t.Fatalf("create revocable link: %v", err)
	}
	if _, err := svc.RevokeSecureLink(ctx, tenantID, revokable.ID); err != nil {
		t.Fatalf("revoke secure link: %v", err)
	}
	if _, err := svc.RedeemSecureLink(ctx, token3); !errors.Is(err, communications.ErrLinkRevoked) {
		t.Fatalf("redeeming a revoked secure link must fail with ErrLinkRevoked, got %v", err)
	}
}

func TestCommunicationsOwnerAndGuestAudiencesRemainSeparate(t *testing.T) {
	svc := newCommunicationsService(t)
	ctx := context.Background()
	tenantID := "tenant-com-aud"

	ownerTmpl := createTemplate(t, svc, tenantID, "owner_receipt", communications.AudienceOwner, communications.ConsentClassTransactional)
	addVersion(t, svc, tenantID, ownerTmpl.ID, communications.LanguageEnglish, "Your receipt", "Invoice attached.")
	guestTmpl := createTemplate(t, svc, tenantID, "guest_checkout", communications.AudienceGuest, communications.ConsentClassTransactional)
	addVersion(t, svc, tenantID, guestTmpl.ID, communications.LanguageEnglish, "Checkout guide", "Please leave keys on the table.")

	// An owner template cannot be used to draft a guest message and vice versa.
	if _, err := svc.CreateDraft(ctx, tenantID, communications.DraftParams{
		Audience:    communications.AudienceGuest,
		RecipientID: "guest-aud-1",
		Source:      communications.SourceTemplate,
		TemplateKey: "owner_receipt",
	}, "actor-comm-1"); !errors.Is(err, communications.ErrAudienceMismatch) {
		t.Fatalf("owner template into a guest draft must fail with ErrAudienceMismatch, got %v", err)
	}
	if _, err := svc.CreateDraft(ctx, tenantID, communications.DraftParams{
		Audience:    communications.AudienceOwner,
		RecipientID: "owner-aud-1",
		Source:      communications.SourceTemplate,
		TemplateKey: "guest_checkout",
	}, "actor-comm-1"); !errors.Is(err, communications.ErrAudienceMismatch) {
		t.Fatalf("guest template into an owner draft must fail with ErrAudienceMismatch, got %v", err)
	}

	// Listings filter by audience.
	ownerTemplates, err := svc.ListTemplates(ctx, tenantID, communications.AudienceOwner)
	if err != nil {
		t.Fatalf("list owner templates: %v", err)
	}
	if len(ownerTemplates) != 1 || ownerTemplates[0].TemplateKey != "owner_receipt" {
		t.Fatalf("owner listing must contain only the owner template, got %+v", ownerTemplates)
	}

	// Owner marketing consent never leaks to guest audiences.
	owner := "owner-aud-2"
	guest := "guest-aud-2"
	if _, err := svc.SetPreferences(ctx, tenantID, communications.PreferencesParams{
		RecipientID:          owner,
		Audience:             communications.AudienceOwner,
		ConsentTransactional: true,
		ConsentUrgent:        true,
		ConsentMarketing:     true,
		ConsentSponsored:     false,
	}, "actor-comm-1"); err != nil {
		t.Fatalf("set owner preferences: %v", err)
	}

	ownerPrefs, err := svc.GetPreferences(ctx, tenantID, owner, communications.AudienceOwner)
	if err != nil {
		t.Fatalf("get owner preferences: %v", err)
	}
	if !ownerPrefs.ConsentMarketing {
		t.Fatal("owner marketing consent must be true")
	}
	guestPrefs, err := svc.GetPreferences(ctx, tenantID, guest, communications.AudienceGuest)
	if err != nil {
		t.Fatalf("get guest preferences: %v", err)
	}
	if guestPrefs.ConsentMarketing {
		t.Fatal("guest marketing consent must stay separate and default to false")
	}

	// A marketing template draft delivers to the consenting owner but is
	// blocked for the non-consenting guest.
	marketingTmpl := createTemplate(t, svc, tenantID, "owner_upsell", communications.AudienceOwner, communications.ConsentClassMarketing)
	addVersion(t, svc, tenantID, marketingTmpl.ID, communications.LanguageEnglish, "Exclusive offer", "Upgrade your service.")
	ownerDraft, err := svc.CreateDraft(ctx, tenantID, communications.DraftParams{
		Audience:    communications.AudienceOwner,
		RecipientID: owner,
		Source:      communications.SourceTemplate,
		TemplateKey: "owner_upsell",
	}, "actor-comm-1")
	if err != nil {
		t.Fatalf("create owner marketing draft: %v", err)
	}
	if _, err := svc.Deliver(ctx, tenantID, ownerDraft.ID, "actor-comm-1"); err != nil {
		t.Fatalf("owner with marketing consent must receive the marketing message: %v", err)
	}

	guestMarketingTmpl := createTemplate(t, svc, tenantID, "guest_upsell", communications.AudienceGuest, communications.ConsentClassMarketing)
	addVersion(t, svc, tenantID, guestMarketingTmpl.ID, communications.LanguageEnglish, "Special stay offer", "Book again and save.")
	guestDraft, err := svc.CreateDraft(ctx, tenantID, communications.DraftParams{
		Audience:    communications.AudienceGuest,
		RecipientID: guest,
		Source:      communications.SourceTemplate,
		TemplateKey: "guest_upsell",
	}, "actor-comm-1")
	if err != nil {
		t.Fatalf("create guest marketing draft: %v", err)
	}
	if _, err := svc.Deliver(ctx, tenantID, guestDraft.ID, "actor-comm-1"); !errors.Is(err, communications.ErrConsentNotGranted) {
		t.Fatalf("guest without marketing consent must be blocked with ErrConsentNotGranted, got %v", err)
	}
}

func TestCommunicationsTemplatesVersionedAndLocalized(t *testing.T) {
	svc := newCommunicationsService(t)
	ctx := context.Background()
	tenantID := "tenant-com-tpl"

	tmpl := createTemplate(t, svc, tenantID, "welcome", communications.AudienceGuest, communications.ConsentClassTransactional)
	addVersion(t, svc, tenantID, tmpl.ID, communications.LanguageEnglish, "Welcome", "Welcome to your stay.")
	addVersion(t, svc, tenantID, tmpl.ID, communications.LanguageHindi, "स्वागत है", "आपके प्रवास में स्वागत है।")

	hi, err := svc.ResolveTemplateContent(ctx, tenantID, "welcome", communications.LanguageHindi)
	if err != nil {
		t.Fatalf("resolve Hindi template: %v", err)
	}
	if hi.Language != communications.LanguageHindi || hi.Subject != "स्वागत है" {
		t.Fatalf("Hindi resolution mismatch: lang=%s subject=%q", hi.Language, hi.Subject)
	}

	en, err := svc.ResolveTemplateContent(ctx, tenantID, "welcome", communications.LanguageEnglish)
	if err != nil {
		t.Fatalf("resolve English template: %v", err)
	}
	if en.Language != communications.LanguageEnglish || en.Subject != "Welcome" {
		t.Fatalf("English resolution mismatch: lang=%s subject=%q", en.Language, en.Subject)
	}

	// A new version supersedes the older English version.
	addVersion(t, svc, tenantID, tmpl.ID, communications.LanguageEnglish, "Welcome (v3)", "Welcome to your stay, again.")
	en2, err := svc.ResolveTemplateContent(ctx, tenantID, "welcome", communications.LanguageEnglish)
	if err != nil {
		t.Fatalf("resolve superseded English template: %v", err)
	}
	if en2.Subject != "Welcome (v3)" || en2.Version != 3 {
		t.Fatalf("latest English version must win, got subject=%q version=%d", en2.Subject, en2.Version)
	}

	// A Hindi draft resolves the Hindi version of the template.
	draft, err := svc.CreateDraft(ctx, tenantID, communications.DraftParams{
		Audience:    communications.AudienceGuest,
		RecipientID: "guest-hi-1",
		Source:      communications.SourceTemplate,
		TemplateKey: "welcome",
		Language:    communications.LanguageHindi,
	}, "actor-comm-1")
	if err != nil {
		t.Fatalf("create Hindi template draft: %v", err)
	}
	if draft.Subject != "स्वागत है" {
		t.Fatalf("Hindi draft must carry Hindi content, got subject=%q", draft.Subject)
	}

	// Fallback to English when the requested language has no version.
	if _, err := svc.CreateTemplate(ctx, tenantID, communications.TemplateParams{
		TemplateKey:  "en_only",
		Audience:     communications.AudienceOwner,
		ConsentClass: communications.ConsentClassTransactional,
	}, "actor-comm-1"); err != nil {
		t.Fatalf("create en-only template: %v", err)
	}
	enOnlyTmpl, err := svc.GetTemplateByKey(ctx, tenantID, "en_only")
	if err != nil {
		t.Fatalf("get en-only template: %v", err)
	}
	addVersion(t, svc, tenantID, enOnlyTmpl.ID, communications.LanguageEnglish, "English only", "This template has no Hindi version.")
	fallback, err := svc.ResolveTemplateContent(ctx, tenantID, "en_only", communications.LanguageHindi)
	if err != nil {
		t.Fatalf("Hindi fallback must resolve to English: %v", err)
	}
	if fallback.Language != communications.LanguageEnglish {
		t.Fatalf("fallback must select English, got %q", fallback.Language)
	}
}

func TestCommunicationsConsentAndQuietHoursEnforced(t *testing.T) {
	svc := newCommunicationsService(t)
	ctx := context.Background()
	tenantID := "tenant-com-pref"

	recipient := "owner-pref-1"
	// No marketing or sponsored consent, quiet hours covering all day.
	if _, err := svc.SetPreferences(ctx, tenantID, communications.PreferencesParams{
		RecipientID:           recipient,
		Audience:              communications.AudienceOwner,
		ConsentTransactional:  true,
		ConsentUrgent:         true,
		ConsentMarketing:      false,
		ConsentSponsored:      false,
		QuietHoursStartMinute: 0,
		QuietHoursEndMinute:   1439,
		EscalationContacts:    []string{"owner-mobile", "owner-partner"},
	}, "actor-comm-1"); err != nil {
		t.Fatalf("set preferences: %v", err)
	}

	prefs, err := svc.GetPreferences(ctx, tenantID, recipient, communications.AudienceOwner)
	if err != nil {
		t.Fatalf("get preferences: %v", err)
	}
	if len(prefs.EscalationContacts) != 2 {
		t.Fatalf("escalation contacts must be stored, got %v", prefs.EscalationContacts)
	}
	if prefs.QuietHoursStartMinute != 0 || prefs.QuietHoursEndMinute != 1439 {
		t.Fatalf("quiet hours must be stored, got %d-%d", prefs.QuietHoursStartMinute, prefs.QuietHoursEndMinute)
	}
	if !communications.IsWithinQuietHours(time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC), prefs.QuietHoursStartMinute, prefs.QuietHoursEndMinute) {
		t.Fatal("all-day quiet hours must be active at noon")
	}

	// A marketing-class message is blocked by missing consent before quiet hours.
	marketingTmpl := createTemplate(t, svc, tenantID, "pref_marketing", communications.AudienceOwner, communications.ConsentClassMarketing)
	addVersion(t, svc, tenantID, marketingTmpl.ID, communications.LanguageEnglish, "Offer", "New promotion.")
	marketingDraft, err := svc.CreateDraft(ctx, tenantID, communications.DraftParams{
		Audience:    communications.AudienceOwner,
		RecipientID: recipient,
		Source:      communications.SourceTemplate,
		TemplateKey: "pref_marketing",
	}, "actor-comm-1")
	if err != nil {
		t.Fatalf("create marketing draft: %v", err)
	}
	if _, err := svc.DeliverAt(ctx, tenantID, marketingDraft.ID, "actor-comm-1", time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)); !errors.Is(err, communications.ErrConsentNotGranted) {
		t.Fatalf("marketing without consent must fail with ErrConsentNotGranted, got %v", err)
	}

	// A transactional message is consented but blocked by quiet hours.
	transactionalTmpl := createTemplate(t, svc, tenantID, "pref_transactional", communications.AudienceOwner, communications.ConsentClassTransactional)
	addVersion(t, svc, tenantID, transactionalTmpl.ID, communications.LanguageEnglish, "Invoice", "Your invoice is ready.")
	transactionalDraft, err := svc.CreateDraft(ctx, tenantID, communications.DraftParams{
		Audience:    communications.AudienceOwner,
		RecipientID: recipient,
		Source:      communications.SourceTemplate,
		TemplateKey: "pref_transactional",
	}, "actor-comm-1")
	if err != nil {
		t.Fatalf("create transactional draft: %v", err)
	}
	if _, err := svc.DeliverAt(ctx, tenantID, transactionalDraft.ID, "actor-comm-1", time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)); !errors.Is(err, communications.ErrQuietHours) {
		t.Fatalf("non-urgent message during quiet hours must fail with ErrQuietHours, got %v", err)
	}

	// Outside quiet hours the same transactional message is deliverable.
	if _, err := svc.DeliverAt(ctx, tenantID, transactionalDraft.ID, "actor-comm-1", time.Date(2026, 8, 5, 23, 59, 0, 0, time.UTC)); err != nil {
		t.Fatalf("transactional message outside quiet hours must deliver, got %v", err)
	}

	// Urgent messages bypass quiet hours entirely.
	urgentTmpl := createTemplate(t, svc, tenantID, "pref_urgent", communications.AudienceOwner, communications.ConsentClassUrgent)
	addVersion(t, svc, tenantID, urgentTmpl.ID, communications.LanguageEnglish, "Safety alert", "Immediate action needed.")
	urgentDraft, err := svc.CreateDraft(ctx, tenantID, communications.DraftParams{
		Audience:    communications.AudienceOwner,
		RecipientID: recipient,
		Source:      communications.SourceTemplate,
		TemplateKey: "pref_urgent",
	}, "actor-comm-1")
	if err != nil {
		t.Fatalf("create urgent draft: %v", err)
	}
	if _, err := svc.DeliverAt(ctx, tenantID, urgentDraft.ID, "actor-comm-1", time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("urgent message must bypass quiet hours, got %v", err)
	}
}

func TestCommunicationsDeliveryHistoryRecordsStatusAndFailure(t *testing.T) {
	svc := newCommunicationsService(t)
	ctx := context.Background()
	tenantID := "tenant-com-dlv"

	tmpl := createTemplate(t, svc, tenantID, "checkin_reminder", communications.AudienceGuest, communications.ConsentClassTransactional)
	addVersion(t, svc, tenantID, tmpl.ID, communications.LanguageEnglish, "Check-in reminder", "See you tomorrow.")
	draft, err := svc.CreateDraft(ctx, tenantID, communications.DraftParams{
		Audience:    communications.AudienceGuest,
		RecipientID: "guest-dlv-1",
		Source:      communications.SourceTemplate,
		TemplateKey: "checkin_reminder",
	}, "actor-comm-1")
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}

	delivery, err := svc.Deliver(ctx, tenantID, draft.ID, "actor-comm-1")
	if err != nil {
		t.Fatalf("deliver: %v", err)
	}

	sent, err := svc.RecordDeliveryResult(ctx, tenantID, delivery.ID, communications.DeliveryStatusSent, "")
	if err != nil {
		t.Fatalf("record sent: %v", err)
	}
	if sent.Status != communications.DeliveryStatusSent {
		t.Fatalf("delivery must be sent, got %q", sent.Status)
	}

	delivered, err := svc.RecordDeliveryResult(ctx, tenantID, delivery.ID, communications.DeliveryStatusDelivered, "")
	if err != nil {
		t.Fatalf("record delivered: %v", err)
	}
	if delivered.Status != communications.DeliveryStatusDelivered || delivered.DeliveredAt == nil {
		t.Fatalf("delivery must be delivered with a timestamp, got status=%q delivered_at=%v", delivered.Status, delivered.DeliveredAt)
	}

	// A second message that fails records the failure visibly.
	draft2, err := svc.CreateDraft(ctx, tenantID, communications.DraftParams{
		Audience:    communications.AudienceGuest,
		RecipientID: "guest-dlv-2",
		Source:      communications.SourceTemplate,
		TemplateKey: "checkin_reminder",
	}, "actor-comm-1")
	if err != nil {
		t.Fatalf("create second draft: %v", err)
	}
	delivery2, err := svc.Deliver(ctx, tenantID, draft2.ID, "actor-comm-1")
	if err != nil {
		t.Fatalf("deliver second: %v", err)
	}
	failed, err := svc.RecordDeliveryResult(ctx, tenantID, delivery2.ID, communications.DeliveryStatusFailed, "provider rejected: invalid phone")
	if err != nil {
		t.Fatalf("record failure: %v", err)
	}
	if failed.Status != communications.DeliveryStatusFailed || failed.Error != "provider rejected: invalid phone" {
		t.Fatalf("failure must be recorded with its error, got status=%q error=%q", failed.Status, failed.Error)
	}

	deliveries, err := svc.ListDeliveries(ctx, tenantID, "")
	if err != nil {
		t.Fatalf("list deliveries: %v", err)
	}
	if len(deliveries) != 2 {
		t.Fatalf("delivery history must keep both records, got %d", len(deliveries))
	}

	// Invalid delivery statuses are rejected and never recorded.
	if _, err := svc.RecordDeliveryResult(ctx, tenantID, delivery.ID, "bounced", ""); !errors.Is(err, communications.ErrInvalidDeliveryStatus) {
		t.Fatalf("invalid delivery status must be rejected, got %v", err)
	}
}

func TestCommunicationsPreviewHidesAccessDetails(t *testing.T) {
	svc := newCommunicationsService(t)
	ctx := context.Background()
	tenantID := "tenant-com-prev"

	tmpl := createTemplate(t, svc, tenantID, "stay_access", communications.AudienceGuest, communications.ConsentClassTransactional)
	addVersion(t, svc, tenantID, tmpl.ID, communications.LanguageEnglish,
		"Your access details",
		"Your door code is {access_code} and the smart lock PIN is 482913.",
	)

	preview, err := svc.PreviewTemplate(ctx, tenantID, "stay_access", communications.LanguageEnglish)
	if err != nil {
		t.Fatalf("preview template: %v", err)
	}
	if preview.Subject == "" || preview.Body == "" {
		t.Fatal("preview must contain redacted subject and body")
	}
	if containsAny(preview.Subject, "482913") || containsAny(preview.Body, "482913") {
		t.Fatalf("preview leaked the smart lock PIN: %+v", preview)
	}
	if containsAny(preview.Body, "access_code") {
		t.Fatalf("preview leaked the access code placeholder: %q", preview.Body)
	}
	if !containsAny(preview.Body, "{access_details_hidden}") {
		t.Fatalf("preview must hide the access placeholder, got %q", preview.Body)
	}

	// Draft previews are redacted the same way.
	draft, err := svc.CreateDraft(ctx, tenantID, communications.DraftParams{
		Audience:    communications.AudienceGuest,
		RecipientID: "guest-prev-1",
		Source:      communications.SourceTemplate,
		TemplateKey: "stay_access",
	}, "actor-comm-1")
	if err != nil {
		t.Fatalf("create access draft: %v", err)
	}
	draftPreview, err := svc.PreviewDraft(ctx, tenantID, draft.ID)
	if err != nil {
		t.Fatalf("preview draft: %v", err)
	}
	if containsAny(draftPreview.Body, "482913", "access_code") {
		t.Fatalf("draft preview leaked access details: %q", draftPreview.Body)
	}
}

func TestCommunicationsCrossTenantFailsClosed(t *testing.T) {
	pool := communicationsPool(t)
	ctx := context.Background()

	tenantA := communications.NewCommunicationsService(pool).WithAudit(audit.NewAuditStore(pool))
	tmpl, err := tenantA.CreateTemplate(ctx, "tenant-com-a", communications.TemplateParams{
		TemplateKey:  "secret_key",
		Audience:     communications.AudienceOwner,
		ConsentClass: communications.ConsentClassTransactional,
	}, "actor-a")
	if err != nil {
		t.Fatalf("create template in tenant A: %v", err)
	}
	addVersion(t, tenantA, "tenant-com-a", tmpl.ID, communications.LanguageEnglish, "Owner message", "Hello.")

	tenantB := communications.NewCommunicationsService(pool).WithAudit(audit.NewAuditStore(pool))
	if _, err := tenantB.GetTemplateByKey(ctx, "tenant-com-b", "secret_key"); !errors.Is(err, communications.ErrTemplateNotFound) {
		t.Fatalf("cross-tenant template read must fail closed with ErrTemplateNotFound, got %v", err)
	}
	if _, err := tenantB.CreateDraft(ctx, "tenant-com-b", communications.DraftParams{
		Audience:    communications.AudienceOwner,
		RecipientID: "owner-b",
		Source:      communications.SourceTemplate,
		TemplateKey: "secret_key",
	}, "actor-b"); !errors.Is(err, communications.ErrTemplateNotFound) {
		t.Fatalf("cross-tenant template draft must fail closed with ErrTemplateNotFound, got %v", err)
	}
	if _, err := tenantB.ListSecureLinks(ctx, "tenant-com-b", "prop-x"); err != nil {
		t.Fatalf("empty cross-tenant link listing must be allowed, got %v", err)
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if sub != "" && strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
