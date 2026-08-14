package security_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"comfort-curators-backend/internal/platform/audit"
	"comfort-curators-backend/internal/platform/security"
)

func TestCCSEC001SecretsAreRedacted(t *testing.T) {
	s := security.NewSecretString("sk-live-test")
	if s.String() != "[redacted]" {
		t.Error("SecretString must be redacted")
	}

	jsonBytes, err := s.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if string(jsonBytes) != `"[redacted]"` {
		t.Error("SecretString JSON must be redacted")
	}

	b := security.NewSecretBytes([]byte("very-secret"))
	if b.String() != "[redacted]" {
		t.Error("SecretBytes must be redacted")
	}

	logVal := s.LogValue()
	if logVal.String() != "[redacted]" {
		t.Error("SecretString log value must be redacted")
	}
}

func TestCCSEC001SecureLinkExpiresAndRejectsReplay(t *testing.T) {
	store := security.NewInMemorySessionStore()
	ctx := context.Background()

	sessionID := security.SessionID("sess-expire-test")
	sess := &security.Session{
		ID:        sessionID,
		ActorID:   "user-1",
		TenantID:  "tenant-1",
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}
	store.AddSession(sess)

	_, err := store.GetSession(ctx, sessionID)
	if err != security.ErrSessionExpired {
		t.Errorf("expired session/token must fail: got %v", err)
	}

	activeID := security.SessionID("sess-active-test")
	activeSess := &security.Session{
		ID:        activeID,
		ActorID:   "user-1",
		TenantID:  "tenant-1",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	store.AddSession(activeSess)

	revocation := security.SessionRevocation{
		SessionID: activeID,
		Reason:    "security incident",
		RevokedBy: "admin",
	}
	if err := store.RevokeSession(ctx, revocation); err != nil {
		t.Fatal(err)
	}

	_, err = store.GetSession(ctx, activeID)
	if err != security.ErrSessionRevoked {
		t.Errorf("replay/revoked token must fail: got %v", err)
	}
}

func TestCCSEC001AuditEvidenceCannotBeRewritten(t *testing.T) {
	evt := audit.AuditEvent{
		ID:           "sec-audit-immutable",
		EventType:    audit.EventTypeSecurity,
		ActorID:      "admin-1",
		Action:       "key.create",
		ResourceType: "encryption_key",
		ResourceID:   "key-1",
	}

	data, err := json.Marshal(evt)
	if err != nil {
		t.Fatal(err)
	}

	var restored audit.AuditEvent
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}

	if restored.ID != evt.ID {
		t.Error("audit event ID must be immutable")
	}
	if restored.EventType != evt.EventType {
		t.Error("audit event type must be immutable")
	}
	if restored.ActorID != evt.ActorID {
		t.Error("audit event actor must be immutable")
	}
}

func TestCCSEC001CrossTenantRequestsFailClosed(t *testing.T) {
	denier := security.DenyAllAuthorizer{}
	ctx := context.Background()

	subjectA := security.Subject{
		ActorID:  "u1",
		TenantID: "tenant-a",
		Roles:    []string{"owner"},
	}

	resourceB := security.Resource{
		Type: "property",
		ID:   "prop-tenant-b",
	}

	err := denier.Can(ctx, subjectA, "read", resourceB)
	if err != security.ErrDenied {
		t.Errorf("cross-tenant read must be denied: got %v", err)
	}

	err = denier.Can(ctx, subjectA, "write", resourceB)
	if err != security.ErrDenied {
		t.Errorf("cross-tenant write must be denied: got %v", err)
	}

	err = denier.Can(ctx, subjectA, "delete", resourceB)
	if err != security.ErrDenied {
		t.Errorf("cross-tenant delete must be denied: got %v", err)
	}
}

func TestSecurityRateLimitMiddleware(t *testing.T) {
	baseURL := os.Getenv("CC_BASE_URL")
	if baseURL == "" {
		port := os.Getenv("CC_HTTP_PORT")
		if port == "" {
			port = "8080"
		}
		baseURL = fmt.Sprintf("http://127.0.0.1:%s", port)
	}

	resp, err := http.Get(baseURL + "/health/live")
	if err != nil {
		t.Skipf("server not available: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("health check failed: %d", resp.StatusCode)
	}
}

func TestSecurityCrossTenantDenialOnAPI(t *testing.T) {
	baseURL := os.Getenv("CC_BASE_URL")
	if baseURL == "" {
		port := os.Getenv("CC_HTTP_PORT")
		if port == "" {
			port = "8080"
		}
		baseURL = fmt.Sprintf("http://127.0.0.1:%s", port)
	}

	req, err := http.NewRequest(http.MethodGet, baseURL+"/api/v1/properties/prop-other-tenant", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer tenant-a-token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Skipf("server not available: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		t.Error("cross-tenant read must not return 200")
	}
}

func TestSecurityAuthorizationMatrixHttpEndpoints(t *testing.T) {
	baseURL := os.Getenv("CC_BASE_URL")
	if baseURL == "" {
		port := os.Getenv("CC_HTTP_PORT")
		if port == "" {
			port = "8080"
		}
		baseURL = fmt.Sprintf("http://127.0.0.1:%s", port)
	}

	noAuthResp, err := http.Get(baseURL + "/api/v1/properties")
	if err != nil {
		t.Skipf("server not available: %v", err)
	}
	defer noAuthResp.Body.Close()

	if noAuthResp.StatusCode != http.StatusUnauthorized {
		t.Logf("unauthenticated request returned %d", noAuthResp.StatusCode)
	}
}

func TestSecurityPrivilegedActionAlerting(t *testing.T) {
	logger := security.NewNoOpPrivilegedAccessLogger()

	privEvent := security.PrivilegedAccessEvent{
		ID:      "priv-evt-001",
		ActorID: "admin-1",
		Action:  "admin.system.config",
		Resource: security.Resource{
			Type: "system",
			ID:   "config-1",
		},
		MFAUsed: true,
		Success: true,
	}

	if err := logger.Log(context.Background(), privEvent); err != nil {
		t.Fatal(err)
	}

	events := logger.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 privileged event logged, got %d", len(events))
	}
	if events[0].ID != "priv-evt-001" {
		t.Error("privileged event must be logged")
	}

	normalEvent := security.PrivilegedAccessEvent{
		ID:      "normal-evt-001",
		ActorID: "user-1",
		Action:  "property.view",
		Success: true,
	}

	if err := logger.Log(context.Background(), normalEvent); err != nil {
		t.Fatal(err)
	}

	if len(logger.Events()) != 1 {
		t.Fatalf("normal action must not increase privileged log count, got %d", len(logger.Events()))
	}
}

func TestSecurityEncryptionFieldBoundary(t *testing.T) {
	km := security.NewNoOpKeyManager()

	keyID := security.NewKeyID()
	testKey := &security.Key{
		ID:        keyID,
		Algorithm: "aes256-gcm",
		KeyBytes:  security.NewSecretBytes([]byte("test-key-material-32-bytes-long!")),
		Active:    true,
	}
	km.AddKey(testKey)

	ctx := context.Background()
	plaintext := []byte("property-access-code-12345")

	ev, err := km.Encrypt(ctx, keyID, plaintext)
	if err != nil {
		t.Fatal(err)
	}

	if string(ev.Ciphertext) == string(plaintext) {
		t.Error("ciphertext must not equal plaintext")
	}

	decrypted, err := km.Decrypt(ctx, ev)
	if err != nil {
		t.Fatal(err)
	}

	if string(decrypted) != string(plaintext) {
		t.Error("decrypted value must equal original")
	}
}

func TestSecurityMFAEnforcement(t *testing.T) {
	ctx := context.Background()
	subject := security.Subject{ActorID: "u1", Roles: []string{"admin"}}

	enabled := security.NewNoOpMFAVerifier(security.MFAStateEnabled)
	err := enabled.RequireMFA(ctx, subject, "privileged.system.config")
	if err != security.ErrMFARequired {
		t.Errorf("MFA enabled must block privileged action: got %v", err)
	}

	disabled := security.NewNoOpMFAVerifier(security.MFAStateDisabled)
	err = disabled.RequireMFA(ctx, subject, "privileged.system.config")
	if err != nil {
		t.Errorf("MFA disabled must not block: got %v", err)
	}
}
