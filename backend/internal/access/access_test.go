package access_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"comfort-curators-backend/internal/access"
	"comfort-curators-backend/internal/platform/audit"

	"github.com/jackc/pgx/v5/pgxpool"
)

func accessPostgresAvailable() bool {
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

func accessDBConnString() string {
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

func accessPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if !accessPostgresAvailable() {
		t.Skip("PostgreSQL not available for access integration test")
	}
	pool, err := pgxpool.New(context.Background(), accessDBConnString())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := access.EnsureSchema(context.Background(), pool); err != nil {
		t.Fatalf("ensure access schema: %v", err)
	}
	if err := audit.EnsureSchema(context.Background(), pool); err != nil {
		t.Fatalf("ensure audit schema: %v", err)
	}
	for _, table := range []string{
		"access_disclosures",
		"access_custody_events",
		"access_grants",
		"access_holds",
		"property_access_secrets",
		"audit_events",
	} {
		if _, err := pool.Exec(context.Background(), "TRUNCATE "+table+" CASCADE"); err != nil {
			t.Fatalf("truncate %s: %v", table, err)
		}
	}
	return pool
}

func newAccessService(t *testing.T) *access.Service {
	t.Helper()
	pool := accessPool(t)
	return access.NewService(pool).WithAudit(audit.NewAuditStore(pool))
}

func TestStoreAndRetrieveSecret(t *testing.T) {
	svc := newAccessService(t)
	ctx := context.Background()
	tenantID := "t-acc-test-1"
	propertyID := "prop-acc-test-1"

	sec, err := svc.StoreSecret(ctx, tenantID, propertyID, access.CreateSecretParams{
		SecretType:      access.SecretTypeKeyCode,
		Label:           "Main Door",
		EncryptedValue:  "enc:k1:aGVsbG8=",
		EncryptionKeyID: "k1",
		Metadata:        "{}",
	}, "actor-1")
	if err != nil {
		t.Fatalf("StoreSecret: %v", err)
	}
	if sec.ID == "" {
		t.Fatal("expected secret ID")
	}
	if sec.SecretType != access.SecretTypeKeyCode {
		t.Fatalf("expected key_code, got %s", sec.SecretType)
	}

	got, err := svc.GetSecret(ctx, tenantID, sec.ID)
	if err != nil {
		t.Fatalf("GetSecret: %v", err)
	}
	if got.EncryptedValue != "enc:k1:aGVsbG8=" {
		t.Fatalf("encrypted value mismatch: got %s", got.EncryptedValue)
	}

	list, err := svc.ListSecrets(ctx, tenantID, propertyID)
	if err != nil {
		t.Fatalf("ListSecrets: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 secret, got %d", len(list))
	}
}

func TestCreateGrant(t *testing.T) {
	svc := newAccessService(t)
	ctx := context.Background()
	tenantID := "t-acc-test-2"
	propertyID := "prop-acc-test-2"

	sec, err := svc.StoreSecret(ctx, tenantID, propertyID, access.CreateSecretParams{
		SecretType:      access.SecretTypeLockboxCode,
		Label:           "Lockbox",
		EncryptedValue:  "enc:k2:Y29kZQ==",
		EncryptionKeyID: "k2",
	}, "owner-1")
	if err != nil {
		t.Fatalf("StoreSecret: %v", err)
	}

	now := time.Now().UTC()
	windowStart := now.Add(-1 * time.Hour)
	windowEnd := now.Add(24 * time.Hour)

	grant, err := svc.CreateGrant(ctx, tenantID, propertyID, sec.ID, access.CreateGrantParams{
		GranteeID:   "curator-1",
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
		Reason:      "turnover cleaning",
	}, "ops-1")
	if err != nil {
		t.Fatalf("CreateGrant: %v", err)
	}
	if grant.Status != access.GrantStatusActive {
		t.Fatalf("expected active, got %s", grant.Status)
	}
	if grant.GranteeID != "curator-1" {
		t.Fatalf("expected curator-1, got %s", grant.GranteeID)
	}
}

func TestCreateGrantInvalidWindow(t *testing.T) {
	svc := newAccessService(t)
	ctx := context.Background()
	tenantID := "t-acc-test-3"
	propertyID := "prop-acc-test-3"

	sec, err := svc.StoreSecret(ctx, tenantID, propertyID, access.CreateSecretParams{
		SecretType:     access.SecretTypeSmartLockPIN,
		Label:          "Smart Lock",
		EncryptedValue: "enc:k3:cGlu",
	}, "owner-1")
	if err != nil {
		t.Fatalf("StoreSecret: %v", err)
	}

	now := time.Now().UTC()
	_, err = svc.CreateGrant(ctx, tenantID, propertyID, sec.ID, access.CreateGrantParams{
		GranteeID:   "curator-1",
		WindowStart: now.Add(1 * time.Hour),
		WindowEnd:   now,
		Reason:      "bad window",
	}, "ops-1")
	if err == nil {
		t.Fatal("expected error for invalid window")
	}
	if !errors.Is(err, access.ErrInvalidWindow) {
		t.Fatalf("expected ErrInvalidWindow, got %v", err)
	}
}

func TestDiscloseWithinWindow(t *testing.T) {
	svc := newAccessService(t)
	ctx := context.Background()
	tenantID := "t-acc-test-4"
	propertyID := "prop-acc-test-4"

	sec, err := svc.StoreSecret(ctx, tenantID, propertyID, access.CreateSecretParams{
		SecretType:     access.SecretTypeGateCode,
		Label:          "Gate",
		EncryptedValue: "enc:k4:Z2F0ZQ==",
	}, "owner-1")
	if err != nil {
		t.Fatalf("StoreSecret: %v", err)
	}

	now := time.Now().UTC()
	grant, err := svc.CreateGrant(ctx, tenantID, propertyID, sec.ID, access.CreateGrantParams{
		GranteeID:   "curator-2",
		WindowStart: now.Add(-1 * time.Hour),
		WindowEnd:   now.Add(4 * time.Hour),
		Reason:      "gate maintenance",
	}, "ops-1")
	if err != nil {
		t.Fatalf("CreateGrant: %v", err)
	}

	disclosedSecret, disclosure, err := svc.DiscloseSecret(ctx, tenantID, grant.ID, "curator-2", now)
	if err != nil {
		t.Fatalf("DiscloseSecret: %v", err)
	}
	if disclosure.Result != access.DisclosureResultSuccess {
		t.Fatalf("expected success, got %s", disclosure.Result)
	}
	if disclosedSecret.EncryptedValue != "enc:k4:Z2F0ZQ==" {
		t.Fatalf("wrong encrypted value: %s", disclosedSecret.EncryptedValue)
	}
}

func TestDiscloseOutOfWindowFails(t *testing.T) {
	svc := newAccessService(t)
	ctx := context.Background()
	tenantID := "t-acc-test-5"
	propertyID := "prop-acc-test-5"

	sec, err := svc.StoreSecret(ctx, tenantID, propertyID, access.CreateSecretParams{
		SecretType:     access.SecretTypeAlarmCode,
		Label:          "Alarm",
		EncryptedValue: "enc:k5:YWxhcm0=",
	}, "owner-1")
	if err != nil {
		t.Fatalf("StoreSecret: %v", err)
	}

	now := time.Now().UTC()
	grant, err := svc.CreateGrant(ctx, tenantID, propertyID, sec.ID, access.CreateGrantParams{
		GranteeID:   "curator-3",
		WindowStart: now.Add(2 * time.Hour),
		WindowEnd:   now.Add(6 * time.Hour),
		Reason:      "alarm check",
	}, "ops-1")
	if err != nil {
		t.Fatalf("CreateGrant: %v", err)
	}

	_, disclosure, err := svc.DiscloseSecret(ctx, tenantID, grant.ID, "curator-3", now)
	if err == nil {
		t.Fatal("expected out-of-window error")
	}
	if !errors.Is(err, access.ErrGrantWindowMismatch) {
		t.Fatalf("expected ErrGrantWindowMismatch, got %v", err)
	}
	if disclosure.Result != access.DisclosureResultOutOfWindow {
		t.Fatalf("expected out_of_window, got %s", disclosure.Result)
	}
}

func TestDiscloseUnassignedFails(t *testing.T) {
	svc := newAccessService(t)
	ctx := context.Background()
	tenantID := "t-acc-test-6"
	propertyID := "prop-acc-test-6"

	sec, err := svc.StoreSecret(ctx, tenantID, propertyID, access.CreateSecretParams{
		SecretType:     access.SecretTypeOther,
		Label:          "Other",
		EncryptedValue: "enc:k6:b3RoZXI=",
	}, "owner-1")
	if err != nil {
		t.Fatalf("StoreSecret: %v", err)
	}

	now := time.Now().UTC()
	grant, err := svc.CreateGrant(ctx, tenantID, propertyID, sec.ID, access.CreateGrantParams{
		GranteeID:   "curator-4",
		WindowStart: now.Add(-1 * time.Hour),
		WindowEnd:   now.Add(4 * time.Hour),
		Reason:      "cleaning",
	}, "ops-1")
	if err != nil {
		t.Fatalf("CreateGrant: %v", err)
	}

	_, disclosure, err := svc.DiscloseSecret(ctx, tenantID, grant.ID, "wrong-person", now)
	if err == nil {
		t.Fatal("expected unauthorized error for unassigned user")
	}
	if !errors.Is(err, access.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
	if disclosure.Result != access.DisclosureResultDenied {
		t.Fatalf("expected denied, got %s", disclosure.Result)
	}
}

func TestRevocationIsImmediate(t *testing.T) {
	svc := newAccessService(t)
	ctx := context.Background()
	tenantID := "t-acc-test-7"
	propertyID := "prop-acc-test-7"

	sec, err := svc.StoreSecret(ctx, tenantID, propertyID, access.CreateSecretParams{
		SecretType:     access.SecretTypeKeyCode,
		Label:          "Key",
		EncryptedValue: "enc:k7:a2V5",
	}, "owner-1")
	if err != nil {
		t.Fatalf("StoreSecret: %v", err)
	}

	now := time.Now().UTC()
	grant, err := svc.CreateGrant(ctx, tenantID, propertyID, sec.ID, access.CreateGrantParams{
		GranteeID:   "curator-5",
		WindowStart: now.Add(-1 * time.Hour),
		WindowEnd:   now.Add(24 * time.Hour),
		Reason:      "service",
	}, "ops-1")
	if err != nil {
		t.Fatalf("CreateGrant: %v", err)
	}

	revoked, err := svc.RevokeGrant(ctx, tenantID, grant.ID, "ops-1", "security concern")
	if err != nil {
		t.Fatalf("RevokeGrant: %v", err)
	}
	if revoked.Status != access.GrantStatusRevoked {
		t.Fatalf("expected revoked, got %s", revoked.Status)
	}

	_, disclosure, err := svc.DiscloseSecret(ctx, tenantID, grant.ID, "curator-5", now)
	if err == nil {
		t.Fatal("expected error after revocation")
	}
	if !errors.Is(err, access.ErrGrantNotActive) {
		t.Fatalf("expected ErrGrantNotActive, got %v", err)
	}
	if disclosure.Result != access.DisclosureResultRevoked {
		t.Fatalf("expected revoked, got %s", disclosure.Result)
	}
}

func TestEmergencyAccessIsAttributed(t *testing.T) {
	svc := newAccessService(t)
	ctx := context.Background()
	tenantID := "t-acc-test-8"
	propertyID := "prop-acc-test-8"

	_, err := svc.StoreSecret(ctx, tenantID, propertyID, access.CreateSecretParams{
		SecretType:     access.SecretTypeLockboxCode,
		Label:          "Lockbox",
		EncryptedValue: "enc:k8:bG9jaw==",
	}, "owner-1")
	if err != nil {
		t.Fatalf("StoreSecret: %v", err)
	}

	grant, secret, err := svc.EmergencyAccess(ctx, tenantID, propertyID, access.EmergencyAccessParams{
		Reason: "burst pipe flooding",
	}, "ops-emergency-1")
	if err != nil {
		t.Fatalf("EmergencyAccess: %v", err)
	}
	if !grant.IsEmergency {
		t.Fatal("expected emergency flag")
	}
	if grant.EmergencyReason != "burst pipe flooding" {
		t.Fatalf("wrong emergency reason: %s", grant.EmergencyReason)
	}
	if grant.GranteeID != "ops-emergency-1" {
		t.Fatalf("wrong grantee: %s", grant.GranteeID)
	}
	if secret.EncryptedValue != "enc:k8:bG9jaw==" {
		t.Fatalf("wrong secret value")
	}

	events, err := svc.ListCustodyEvents(ctx, tenantID, propertyID)
	if err != nil {
		t.Fatalf("ListCustodyEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 custody event, got %d", len(events))
	}
	if events[0].EventType != access.CustodyEventTypeEmergencyAccess {
		t.Fatalf("expected emergency_access, got %s", events[0].EventType)
	}
	if events[0].ActorID != "ops-emergency-1" {
		t.Fatalf("wrong actor: %s", events[0].ActorID)
	}
}

func TestAcknowledgeAndReturn(t *testing.T) {
	svc := newAccessService(t)
	ctx := context.Background()
	tenantID := "t-acc-test-9"
	propertyID := "prop-acc-test-9"

	sec, err := svc.StoreSecret(ctx, tenantID, propertyID, access.CreateSecretParams{
		SecretType:     access.SecretTypeKeyCode,
		Label:          "Main Key",
		EncryptedValue: "enc:k9:bWFpbg==",
	}, "owner-1")
	if err != nil {
		t.Fatalf("StoreSecret: %v", err)
	}

	now := time.Now().UTC()
	grant, err := svc.CreateGrant(ctx, tenantID, propertyID, sec.ID, access.CreateGrantParams{
		GranteeID:   "curator-6",
		WindowStart: now.Add(-1 * time.Hour),
		WindowEnd:   now.Add(8 * time.Hour),
		Reason:      "turnover",
	}, "ops-1")
	if err != nil {
		t.Fatalf("CreateGrant: %v", err)
	}

	ack, err := svc.AcknowledgeAccess(ctx, tenantID, grant.ID, "curator-6")
	if err != nil {
		t.Fatalf("AcknowledgeAccess: %v", err)
	}
	if ack.Status != access.GrantStatusAcknowledged {
		t.Fatalf("expected acknowledged, got %s", ack.Status)
	}

	ret, err := svc.ReturnAccess(ctx, tenantID, grant.ID, "curator-6")
	if err != nil {
		t.Fatalf("ReturnAccess: %v", err)
	}
	if ret.Status != access.GrantStatusReturned {
		t.Fatalf("expected returned, got %s", ret.Status)
	}
}

func TestAccessHoldBlocksDisclosure(t *testing.T) {
	svc := newAccessService(t)
	ctx := context.Background()
	tenantID := "t-acc-test-10"
	propertyID := "prop-acc-test-10"

	sec, err := svc.StoreSecret(ctx, tenantID, propertyID, access.CreateSecretParams{
		SecretType:     access.SecretTypeKeyCode,
		Label:          "Main Key",
		EncryptedValue: "enc:k10:aG9sZA==",
	}, "owner-1")
	if err != nil {
		t.Fatalf("StoreSecret: %v", err)
	}

	now := time.Now().UTC()
	grant, err := svc.CreateGrant(ctx, tenantID, propertyID, sec.ID, access.CreateGrantParams{
		GranteeID:   "curator-7",
		WindowStart: now.Add(-1 * time.Hour),
		WindowEnd:   now.Add(4 * time.Hour),
		Reason:      "inspection",
	}, "ops-1")
	if err != nil {
		t.Fatalf("CreateGrant: %v", err)
	}

	_, err = svc.PlaceHold(ctx, tenantID, propertyID, access.CreateHoldParams{
		Reason: "owner on premises",
	}, "owner-1")
	if err != nil {
		t.Fatalf("PlaceHold: %v", err)
	}

	_, disclosure, err := svc.DiscloseSecret(ctx, tenantID, grant.ID, "curator-7", now)
	if err == nil {
		t.Fatal("expected hold error")
	}
	if !errors.Is(err, access.ErrAccessHeld) {
		t.Fatalf("expected ErrAccessHeld, got %v", err)
	}
	if disclosure.Result != access.DisclosureResultHeld {
		t.Fatalf("expected held, got %s", disclosure.Result)
	}
}

func TestReleaseHoldAllowsDisclosure(t *testing.T) {
	svc := newAccessService(t)
	ctx := context.Background()
	tenantID := "t-acc-test-11"
	propertyID := "prop-acc-test-11"

	sec, err := svc.StoreSecret(ctx, tenantID, propertyID, access.CreateSecretParams{
		SecretType:     access.SecretTypeKeyCode,
		Label:          "Key",
		EncryptedValue: "enc:k11:cmVsZWFzZQ==",
	}, "owner-1")
	if err != nil {
		t.Fatalf("StoreSecret: %v", err)
	}

	now := time.Now().UTC()
	grant, err := svc.CreateGrant(ctx, tenantID, propertyID, sec.ID, access.CreateGrantParams{
		GranteeID:   "curator-8",
		WindowStart: now.Add(-1 * time.Hour),
		WindowEnd:   now.Add(4 * time.Hour),
		Reason:      "cleaning",
	}, "ops-1")
	if err != nil {
		t.Fatalf("CreateGrant: %v", err)
	}

	hold, err := svc.PlaceHold(ctx, tenantID, propertyID, access.CreateHoldParams{
		Reason: "owner present",
	}, "owner-1")
	if err != nil {
		t.Fatalf("PlaceHold: %v", err)
	}

	_, err = svc.ReleaseHold(ctx, tenantID, hold.ID, "owner-1")
	if err != nil {
		t.Fatalf("ReleaseHold: %v", err)
	}

	_, disclosure, err := svc.DiscloseSecret(ctx, tenantID, grant.ID, "curator-8", now)
	if err != nil {
		t.Fatalf("DiscloseSecret after hold release: %v", err)
	}
	if disclosure.Result != access.DisclosureResultSuccess {
		t.Fatalf("expected success, got %s", disclosure.Result)
	}
}

func TestCreateGrantBlockedByHold(t *testing.T) {
	svc := newAccessService(t)
	ctx := context.Background()
	tenantID := "t-acc-test-12"
	propertyID := "prop-acc-test-12"

	sec, err := svc.StoreSecret(ctx, tenantID, propertyID, access.CreateSecretParams{
		SecretType:     access.SecretTypeKeyCode,
		Label:          "Key",
		EncryptedValue: "enc:k12:YmxvY2s=",
	}, "owner-1")
	if err != nil {
		t.Fatalf("StoreSecret: %v", err)
	}

	_, err = svc.PlaceHold(ctx, tenantID, propertyID, access.CreateHoldParams{
		Reason: "security incident",
	}, "ops-1")
	if err != nil {
		t.Fatalf("PlaceHold: %v", err)
	}

	now := time.Now().UTC()
	_, err = svc.CreateGrant(ctx, tenantID, propertyID, sec.ID, access.CreateGrantParams{
		GranteeID:   "curator-9",
		WindowStart: now,
		WindowEnd:   now.Add(4 * time.Hour),
		Reason:      "should be blocked",
	}, "ops-1")
	if err == nil {
		t.Fatal("expected hold to block grant creation")
	}
	if !errors.Is(err, access.ErrAccessHeld) {
		t.Fatalf("expected ErrAccessHeld, got %v", err)
	}
}

func TestInvalidSecretType(t *testing.T) {
	svc := newAccessService(t)
	ctx := context.Background()

	_, err := svc.StoreSecret(ctx, "t-err", "p-err", access.CreateSecretParams{
		SecretType:     "invalid_type",
		EncryptedValue: "enc:xxx",
	}, "actor-1")
	if err == nil {
		t.Fatal("expected error for invalid secret type")
	}
}

func TestCrossTenantSecretAccess(t *testing.T) {
	svc := newAccessService(t)
	ctx := context.Background()
	tenantID := "t-acc-cross-1"
	propertyID := "prop-acc-cross-1"

	sec, err := svc.StoreSecret(ctx, tenantID, propertyID, access.CreateSecretParams{
		SecretType:     access.SecretTypeKeyCode,
		Label:          "Key",
		EncryptedValue: "enc:k13:Y3Jvc3M=",
	}, "owner-1")
	if err != nil {
		t.Fatalf("StoreSecret: %v", err)
	}

	_, err = svc.GetSecret(ctx, "other-tenant", sec.ID)
	if err == nil {
		t.Fatal("expected error for cross-tenant access")
	}
	if !errors.Is(err, access.ErrSecretNotFound) {
		t.Fatalf("expected ErrSecretNotFound, got %v", err)
	}
}

func TestCrossTenantGrantAccess(t *testing.T) {
	svc := newAccessService(t)
	ctx := context.Background()
	tenantID := "t-acc-cross-2"
	propertyID := "prop-acc-cross-2"

	sec, err := svc.StoreSecret(ctx, tenantID, propertyID, access.CreateSecretParams{
		SecretType:     access.SecretTypeKeyCode,
		Label:          "Key",
		EncryptedValue: "enc:k14:Y3Jvc3My",
	}, "owner-1")
	if err != nil {
		t.Fatalf("StoreSecret: %v", err)
	}

	now := time.Now().UTC()
	grant, err := svc.CreateGrant(ctx, tenantID, propertyID, sec.ID, access.CreateGrantParams{
		GranteeID:   "curator-10",
		WindowStart: now,
		WindowEnd:   now.Add(4 * time.Hour),
		Reason:      "valid",
	}, "ops-1")
	if err != nil {
		t.Fatalf("CreateGrant: %v", err)
	}

	_, err = svc.GetGrant(ctx, "other-tenant", grant.ID)
	if err == nil {
		t.Fatal("expected error for cross-tenant grant access")
	}
	if !errors.Is(err, access.ErrGrantNotFound) {
		t.Fatalf("expected ErrGrantNotFound, got %v", err)
	}
}

func TestCustodyEventsAreAppendOnly(t *testing.T) {
	svc := newAccessService(t)
	ctx := context.Background()
	tenantID := "t-acc-events-1"
	propertyID := "prop-acc-events-1"

	sec, err := svc.StoreSecret(ctx, tenantID, propertyID, access.CreateSecretParams{
		SecretType:     access.SecretTypeKeyCode,
		Label:          "Key",
		EncryptedValue: "enc:k15:ZXZlbnRz",
	}, "owner-1")
	if err != nil {
		t.Fatalf("StoreSecret: %v", err)
	}

	now := time.Now().UTC()
	grant, err := svc.CreateGrant(ctx, tenantID, propertyID, sec.ID, access.CreateGrantParams{
		GranteeID:   "curator-11",
		WindowStart: now,
		WindowEnd:   now.Add(4 * time.Hour),
		Reason:      "service",
	}, "ops-1")
	if err != nil {
		t.Fatalf("CreateGrant: %v", err)
	}

	_, _, err = svc.DiscloseSecret(ctx, tenantID, grant.ID, "curator-11", now.Add(30*time.Minute))
	if err != nil {
		t.Fatalf("DiscloseSecret: %v", err)
	}

	_, err = svc.AcknowledgeAccess(ctx, tenantID, grant.ID, "curator-11")
	if err != nil {
		t.Fatalf("AcknowledgeAccess: %v", err)
	}

	_, err = svc.ReturnAccess(ctx, tenantID, grant.ID, "curator-11")
	if err != nil {
		t.Fatalf("ReturnAccess: %v", err)
	}

	events, err := svc.ListCustodyEvents(ctx, tenantID, propertyID)
	if err != nil {
		t.Fatalf("ListCustodyEvents: %v", err)
	}

	expectedTypes := []string{
		access.CustodyEventTypeReturned,
		access.CustodyEventTypeAcknowledged,
		access.CustodyEventTypeDisclosed,
		access.CustodyEventTypeIssued,
	}
	if len(events) != len(expectedTypes) {
		t.Fatalf("expected %d events, got %d", len(expectedTypes), len(events))
	}
	for i, evt := range events {
		if evt.EventType != expectedTypes[i] {
			t.Fatalf("event %d: expected %s, got %s", i, expectedTypes[i], evt.EventType)
		}
	}
}

func TestDisclosureIsAudited(t *testing.T) {
	svc := newAccessService(t)
	ctx := context.Background()
	tenantID := "t-acc-audit-1"
	propertyID := "prop-acc-audit-1"

	sec, err := svc.StoreSecret(ctx, tenantID, propertyID, access.CreateSecretParams{
		SecretType:     access.SecretTypeKeyCode,
		Label:          "Key",
		EncryptedValue: "enc:k16:YXVkaXQ=",
	}, "owner-1")
	if err != nil {
		t.Fatalf("StoreSecret: %v", err)
	}

	now := time.Now().UTC()
	grant, err := svc.CreateGrant(ctx, tenantID, propertyID, sec.ID, access.CreateGrantParams{
		GranteeID:   "curator-12",
		WindowStart: now.Add(-1 * time.Hour),
		WindowEnd:   now.Add(4 * time.Hour),
		Reason:      "audit test",
	}, "ops-1")
	if err != nil {
		t.Fatalf("CreateGrant: %v", err)
	}

	_, _, err = svc.DiscloseSecret(ctx, tenantID, grant.ID, "curator-12", now)
	if err != nil {
		t.Fatalf("DiscloseSecret: %v", err)
	}

	disclosures, err := svc.ListDisclosures(ctx, tenantID, grant.ID)
	if err != nil {
		t.Fatalf("ListDisclosures: %v", err)
	}
	if len(disclosures) != 1 {
		t.Fatalf("expected 1 disclosure, got %d", len(disclosures))
	}
	if disclosures[0].Result != access.DisclosureResultSuccess {
		t.Fatalf("expected success, got %s", disclosures[0].Result)
	}
	if disclosures[0].RequestorID != "curator-12" {
		t.Fatalf("wrong requestor: %s", disclosures[0].RequestorID)
	}
}

func TestOutOfWindowDisclosureIsAudited(t *testing.T) {
	svc := newAccessService(t)
	ctx := context.Background()
	tenantID := "t-acc-audit-2"
	propertyID := "prop-acc-audit-2"

	sec, err := svc.StoreSecret(ctx, tenantID, propertyID, access.CreateSecretParams{
		SecretType:     access.SecretTypeKeyCode,
		Label:          "Key",
		EncryptedValue: "enc:k17:b3V0",
	}, "owner-1")
	if err != nil {
		t.Fatalf("StoreSecret: %v", err)
	}

	now := time.Now().UTC()
	grant, err := svc.CreateGrant(ctx, tenantID, propertyID, sec.ID, access.CreateGrantParams{
		GranteeID:   "curator-13",
		WindowStart: now.Add(2 * time.Hour),
		WindowEnd:   now.Add(4 * time.Hour),
		Reason:      "future window",
	}, "ops-1")
	if err != nil {
		t.Fatalf("CreateGrant: %v", err)
	}

	_, _, err = svc.DiscloseSecret(ctx, tenantID, grant.ID, "curator-13", now)
	if !errors.Is(err, access.ErrGrantWindowMismatch) {
		t.Fatalf("expected ErrGrantWindowMismatch, got %v", err)
	}

	disclosures, err := svc.ListDisclosures(ctx, tenantID, grant.ID)
	if err != nil {
		t.Fatalf("ListDisclosures: %v", err)
	}
	if len(disclosures) != 1 {
		t.Fatalf("expected 1 denied disclosure record, got %d", len(disclosures))
	}
	if disclosures[0].Result != access.DisclosureResultOutOfWindow {
		t.Fatalf("expected out_of_window, got %s", disclosures[0].Result)
	}
}
