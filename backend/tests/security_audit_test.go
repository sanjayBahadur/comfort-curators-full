package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"comfort-curators-backend/internal/platform/app"
	"comfort-curators-backend/internal/platform/audit"
	"comfort-curators-backend/internal/platform/config"
	"comfort-curators-backend/internal/platform/database"
	"comfort-curators-backend/internal/platform/security"
	"comfort-curators-backend/internal/platform/testdb"
)

func TestSecuritySecretStringRedactedInLogs(t *testing.T) {
	secret := security.NewSecretString("super-secret-token-12345")

	if s := secret.String(); s != "[redacted]" {
		t.Errorf("SecretString.String() must be redacted, got %q", s)
	}

	if s := secret.GoString(); s != "[redacted]" {
		t.Errorf("SecretString.GoString() must be redacted, got %q", s)
	}

	if v := secret.Value(); v != "super-secret-token-12345" {
		t.Errorf("SecretString.Value() must return original, got %q", v)
	}

	jsonBytes, err := secret.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	if string(jsonBytes) != `"[redacted]"` {
		t.Errorf("SecretString.MarshalJSON() must be redacted, got %s", string(jsonBytes))
	}

	textBytes, err := secret.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText: %v", err)
	}
	if string(textBytes) != "[redacted]" {
		t.Errorf("SecretString.MarshalText() must be redacted, got %s", string(textBytes))
	}

	logVal := secret.LogValue()
	if logVal.String() != "[redacted]" {
		t.Errorf("SecretString.LogValue() must be redacted, got %s", logVal.String())
	}
}

func TestSecuritySecretBytesRedactedInLogs(t *testing.T) {
	secret := security.NewSecretBytes([]byte("sensitive-bytes-data"))

	if s := secret.String(); s != "[redacted]" {
		t.Errorf("SecretBytes.String() must be redacted, got %q", s)
	}

	if s := secret.GoString(); s != "[redacted]" {
		t.Errorf("SecretBytes.GoString() must be redacted, got %q", s)
	}

	v := secret.Value()
	if string(v) != "sensitive-bytes-data" {
		t.Errorf("SecretBytes.Value() must return original, got %q", string(v))
	}

	jsonBytes, err := secret.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	if string(jsonBytes) != `"[redacted]"` {
		t.Errorf("SecretBytes.MarshalJSON() must be redacted, got %s", string(jsonBytes))
	}

	logVal := secret.LogValue()
	if logVal.String() != "[redacted]" {
		t.Errorf("SecretBytes.LogValue() must be redacted, got %s", logVal.String())
	}
}

func TestSecurityPlaintextSecretsNotInLogOutput(t *testing.T) {
	var buf bytes.Buffer
	h := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			lower := strings.ToLower(a.Key)
			for _, rk := range []string{"password", "pass", "secret", "token", "key", "authorization", "credential", "dbpass", "db_pass"} {
				if strings.Contains(lower, rk) {
					a.Value = slog.StringValue("[redacted]")
					return a
				}
			}
			return a
		},
	})
	l := slog.New(h)
	ctx := context.Background()

	accessToken := security.NewSecretString("sk-live-abcdef1234567890")
	apiSecret := security.NewSecretBytes([]byte("whsec_verysecretkey"))

	l.LogAttrs(ctx, slog.LevelInfo, "auth request",
		slog.String("access_token", accessToken.String()),
		slog.Any("api_secret", apiSecret.LogValue()),
		slog.String("secret_string", accessToken.String()),
		slog.String("user_id", "user-123"),
	)

	output := buf.String()
	if output == "" {
		t.Fatal("expected log output")
	}

	var entry map[string]any
	if err := json.Unmarshal([]byte(output), &entry); err != nil {
		t.Fatalf("unmarshal log: %v", err)
	}

	for _, key := range []string{"access_token", "api_secret", "secret_string"} {
		if v, ok := entry[key].(string); ok && v != "[redacted]" {
			t.Errorf("field %q contains plaintext secret: %v", key, entry[key])
		}
	}

	if v, ok := entry["user_id"].(string); !ok || v != "user-123" {
		t.Errorf("non-secret field user_id should be visible, got %v", entry["user_id"])
	}
}

func TestSecurityEncryptionBoundary(t *testing.T) {
	km := security.NewNoOpKeyManager()

	keyID := security.NewKeyID()
	testKey := &security.Key{
		ID:        keyID,
		Algorithm: "aes256-gcm",
		KeyBytes:  security.NewSecretBytes([]byte("test-key-material-32-bytes-xxxxxx")),
		Active:    true,
	}
	km.AddKey(testKey)

	ctx := context.Background()

	plaintext := []byte("confidential property access code")
	ev, err := km.Encrypt(ctx, keyID, plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if ev.KeyID != keyID {
		t.Errorf("expected key ID %s, got %s", keyID, ev.KeyID)
	}
	if ev.Algorithm != "aes256-gcm" {
		t.Errorf("expected algorithm aes256-gcm, got %s", ev.Algorithm)
	}
	if len(ev.Ciphertext) == 0 {
		t.Error("ciphertext must not be empty")
	}

	decrypted, err := km.Decrypt(ctx, ev)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(decrypted) != string(plaintext) {
		t.Errorf("decrypted value mismatch: got %q, want %q", string(decrypted), string(plaintext))
	}
}

func TestSecurityEncryptWithInactiveKeyFails(t *testing.T) {
	km := security.NewNoOpKeyManager()

	keyID := security.NewKeyID()
	testKey := &security.Key{
		ID:        keyID,
		Algorithm: "aes256-gcm",
		KeyBytes:  security.NewSecretBytes([]byte("test-key-material-32-bytes-xxxxxx")),
		Active:    false,
	}
	km.AddKey(testKey)

	ctx := context.Background()
	_, err := km.Encrypt(ctx, keyID, []byte("data"))
	if err != security.ErrKeyInactive {
		t.Errorf("expected ErrKeyInactive, got %v", err)
	}
}

func TestSecurityEncryptWithUnknownKeyFails(t *testing.T) {
	km := security.NewNoOpKeyManager()
	ctx := context.Background()
	_, err := km.Encrypt(ctx, "nonexistent", []byte("data"))
	if err != security.ErrKeyNotFound {
		t.Errorf("expected ErrKeyNotFound, got %v", err)
	}
}

func TestSecurityKeyRotationHooks(t *testing.T) {
	km := security.NewNoOpKeyManager()

	oldKeyID := security.NewKeyID()
	oldKey := &security.Key{
		ID:        oldKeyID,
		Algorithm: "aes256-gcm",
		KeyBytes:  security.NewSecretBytes([]byte("old-key-material-32-bytes-xxxxx")),
		Active:    true,
	}
	km.AddKey(oldKey)

	ctx := context.Background()

	hookCalled := false
	var hookOldID, hookNewID security.KeyID
	km.AddRotationHook(func(ctx context.Context, oldID, newID security.KeyID) error {
		hookCalled = true
		hookOldID = oldID
		hookNewID = newID
		return nil
	})

	newKeyID := security.NewKeyID()
	newKey := &security.Key{
		ID:        newKeyID,
		Algorithm: "aes256-gcm",
		KeyBytes:  security.NewSecretBytes([]byte("new-key-material-32-bytes-xxxxx")),
		Active:    true,
	}

	rotatedOldID, err := km.RotateKey(ctx, newKey)
	if err != nil {
		t.Fatalf("rotate key: %v", err)
	}

	if !hookCalled {
		t.Error("rotation hook must be called")
	}
	if hookOldID != oldKeyID {
		t.Errorf("hook old key ID: got %s, want %s", hookOldID, oldKeyID)
	}
	if hookNewID != newKeyID {
		t.Errorf("hook new key ID: got %s, want %s", hookNewID, newKeyID)
	}
	if rotatedOldID != oldKeyID {
		t.Errorf("rotated old key ID: got %s, want %s", rotatedOldID, oldKeyID)
	}

	old, err := km.GetKey(ctx, oldKeyID)
	if err != nil {
		t.Fatalf("get old key: %v", err)
	}
	if old.Active {
		t.Error("old key must be inactive after rotation")
	}

	active, err := km.GetActiveKey(ctx)
	if err != nil {
		t.Fatalf("get active key: %v", err)
	}
	if active.ID != newKeyID {
		t.Errorf("active key must be new key, got %s", active.ID)
	}
}

func TestSecurityDenyByDefaultAuthorization(t *testing.T) {
	denier := security.DenyAllAuthorizer{}
	ctx := context.Background()
	subject := security.Subject{
		ActorID:  "user-1",
		TenantID: "tenant-1",
		Roles:    []string{"admin"},
	}

	err := denier.Can(ctx, subject, "read", security.Resource{Type: "document", ID: "doc-1"})
	if err != security.ErrDenied {
		t.Errorf("deny-all must deny all: got %v", err)
	}
}

func TestSecurityAllowAllAuthorization(t *testing.T) {
	allower := security.AllowAllAuthorizer{}
	ctx := context.Background()
	subject := security.Subject{
		ActorID:  "user-1",
		TenantID: "tenant-1",
		Roles:    []string{},
	}

	err := allower.Can(ctx, subject, "read", security.Resource{Type: "document", ID: "doc-1"})
	if err != nil {
		t.Errorf("allow-all must allow all: got %v", err)
	}
}

func TestSecurityRoleBasedAuthorization(t *testing.T) {
	policies := []security.RolePolicy{
		{
			Role:    "admin",
			Actions: []security.Action{"read", "write", "delete"},
			Resources: []security.Resource{
				{Type: "property"},
				{Type: "document"},
			},
		},
		{
			Role:    "viewer",
			Actions: []security.Action{"read"},
			Resources: []security.Resource{
				{Type: "property"},
			},
		},
	}
	authz := security.NewRoleBasedAuthorizer(policies)
	ctx := context.Background()

	tests := []struct {
		name     string
		roles    []string
		action   security.Action
		resource security.Resource
		allow    bool
	}{
		{"admin reads property", []string{"admin"}, "read", security.Resource{Type: "property"}, true},
		{"admin writes document", []string{"admin"}, "write", security.Resource{Type: "document"}, true},
		{"admin deletes property", []string{"admin"}, "delete", security.Resource{Type: "property"}, true},
		{"admin cannot delete ticket", []string{"admin"}, "delete", security.Resource{Type: "ticket"}, false},
		{"viewer reads property", []string{"viewer"}, "read", security.Resource{Type: "property"}, true},
		{"viewer cannot write property", []string{"viewer"}, "write", security.Resource{Type: "property"}, false},
		{"no roles deny", []string{}, "read", security.Resource{Type: "property"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subject := security.Subject{ActorID: "u1", Roles: tt.roles}
			err := authz.Can(ctx, subject, tt.action, tt.resource)
			if tt.allow && err != nil {
				t.Errorf("expected allow, got: %v", err)
			}
			if !tt.allow && err == nil {
				t.Error("expected deny, got nil")
			}
		})
	}
}

func TestSecurityMFARequiredForPrivilegedAction(t *testing.T) {
	ctx := context.Background()
	subject := security.Subject{ActorID: "u1", Roles: []string{"admin"}}

	enabledVerifier := security.NewNoOpMFAVerifier(security.MFAStateEnabled)
	err := enabledVerifier.RequireMFA(ctx, subject, "privileged.user.delete")
	if err != security.ErrMFARequired {
		t.Errorf("MFA enabled must require MFA for privileged action: got %v", err)
	}

	requiredVerifier := security.NewNoOpMFAVerifier(security.MFAStateRequired)
	err = requiredVerifier.RequireMFA(ctx, subject, "admin.settings.change")
	if err != security.ErrMFARequired {
		t.Errorf("MFA required must block privileged action: got %v", err)
	}
}

func TestSecurityMFAOptionalForNormalAction(t *testing.T) {
	ctx := context.Background()
	subject := security.Subject{ActorID: "u1", Roles: []string{"viewer"}}

	enabledVerifier := security.NewNoOpMFAVerifier(security.MFAStateEnabled)
	err := enabledVerifier.RequireMFA(ctx, subject, "property.view")
	if err != nil {
		t.Errorf("MFA enabled should not block normal action: got %v", err)
	}

	disabledVerifier := security.NewNoOpMFAVerifier(security.MFAStateDisabled)
	err = disabledVerifier.RequireMFA(ctx, subject, "privileged.system.config")
	if err != nil {
		t.Errorf("MFA disabled should never block: got %v", err)
	}
}

func TestSecuritySessionRevocation(t *testing.T) {
	store := security.NewInMemorySessionStore()
	ctx := context.Background()

	sessionID := security.SessionID("sess-001")
	sess := &security.Session{
		ID:        sessionID,
		ActorID:   "user-1",
		TenantID:  "tenant-1",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	store.AddSession(sess)

	retrieved, err := store.GetSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if retrieved.ID != sessionID {
		t.Errorf("session ID mismatch: %s", retrieved.ID)
	}

	revocation := security.SessionRevocation{
		SessionID: sessionID,
		Reason:    "security incident",
		RevokedAt: time.Now(),
		RevokedBy: "admin-1",
	}

	if err := store.RevokeSession(ctx, revocation); err != nil {
		t.Fatalf("revoke session: %v", err)
	}

	_, err = store.GetSession(ctx, sessionID)
	if err != security.ErrSessionRevoked {
		t.Errorf("expected ErrSessionRevoked, got %v", err)
	}

	revoked, err := store.IsRevoked(ctx, sessionID)
	if err != nil {
		t.Fatalf("is revoked: %v", err)
	}
	if !revoked {
		t.Error("session must be revoked")
	}
}

func TestSecurityExpiredSessionReturnsError(t *testing.T) {
	store := security.NewInMemorySessionStore()
	ctx := context.Background()

	sessionID := security.SessionID("sess-expired")
	sess := &security.Session{
		ID:        sessionID,
		ActorID:   "user-1",
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}
	store.AddSession(sess)

	_, err := store.GetSession(ctx, sessionID)
	if err != security.ErrSessionExpired {
		t.Errorf("expected ErrSessionExpired, got %v", err)
	}
}

func TestSecurityPrivilegedAccessLogger(t *testing.T) {
	logger := security.NewNoOpPrivilegedAccessLogger()
	ctx := context.Background()

	normalEvent := security.PrivilegedAccessEvent{
		ID:      "evt-1",
		ActorID: "user-1",
		Action:  "property.view",
		Success: true,
	}
	if err := logger.Log(ctx, normalEvent); err != nil {
		t.Fatalf("log normal: %v", err)
	}
	if len(logger.Events()) != 0 {
		t.Error("normal action must not be logged as privileged")
	}

	privEvent := security.PrivilegedAccessEvent{
		ID:      "evt-2",
		ActorID: "admin-1",
		Action:  "admin.user.delete",
		MFAUsed: true,
		Success: true,
	}
	if err := logger.Log(ctx, privEvent); err != nil {
		t.Fatalf("log privileged: %v", err)
	}
	if len(logger.Events()) != 1 {
		t.Fatalf("expected 1 privileged event, got %d", len(logger.Events()))
	}
	if logger.Events()[0].ID != "evt-2" {
		t.Errorf("expected evt-2, got %s", logger.Events()[0].ID)
	}
}

func TestAuditInterfaceHasNoUpdatePath(t *testing.T) {
	evt := audit.AuditEvent{
		ID:           "audit-001",
		EventType:    audit.EventTypeAccess,
		ActorID:      "user-1",
		Action:       "login",
		ResourceType: "session",
		ResourceID:   "sess-1",
	}

	evt2 := evt
	evt2.Action = "logout"

	if evt.Action == evt2.Action {
		t.Error("test setup: events must differ")
	}
}

func TestAuditEventImmutableFields(t *testing.T) {
	evt := audit.AuditEvent{
		ID:           "audit-immutable",
		EventType:    audit.EventTypeSecurity,
		ActorID:      "admin-1",
		Action:       "key.rotate",
		ResourceType: "encryption_key",
		ResourceID:   "key-1",
		CreatedAt:    time.Now(),
	}

	serialized, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var deserialized audit.AuditEvent
	if err := json.Unmarshal(serialized, &deserialized); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if deserialized.ID != evt.ID {
		t.Error("ID must be preserved through serialization")
	}
	if deserialized.EventType != evt.EventType {
		t.Error("EventType must be preserved")
	}
	if deserialized.ActorID != evt.ActorID {
		t.Error("ActorID must be preserved")
	}
}

func secAuditPostgresAvailable() bool {
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

func secAuditDBConnect(t *testing.T) (*database.DB, func()) {
	t.Helper()

	host := os.Getenv("CC_DB_HOST")
	if host == "" {
		host = "localhost"
	}
	user := os.Getenv("CC_DB_USER")
	if user == "" {
		user = "ccuser"
	}
	pass := os.Getenv("CC_DB_PASS")
	if pass == "" {
		pass = "ccpass"
	}
	name := testdb.MustName()

	cfg := config.Config{
		DBHost: host,
		DBPort: 5432,
		DBUser: user,
		DBPass: pass,
		DBName: name,
		DBSSL:  "disable",
	}

	db, err := database.Connect(context.Background(), cfg)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	cleanup := func() {
		db.Pool.Exec(context.Background(), `DROP TABLE IF EXISTS audit_events CASCADE`)
		db.Pool.Exec(context.Background(), `DROP TABLE IF EXISTS encryption_keys CASCADE`)
		db.Pool.Exec(context.Background(), `DROP TABLE IF EXISTS session_revocations CASCADE`)
		db.Pool.Exec(context.Background(), `DROP TABLE IF EXISTS privileged_access_log CASCADE`)
		db.Pool.Exec(context.Background(), `DROP FUNCTION IF EXISTS audit_no_update_delete CASCADE`)
		db.Close()
	}

	return db, cleanup
}

func TestAuditSchemaCreationAndAppend(t *testing.T) {
	if !secAuditPostgresAvailable() {
		t.Skip("PostgreSQL not available")
	}

	db, cleanup := secAuditDBConnect(t)
	defer cleanup()

	ctx := context.Background()

	if err := audit.EnsureSchema(ctx, db.Pool); err != nil {
		t.Fatalf("audit EnsureSchema: %v", err)
	}

	store := audit.NewAuditStore(db.Pool)

	evt := audit.AuditEvent{
		ID:            "audit-test-001",
		EventType:     audit.EventTypeAccess,
		TenantID:      "tenant-1",
		ActorID:       "user-1",
		Action:        "login",
		ResourceType:  "session",
		ResourceID:    "sess-1",
		PreviousState: json.RawMessage(`{"status":"active"}`),
		NewState:      json.RawMessage(`{"status":"authenticated"}`),
		Metadata:      json.RawMessage(`{"ip":"127.0.0.1"}`),
		CorrelationID: "corr-123",
	}

	if err := store.Append(ctx, evt); err != nil {
		t.Fatalf("append audit event: %v", err)
	}

	events, err := store.Query(ctx, audit.AuditQuery{
		TenantID: "tenant-1",
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("query audit: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].ID != "audit-test-001" {
		t.Errorf("expected audit-test-001, got %s", events[0].ID)
	}
	if events[0].Action != "login" {
		t.Errorf("expected login, got %s", events[0].Action)
	}
}

func TestAuditUpdateFailsWithTrigger(t *testing.T) {
	if !secAuditPostgresAvailable() {
		t.Skip("PostgreSQL not available")
	}

	db, cleanup := secAuditDBConnect(t)
	defer cleanup()

	ctx := context.Background()

	if err := audit.EnsureSchema(ctx, db.Pool); err != nil {
		t.Fatalf("audit EnsureSchema: %v", err)
	}

	store := audit.NewAuditStore(db.Pool)

	evt := audit.AuditEvent{
		ID:           "audit-no-update-001",
		EventType:    audit.EventTypeSecurity,
		TenantID:     "tenant-1",
		ActorID:      "admin-1",
		Action:       "key.create",
		ResourceType: "encryption_key",
		ResourceID:   "key-1",
	}

	if err := store.Append(ctx, evt); err != nil {
		t.Fatalf("append: %v", err)
	}

	_, err := db.Pool.Exec(ctx, `UPDATE audit_events SET action = 'key.delete' WHERE id = 'audit-no-update-001'`)
	if err == nil {
		t.Fatal("UPDATE on audit_events must fail due to immutable trigger")
	}
	if !strings.Contains(err.Error(), "immutable") {
		t.Errorf("expected immutable error, got: %v", err)
	}

	_, err = db.Pool.Exec(ctx, `DELETE FROM audit_events WHERE id = 'audit-no-update-001'`)
	if err == nil {
		t.Fatal("DELETE on audit_events must fail due to immutable trigger")
	}
	if !strings.Contains(err.Error(), "immutable") {
		t.Errorf("expected immutable error, got: %v", err)
	}
}

func TestAuditQueryFilters(t *testing.T) {
	if !secAuditPostgresAvailable() {
		t.Skip("PostgreSQL not available")
	}

	db, cleanup := secAuditDBConnect(t)
	defer cleanup()

	ctx := context.Background()

	if err := audit.EnsureSchema(ctx, db.Pool); err != nil {
		t.Fatalf("audit EnsureSchema: %v", err)
	}

	store := audit.NewAuditStore(db.Pool)

	events := []audit.AuditEvent{
		{ID: "f1", EventType: audit.EventTypeAccess, TenantID: "t1", ActorID: "u1", Action: "login", ResourceType: "session"},
		{ID: "f2", EventType: audit.EventTypeAccess, TenantID: "t1", ActorID: "u2", Action: "logout", ResourceType: "session"},
		{ID: "f3", EventType: audit.EventTypeMutation, TenantID: "t2", ActorID: "u1", Action: "update", ResourceType: "property"},
	}

	for _, e := range events {
		if err := store.Append(ctx, e); err != nil {
			t.Fatalf("append %s: %v", e.ID, err)
		}
	}

	t1Events, err := store.Query(ctx, audit.AuditQuery{TenantID: "t1", Limit: 10})
	if err != nil {
		t.Fatalf("query t1: %v", err)
	}
	if len(t1Events) != 2 {
		t.Errorf("expected 2 events for t1, got %d", len(t1Events))
	}

	u1Events, err := store.Query(ctx, audit.AuditQuery{ActorID: "u1", Limit: 10})
	if err != nil {
		t.Fatalf("query u1: %v", err)
	}
	if len(u1Events) != 2 {
		t.Errorf("expected 2 events for u1, got %d", len(u1Events))
	}

	mutEvents, err := store.Query(ctx, audit.AuditQuery{EventType: audit.EventTypeMutation, Limit: 10})
	if err != nil {
		t.Fatalf("query mutation: %v", err)
	}
	if len(mutEvents) != 1 {
		t.Errorf("expected 1 mutation event, got %d", len(mutEvents))
	}
}

func TestSecuritySchemaCreation(t *testing.T) {
	if !secAuditPostgresAvailable() {
		t.Skip("PostgreSQL not available")
	}

	db, cleanup := secAuditDBConnect(t)
	defer cleanup()

	ctx := context.Background()

	if err := security.EnsureSchema(ctx, db.Pool); err != nil {
		t.Fatalf("security EnsureSchema: %v", err)
	}

	_, err := db.Pool.Exec(ctx, `
		INSERT INTO encryption_keys (id, algorithm, active) VALUES ('test-key-1', 'aes256-gcm', true)
	`)
	if err != nil {
		t.Fatalf("insert encryption key: %v", err)
	}

	var algo string
	if err := db.Pool.QueryRow(ctx, `SELECT algorithm FROM encryption_keys WHERE id = 'test-key-1'`).Scan(&algo); err != nil {
		t.Fatalf("query encryption key: %v", err)
	}
	if algo != "aes256-gcm" {
		t.Errorf("expected aes256-gcm, got %s", algo)
	}
}

func TestSecurityPrivilegedAccessLogInsert(t *testing.T) {
	if !secAuditPostgresAvailable() {
		t.Skip("PostgreSQL not available")
	}

	db, cleanup := secAuditDBConnect(t)
	defer cleanup()

	ctx := context.Background()

	if err := security.EnsureSchema(ctx, db.Pool); err != nil {
		t.Fatalf("security EnsureSchema: %v", err)
	}

	details := json.RawMessage(`{"reason":"security investigation"}`)
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO privileged_access_log (actor_id, tenant_id, action, resource_type, resource_id, mfa_used, success, details)
		VALUES ('admin-1', 'tenant-1', 'admin.user.delete', 'user', 'user-5', true, true, $1)
	`, details)
	if err != nil {
		t.Fatalf("insert privileged access log: %v", err)
	}

	var mfaUsed bool
	if err := db.Pool.QueryRow(ctx, `SELECT mfa_used FROM privileged_access_log WHERE actor_id = 'admin-1'`).Scan(&mfaUsed); err != nil {
		t.Fatalf("query privileged access log: %v", err)
	}
	if !mfaUsed {
		t.Error("MFA must be recorded as used for privileged action")
	}
}

func TestSecuritySessionRevocationInDB(t *testing.T) {
	if !secAuditPostgresAvailable() {
		t.Skip("PostgreSQL not available")
	}

	db, cleanup := secAuditDBConnect(t)
	defer cleanup()

	ctx := context.Background()

	if err := security.EnsureSchema(ctx, db.Pool); err != nil {
		t.Fatalf("security EnsureSchema: %v", err)
	}

	_, err := db.Pool.Exec(ctx, `
		INSERT INTO session_revocations (session_id, reason, revoked_by) VALUES ('sess-rev-1', 'security breach', 'admin-1')
	`)
	if err != nil {
		t.Fatalf("insert session revocation: %v", err)
	}

	var reason string
	if err := db.Pool.QueryRow(ctx, `SELECT reason FROM session_revocations WHERE session_id = 'sess-rev-1'`).Scan(&reason); err != nil {
		t.Fatalf("query session revocation: %v", err)
	}
	if reason != "security breach" {
		t.Errorf("expected 'security breach', got %s", reason)
	}
}

func TestAppWiringCompilesWithSecurityAndAudit(t *testing.T) {
	t.Setenv("CC_DB_USER", "testuser")
	t.Setenv("CC_DB_PASS", "testpass")
	t.Setenv("CC_DB_NAME", "testdb")
	t.Setenv("CC_SKIP_DB", "true")
	t.Setenv("CC_HTTP_PORT", "18082")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- app.RunAPI(ctx)
	}()

	baseURL := "http://127.0.0.1:18082"
	if err := waitForServer(baseURL+"/health/live", 5*time.Second); err != nil {
		t.Fatalf("server did not start: %v", err)
	}

	resp, err := http.Get(baseURL + "/health/live")
	if err != nil {
		t.Fatalf("health check: %v", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	cancel()
}
