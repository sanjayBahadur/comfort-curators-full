package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"comfort-curators-backend/internal/platform/config"
	"comfort-curators-backend/internal/platform/database"
	"comfort-curators-backend/internal/platform/files"
)

func filesPostgresAvailable() bool {
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

func filesDBConfig() (config.Config, *database.DB, bool) {
	if !filesPostgresAvailable() {
		return config.Config{}, nil, false
	}

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
	name := os.Getenv("CC_DB_NAME")
	if name == "" {
		name = "comfort_curators"
	}

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
		return cfg, nil, false
	}

	if _, err := db.Pool.Exec(context.Background(), `SELECT 1`); err != nil {
		db.Close()
		return cfg, nil, false
	}

	return cfg, db, true
}

func ensureFilesTables(ctx context.Context, db *database.DB) error {
	if err := database.RunMigrations(ctx, db); err != nil {
		return err
	}
	if err := files.Migrate(ctx, db.Pool); err != nil {
		return err
	}
	return nil
}

func cleanupFilesTables(ctx context.Context, db *database.DB) {
	db.Pool.Exec(ctx, `DELETE FROM file_grants`)
	db.Pool.Exec(ctx, `DELETE FROM file_objects`)
}

func TestFilesCrossTenantAccessFailsClosed(t *testing.T) {
	_, db, ok := filesDBConfig()
	if !ok {
		t.Skip("PostgreSQL not available")
	}
	defer db.Close()

	ctx := context.Background()
	if err := ensureFilesTables(ctx, db); err != nil {
		t.Fatalf("ensure tables: %v", err)
	}
	defer cleanupFilesTables(ctx, db)

	store := files.NewFileStore(db.Pool, files.DefaultConfig())

	tenantA := "aaaaaaaa-0000-0000-0000-0000000000a1"
	tenantB := "bbbbbbbb-0000-0000-0000-0000000000b2"

	obj, err := store.CreateObject(ctx, tenantA,
		"uploads/doc-a.pdf", "document.pdf", "application/pdf",
		1024, "abc123def456", nil,
	)
	if err != nil {
		t.Fatalf("create object for tenant A: %v", err)
	}
	t.Logf("created object %s for tenant %s", obj.ID, tenantA)

	_, err = store.GetObject(ctx, tenantB, obj.ID)
	if err != files.ErrNotFound {
		t.Errorf("cross-tenant GetObject: expected ErrNotFound, got %v", err)
	}

	_, err = store.GetAvailableObject(ctx, tenantB, obj.ID)
	if err != files.ErrNotFound {
		t.Errorf("cross-tenant GetAvailableObject: expected ErrNotFound, got %v", err)
	}

	err = store.UpdateScanStatus(ctx, tenantB, obj.ID, files.ScanStatusScanning)
	if err != files.ErrNotFound {
		t.Errorf("cross-tenant UpdateScanStatus: expected ErrNotFound, got %v", err)
	}

	err = store.SoftDeleteObject(ctx, tenantB, obj.ID)
	if err != files.ErrNotFound {
		t.Errorf("cross-tenant SoftDeleteObject: expected ErrNotFound, got %v", err)
	}

	objs, err := store.ListObjects(ctx, tenantB, 10, 0)
	if err != nil {
		t.Fatalf("list objects for tenant B: %v", err)
	}
	if len(objs) != 0 {
		t.Errorf("tenant B should not see tenant A objects, got %d", len(objs))
	}

	_, err = store.CreateDownloadGrant(ctx, tenantB, obj.ID)
	if err != files.ErrNotFound {
		t.Errorf("cross-tenant CreateDownloadGrant: expected ErrNotFound, got %v", err)
	}
}

func TestFilesGrantsExpire(t *testing.T) {
	_, db, ok := filesDBConfig()
	if !ok {
		t.Skip("PostgreSQL not available")
	}
	defer db.Close()

	ctx := context.Background()
	if err := ensureFilesTables(ctx, db); err != nil {
		t.Fatalf("ensure tables: %v", err)
	}
	defer cleanupFilesTables(ctx, db)

	tenantID := "cccccccc-0000-0000-0000-0000000000c3"

	shortCfg := files.DefaultConfig()
	shortCfg.UploadGrantTTL = 1 * time.Second
	shortCfg.DownloadGrantTTL = 1 * time.Second

	store := files.NewFileStore(db.Pool, shortCfg)

	uploadGrant, err := store.CreateUploadGrant(ctx, tenantID, nil, nil)
	if err != nil {
		t.Fatalf("create upload grant: %v", err)
	}
	t.Logf("created upload grant with token %s, expires at %s",
		uploadGrant.GrantToken, uploadGrant.ExpiresAt.Format(time.RFC3339))

	_, err = store.ValidateUploadGrant(ctx, uploadGrant.GrantToken, 100, "text/plain")
	if err != nil {
		t.Fatalf("validate upload grant before expiry: %v", err)
	}
	t.Log("upload grant validated before expiry")

	uploadGrant2, err := store.CreateUploadGrant(ctx, tenantID, nil, nil)
	if err != nil {
		t.Fatalf("create second upload grant: %v", err)
	}

	time.Sleep(1500 * time.Millisecond)

	_, err = store.ValidateUploadGrant(ctx, uploadGrant2.GrantToken, 100, "text/plain")
	if err != files.ErrGrantExpired {
		t.Errorf("expected ErrGrantExpired for expired upload grant, got: %v", err)
	}

	obj, err := store.CreateObject(ctx, tenantID,
		"uploads/doc-expire.pdf", "expire.pdf", "application/pdf",
		512, "hash-expire-001", nil,
	)
	if err != nil {
		t.Fatalf("create object: %v", err)
	}

	err = store.UpdateScanStatus(ctx, tenantID, obj.ID, files.ScanStatusScanning)
	if err != nil {
		t.Fatalf("update scan status to scanning: %v", err)
	}
	err = store.UpdateScanStatus(ctx, tenantID, obj.ID, files.ScanStatusClean)
	if err != nil {
		t.Fatalf("update scan status to clean: %v", err)
	}

	downloadGrant, err := store.CreateDownloadGrant(ctx, tenantID, obj.ID)
	if err != nil {
		t.Fatalf("create download grant: %v", err)
	}
	t.Logf("created download grant with token %s", downloadGrant.GrantToken)

	_, _, err = store.ValidateDownloadGrant(ctx, tenantID, downloadGrant.GrantToken)
	if err != nil {
		t.Fatalf("validate download grant before expiry: %v", err)
	}
	t.Log("download grant validated before expiry")

	downloadGrant2, err := store.CreateDownloadGrant(ctx, tenantID, obj.ID)
	if err != nil {
		t.Fatalf("create second download grant: %v", err)
	}

	time.Sleep(1500 * time.Millisecond)

	_, _, err = store.ValidateDownloadGrant(ctx, tenantID, downloadGrant2.GrantToken)
	if err != files.ErrGrantExpired {
		t.Errorf("expected ErrGrantExpired for expired download grant, got: %v", err)
	}
}

func TestFilesUnscannedObjectsNotAvailable(t *testing.T) {
	_, db, ok := filesDBConfig()
	if !ok {
		t.Skip("PostgreSQL not available")
	}
	defer db.Close()

	ctx := context.Background()
	if err := ensureFilesTables(ctx, db); err != nil {
		t.Fatalf("ensure tables: %v", err)
	}
	defer cleanupFilesTables(ctx, db)

	store := files.NewFileStore(db.Pool, files.DefaultConfig())

	tenantID := "dddddddd-0000-0000-0000-0000000000d4"

	obj, err := store.CreateObject(ctx, tenantID,
		"uploads/unscanned.pdf", "unscanned.pdf", "application/pdf",
		2048, "hash-unscanned-001", nil,
	)
	if err != nil {
		t.Fatalf("create object: %v", err)
	}

	if obj.ScanStatus != files.ScanStatusUnscanned {
		t.Errorf("new object should be unscanned, got %s", obj.ScanStatus)
	}

	_, err = store.GetAvailableObject(ctx, tenantID, obj.ID)
	if err == nil {
		t.Fatal("expected error when getting unscanned object for download")
	}
	if !isErrContains(err, files.ErrUnscannedObject.Error()) {
		t.Errorf("expected ErrUnscannedObject, got: %v", err)
	}

	_, err = store.CreateDownloadGrant(ctx, tenantID, obj.ID)
	if err == nil {
		t.Fatal("expected error when creating download grant for unscanned object")
	}
	if !isErrContains(err, files.ErrUnscannedObject.Error()) {
		t.Errorf("expected ErrUnscannedObject, got: %v", err)
	}

	err = store.UpdateScanStatus(ctx, tenantID, obj.ID, files.ScanStatusScanning)
	if err != nil {
		t.Fatalf("update scan status to scanning: %v", err)
	}

	_, err = store.GetAvailableObject(ctx, tenantID, obj.ID)
	if err == nil {
		t.Fatal("expected error when getting scanning object for download")
	}

	err = store.UpdateScanStatus(ctx, tenantID, obj.ID, files.ScanStatusInfected)
	if err != nil {
		t.Fatalf("update scan status to infected: %v", err)
	}

	_, err = store.GetAvailableObject(ctx, tenantID, obj.ID)
	if err == nil {
		t.Fatal("expected error when getting infected object for download")
	}

	err = store.UpdateScanStatus(ctx, tenantID, obj.ID, files.ScanStatusScanning)
	if err != nil {
		t.Fatalf("update scan status from infected to scanning: %v", err)
	}

	err = store.UpdateScanStatus(ctx, tenantID, obj.ID, files.ScanStatusClean)
	if err != nil {
		t.Fatalf("update scan status to clean: %v", err)
	}

	availableObj, err := store.GetAvailableObject(ctx, tenantID, obj.ID)
	if err != nil {
		t.Fatalf("get clean object for download: %v", err)
	}
	if availableObj.ScanStatus != files.ScanStatusClean {
		t.Errorf("expected scan status clean, got %s", availableObj.ScanStatus)
	}

	downloadGrant, err := store.CreateDownloadGrant(ctx, tenantID, obj.ID)
	if err != nil {
		t.Fatalf("create download grant for clean object: %v", err)
	}
	t.Logf("download grant created for clean object: %s", downloadGrant.GrantToken)
}

func TestFilesCreateAndValidateUploadGrant(t *testing.T) {
	_, db, ok := filesDBConfig()
	if !ok {
		t.Skip("PostgreSQL not available")
	}
	defer db.Close()

	ctx := context.Background()
	if err := ensureFilesTables(ctx, db); err != nil {
		t.Fatalf("ensure tables: %v", err)
	}
	defer cleanupFilesTables(ctx, db)

	store := files.NewFileStore(db.Pool, files.DefaultConfig())

	tenantID := "eeeeeeee-0000-0000-0000-0000000000e5"

	maxSize := int64(10 * 1024 * 1024)
	allowedTypes := []string{"image/jpeg", "image/png"}

	grant, err := store.CreateUploadGrant(ctx, tenantID, &maxSize, allowedTypes)
	if err != nil {
		t.Fatalf("create upload grant: %v", err)
	}

	if grant.GrantType != files.GrantTypeUpload {
		t.Errorf("expected upload grant type, got %s", grant.GrantType)
	}
	if grant.MaxSizeBytes == nil || *grant.MaxSizeBytes != maxSize {
		t.Errorf("max_size_bytes mismatch")
	}
	if len(grant.AllowedContentTypes) != 2 {
		t.Errorf("expected 2 allowed content types, got %d", len(grant.AllowedContentTypes))
	}
	if grant.UsedAt != nil {
		t.Error("new grant should not be used")
	}

	validated, err := store.ValidateUploadGrant(ctx, grant.GrantToken, 5*1024*1024, "image/jpeg")
	if err != nil {
		t.Fatalf("validate upload grant: %v", err)
	}
	if validated.UsedAt == nil {
		t.Error("grant should be marked as used after validation")
	}

	_, err = store.ValidateUploadGrant(ctx, grant.GrantToken, 100, "image/png")
	if err != files.ErrGrantUsed {
		t.Errorf("expected ErrGrantUsed for consumed grant, got: %v", err)
	}
}

func TestFilesUploadGrantSizeAndTypeValidation(t *testing.T) {
	_, db, ok := filesDBConfig()
	if !ok {
		t.Skip("PostgreSQL not available")
	}
	defer db.Close()

	ctx := context.Background()
	if err := ensureFilesTables(ctx, db); err != nil {
		t.Fatalf("ensure tables: %v", err)
	}
	defer cleanupFilesTables(ctx, db)

	store := files.NewFileStore(db.Pool, files.DefaultConfig())

	tenantID := "ffffffff-0000-0000-0000-0000000000f6"

	maxSize := int64(1000)
	allowedTypes := []string{"image/jpeg", "application/pdf"}

	grant, err := store.CreateUploadGrant(ctx, tenantID, &maxSize, allowedTypes)
	if err != nil {
		t.Fatalf("create upload grant: %v", err)
	}

	_, err = store.ValidateUploadGrant(ctx, grant.GrantToken, 2000, "image/jpeg")
	if err != files.ErrSizeLimitExceeded {
		t.Errorf("expected ErrSizeLimitExceeded, got: %v", err)
	}

	_, err = store.ValidateUploadGrant(ctx, grant.GrantToken, 500, "text/html")
	if err != files.ErrContentTypeDenied {
		t.Errorf("expected ErrContentTypeDenied, got: %v", err)
	}

	validated, err := store.ValidateUploadGrant(ctx, grant.GrantToken, 500, "image/jpeg")
	if err != nil {
		t.Fatalf("validate with correct size and type: %v", err)
	}
	if validated.UsedAt == nil {
		t.Error("grant should be consumed")
	}
}

func TestFilesCreateAndValidateDownloadGrant(t *testing.T) {
	_, db, ok := filesDBConfig()
	if !ok {
		t.Skip("PostgreSQL not available")
	}
	defer db.Close()

	ctx := context.Background()
	if err := ensureFilesTables(ctx, db); err != nil {
		t.Fatalf("ensure tables: %v", err)
	}
	defer cleanupFilesTables(ctx, db)

	store := files.NewFileStore(db.Pool, files.DefaultConfig())

	tenantID := "11111111-0000-0000-0000-000000000001"

	obj, err := store.CreateObject(ctx, tenantID,
		"uploads/photo.jpg", "photo.jpg", "image/jpeg",
		4096, "hash-photo-001", nil,
	)
	if err != nil {
		t.Fatalf("create object: %v", err)
	}

	err = store.UpdateScanStatus(ctx, tenantID, obj.ID, files.ScanStatusScanning)
	if err != nil {
		t.Fatalf("update to scanning: %v", err)
	}
	err = store.UpdateScanStatus(ctx, tenantID, obj.ID, files.ScanStatusClean)
	if err != nil {
		t.Fatalf("update to clean: %v", err)
	}

	grant, err := store.CreateDownloadGrant(ctx, tenantID, obj.ID)
	if err != nil {
		t.Fatalf("create download grant: %v", err)
	}

	if grant.GrantType != files.GrantTypeDownload {
		t.Errorf("expected download grant type, got %s", grant.GrantType)
	}
	if grant.ObjectID == nil || *grant.ObjectID != obj.ID {
		t.Errorf("object_id mismatch")
	}

	validatedGrant, returnedObj, err := store.ValidateDownloadGrant(ctx, tenantID, grant.GrantToken)
	if err != nil {
		t.Fatalf("validate download grant: %v", err)
	}
	if validatedGrant.UsedAt == nil {
		t.Error("grant should be marked as used")
	}
	if returnedObj.ID != obj.ID {
		t.Errorf("returned object ID mismatch")
	}

	_, _, err = store.ValidateDownloadGrant(ctx, tenantID, grant.GrantToken)
	if err != files.ErrGrantUsed {
		t.Errorf("expected ErrGrantUsed, got: %v", err)
	}
}

func TestFilesDownloadGrantCrossTenantFails(t *testing.T) {
	_, db, ok := filesDBConfig()
	if !ok {
		t.Skip("PostgreSQL not available")
	}
	defer db.Close()

	ctx := context.Background()
	if err := ensureFilesTables(ctx, db); err != nil {
		t.Fatalf("ensure tables: %v", err)
	}
	defer cleanupFilesTables(ctx, db)

	store := files.NewFileStore(db.Pool, files.DefaultConfig())

	tenantA := "22222222-0000-0000-0000-000000000002"
	tenantB := "33333333-0000-0000-0000-000000000003"

	obj, err := store.CreateObject(ctx, tenantA,
		"uploads/cross.pdf", "cross.pdf", "application/pdf",
		1024, "hash-cross-001", nil,
	)
	if err != nil {
		t.Fatalf("create object for tenant A: %v", err)
	}

	err = store.UpdateScanStatus(ctx, tenantA, obj.ID, files.ScanStatusScanning)
	if err != nil {
		t.Fatalf("update to scanning: %v", err)
	}
	err = store.UpdateScanStatus(ctx, tenantA, obj.ID, files.ScanStatusClean)
	if err != nil {
		t.Fatalf("update to clean: %v", err)
	}

	grant, err := store.CreateDownloadGrant(ctx, tenantA, obj.ID)
	if err != nil {
		t.Fatalf("create download grant for tenant A: %v", err)
	}

	_, _, err = store.ValidateDownloadGrant(ctx, tenantB, grant.GrantToken)
	if err != files.ErrNotFound {
		t.Errorf("expected ErrNotFound when tenant B uses tenant A's download grant, got: %v", err)
	}
}

func TestFilesObjectLifecycle(t *testing.T) {
	_, db, ok := filesDBConfig()
	if !ok {
		t.Skip("PostgreSQL not available")
	}
	defer db.Close()

	ctx := context.Background()
	if err := ensureFilesTables(ctx, db); err != nil {
		t.Fatalf("ensure tables: %v", err)
	}
	defer cleanupFilesTables(ctx, db)

	store := files.NewFileStore(db.Pool, files.DefaultConfig())

	tenantID := "44444444-0000-0000-0000-000000000004"

	obj, err := store.CreateObject(ctx, tenantID,
		"uploads/lifecycle.txt", "lifecycle.txt", "text/plain",
		100, "hash-lifecycle-001", nil,
	)
	if err != nil {
		t.Fatalf("create object: %v", err)
	}

	fetched, err := store.GetObject(ctx, tenantID, obj.ID)
	if err != nil {
		t.Fatalf("get object: %v", err)
	}
	if fetched.ObjectKey != "uploads/lifecycle.txt" {
		t.Errorf("object key mismatch")
	}

	err = store.SoftDeleteObject(ctx, tenantID, obj.ID)
	if err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	_, err = store.GetObject(ctx, tenantID, obj.ID)
	if err != files.ErrNotFound {
		t.Errorf("expected ErrNotFound after soft delete, got %v", err)
	}

	err = store.SoftDeleteObject(ctx, tenantID, obj.ID)
	if err != files.ErrNotFound {
		t.Errorf("expected ErrNotFound for already-deleted object, got %v", err)
	}
}

func TestFilesScanStatusTransitions(t *testing.T) {
	_, db, ok := filesDBConfig()
	if !ok {
		t.Skip("PostgreSQL not available")
	}
	defer db.Close()

	ctx := context.Background()
	if err := ensureFilesTables(ctx, db); err != nil {
		t.Fatalf("ensure tables: %v", err)
	}
	defer cleanupFilesTables(ctx, db)

	store := files.NewFileStore(db.Pool, files.DefaultConfig())

	tenantID := "55555555-0000-0000-0000-000000000005"

	obj, err := store.CreateObject(ctx, tenantID,
		"uploads/scan-test.pdf", "scan-test.pdf", "application/pdf",
		512, "hash-scan-001", nil,
	)
	if err != nil {
		t.Fatalf("create object: %v", err)
	}
	if obj.ScanStatus != files.ScanStatusUnscanned {
		t.Errorf("new object should be unscanned")
	}

	err = store.UpdateScanStatus(ctx, tenantID, obj.ID, files.ScanStatusScanning)
	if err != nil {
		t.Fatalf("unscanned -> scanning: %v", err)
	}

	err = store.UpdateScanStatus(ctx, tenantID, obj.ID, files.ScanStatusClean)
	if err != nil {
		t.Fatalf("scanning -> clean: %v", err)
	}

	err = store.UpdateScanStatus(ctx, tenantID, obj.ID, files.ScanStatusInfected)
	if err != files.ErrScanStatusConflict {
		t.Errorf("expected ErrScanStatusConflict for clean -> infected, got: %v", err)
	}

	err = store.UpdateScanStatus(ctx, tenantID, obj.ID, files.ScanStatusClean)
	if err != files.ErrScanStatusConflict {
		t.Errorf("expected ErrScanStatusConflict for clean -> clean (idempotent should be ok), got: %v", err)
	}

	obj2, err := store.CreateObject(ctx, tenantID,
		"uploads/scan-test2.pdf", "scan-test2.pdf", "application/pdf",
		512, "hash-scan-002", nil,
	)
	if err != nil {
		t.Fatalf("create second object: %v", err)
	}

	err = store.UpdateScanStatus(ctx, tenantID, obj2.ID, files.ScanStatusScanning)
	if err != nil {
		t.Fatalf("unscanned -> scanning: %v", err)
	}
	err = store.UpdateScanStatus(ctx, tenantID, obj2.ID, files.ScanStatusInfected)
	if err != nil {
		t.Fatalf("scanning -> infected: %v", err)
	}

	err = store.UpdateScanStatus(ctx, tenantID, obj2.ID, files.ScanStatusScanning)
	if err != nil {
		t.Fatalf("infected -> scanning: %v", err)
	}
	err = store.UpdateScanStatus(ctx, tenantID, obj2.ID, files.ScanStatusUnscannable)
	if err != nil {
		t.Fatalf("scanning -> unscannable: %v", err)
	}

	err = store.UpdateScanStatus(ctx, tenantID, obj2.ID, files.ScanStatusScanning)
	if err != files.ErrScanStatusConflict {
		t.Errorf("expected ErrScanStatusConflict for unscannable -> scanning: %v", err)
	}
}

func TestFilesContentTypeAndSizeValidation(t *testing.T) {
	_, db, ok := filesDBConfig()
	if !ok {
		t.Skip("PostgreSQL not available")
	}
	defer db.Close()

	ctx := context.Background()
	if err := ensureFilesTables(ctx, db); err != nil {
		t.Fatalf("ensure tables: %v", err)
	}
	defer cleanupFilesTables(ctx, db)

	store := files.NewFileStore(db.Pool, files.DefaultConfig())

	tenantID := "66666666-0000-0000-0000-000000000006"

	_, err := store.CreateObject(ctx, tenantID,
		"uploads/bad-type.exe", "virus.exe", "application/x-msdownload",
		100, "hash-bad-001", nil,
	)
	if err != files.ErrContentTypeDenied {
		t.Errorf("expected ErrContentTypeDenied for disallowed type, got: %v", err)
	}

	_, err = store.CreateObject(ctx, tenantID,
		"uploads/too-big.pdf", "too-big.pdf", "application/pdf",
		200*1024*1024, "hash-big-001", nil,
	)
	if err != files.ErrSizeLimitExceeded {
		t.Errorf("expected ErrSizeLimitExceeded for oversized object, got: %v", err)
	}

	obj, err := store.CreateObject(ctx, tenantID,
		"uploads/valid.png", "valid.png", "image/png",
		1024, "hash-valid-001", nil,
	)
	if err != nil {
		t.Fatalf("create valid object: %v", err)
	}
	if obj.ID == "" {
		t.Error("expected object ID")
	}
}

func TestFilesListObjects(t *testing.T) {
	_, db, ok := filesDBConfig()
	if !ok {
		t.Skip("PostgreSQL not available")
	}
	defer db.Close()

	ctx := context.Background()
	if err := ensureFilesTables(ctx, db); err != nil {
		t.Fatalf("ensure tables: %v", err)
	}
	defer cleanupFilesTables(ctx, db)

	store := files.NewFileStore(db.Pool, files.DefaultConfig())

	tenantA := "77777777-0000-0000-0000-000000000007"
	tenantB := "88888888-0000-0000-0000-000000000008"

	for i := 0; i < 3; i++ {
		_, err := store.CreateObject(ctx, tenantA,
			fmt.Sprintf("uploads/a-file-%d.pdf", i), "file.pdf", "application/pdf",
			int64(100*i), fmt.Sprintf("hash-a-%d", i), nil,
		)
		if err != nil {
			t.Fatalf("create object for tenant A: %v", err)
		}
	}

	_, err := store.CreateObject(ctx, tenantB,
		"uploads/b-file.pdf", "file.pdf", "application/pdf",
		500, "hash-b-001", nil,
	)
	if err != nil {
		t.Fatalf("create object for tenant B: %v", err)
	}

	objs, err := store.ListObjects(ctx, tenantA, 10, 0)
	if err != nil {
		t.Fatalf("list objects for tenant A: %v", err)
	}
	if len(objs) != 3 {
		t.Errorf("expected 3 objects for tenant A, got %d", len(objs))
	}

	objs, err = store.ListObjects(ctx, tenantB, 10, 0)
	if err != nil {
		t.Fatalf("list objects for tenant B: %v", err)
	}
	if len(objs) != 1 {
		t.Errorf("expected 1 object for tenant B, got %d", len(objs))
	}
}

func TestFilesMetadataStorage(t *testing.T) {
	_, db, ok := filesDBConfig()
	if !ok {
		t.Skip("PostgreSQL not available")
	}
	defer db.Close()

	ctx := context.Background()
	if err := ensureFilesTables(ctx, db); err != nil {
		t.Fatalf("ensure tables: %v", err)
	}
	defer cleanupFilesTables(ctx, db)

	store := files.NewFileStore(db.Pool, files.DefaultConfig())

	tenantID := "99999999-0000-0000-0000-000000000009"

	meta := json.RawMessage(`{"description":"test file","tags":["tag1","tag2"],"version":1}`)

	obj, err := store.CreateObject(ctx, tenantID,
		"uploads/meta-test.pdf", "meta-test.pdf", "application/pdf",
		1024, "hash-meta-001", meta,
	)
	if err != nil {
		t.Fatalf("create object with metadata: %v", err)
	}

	if len(obj.Metadata) == 0 {
		t.Error("expected metadata to be stored")
	}
	if string(obj.Metadata) != string(meta) {
		t.Errorf("metadata mismatch: got %s, want %s", obj.Metadata, meta)
	}

	fetched, err := store.GetObject(ctx, tenantID, obj.ID)
	if err != nil {
		t.Fatalf("get object: %v", err)
	}
	if string(fetched.Metadata) != string(meta) {
		t.Errorf("retrieved metadata mismatch")
	}
	if fetched.IsOriginal != true {
		t.Error("new object should be original")
	}
}

func TestFilesCleanupExpiredGrants(t *testing.T) {
	_, db, ok := filesDBConfig()
	if !ok {
		t.Skip("PostgreSQL not available")
	}
	defer db.Close()

	ctx := context.Background()
	if err := ensureFilesTables(ctx, db); err != nil {
		t.Fatalf("ensure tables: %v", err)
	}
	defer cleanupFilesTables(ctx, db)

	tenantID := "aaaaaaaa-1111-0000-0000-000000000010"

	shortCfg := files.DefaultConfig()
	shortCfg.UploadGrantTTL = 1 * time.Second
	shortCfg.DownloadGrantTTL = 5 * time.Minute

	store := files.NewFileStore(db.Pool, shortCfg)

	if _, err := store.CreateUploadGrant(ctx, tenantID, nil, nil); err != nil {
		t.Fatalf("create grant 1: %v", err)
	}

	grant2, err := store.CreateUploadGrant(ctx, tenantID, nil, nil)
	if err != nil {
		t.Fatalf("create grant 2: %v", err)
	}

	_, err = store.ValidateUploadGrant(ctx, grant2.GrantToken, 100, "text/plain")
	if err != nil {
		t.Fatalf("consume grant 2: %v", err)
	}

	time.Sleep(1500 * time.Millisecond)

	count, err := store.CleanupExpiredGrants(ctx)
	if err != nil {
		t.Fatalf("cleanup expired grants: %v", err)
	}

	if count < 1 {
		t.Errorf("expected at least 1 expired grant cleaned up, got %d", count)
	}

	var totalGrants int
	err = db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM file_grants`).Scan(&totalGrants)
	if err != nil {
		t.Fatalf("count grants: %v", err)
	}

	if totalGrants != 1 {
		t.Errorf("expected 1 grant remaining (consumed grant 2), got %d", totalGrants)
	}
}

func TestFilesImmutableOriginalReference(t *testing.T) {
	_, db, ok := filesDBConfig()
	if !ok {
		t.Skip("PostgreSQL not available")
	}
	defer db.Close()

	ctx := context.Background()
	if err := ensureFilesTables(ctx, db); err != nil {
		t.Fatalf("ensure tables: %v", err)
	}
	defer cleanupFilesTables(ctx, db)

	store := files.NewFileStore(db.Pool, files.DefaultConfig())

	tenantID := "bbbbbbbb-1111-0000-0000-000000000011"

	meta := json.RawMessage(`{"original_source":"camera","camera_model":"Canon EOS"}`)

	obj, err := store.CreateObject(ctx, tenantID,
		"uploads/original-photo.jpg", "original-photo.jpg", "image/jpeg",
		102400, "hash-original-001", meta,
	)
	if err != nil {
		t.Fatalf("create object: %v", err)
	}

	if !obj.IsOriginal {
		t.Error("new object should be marked as original")
	}

	err = store.UpdateScanStatus(ctx, tenantID, obj.ID, files.ScanStatusScanning)
	if err != nil {
		t.Fatalf("update to scanning: %v", err)
	}
	err = store.UpdateScanStatus(ctx, tenantID, obj.ID, files.ScanStatusClean)
	if err != nil {
		t.Fatalf("update to clean: %v", err)
	}

	fetched, err := store.GetObject(ctx, tenantID, obj.ID)
	if err != nil {
		t.Fatalf("get object: %v", err)
	}

	if !fetched.IsOriginal {
		t.Error("is_original should remain true after scan status changes")
	}
	if fetched.SHA256Hash != "hash-original-001" {
		t.Errorf("hash should remain unchanged, got %s", fetched.SHA256Hash)
	}
}

func TestFilesGrantReuseFails(t *testing.T) {
	_, db, ok := filesDBConfig()
	if !ok {
		t.Skip("PostgreSQL not available")
	}
	defer db.Close()

	ctx := context.Background()
	if err := ensureFilesTables(ctx, db); err != nil {
		t.Fatalf("ensure tables: %v", err)
	}
	defer cleanupFilesTables(ctx, db)

	store := files.NewFileStore(db.Pool, files.DefaultConfig())

	tenantID := "cccccccc-1111-0000-0000-000000000012"

	obj, err := store.CreateObject(ctx, tenantID,
		"uploads/reuse-test.pdf", "reuse-test.pdf", "application/pdf",
		512, "hash-reuse-001", nil,
	)
	if err != nil {
		t.Fatalf("create object: %v", err)
	}

	err = store.UpdateScanStatus(ctx, tenantID, obj.ID, files.ScanStatusScanning)
	if err != nil {
		t.Fatalf("update to scanning: %v", err)
	}
	err = store.UpdateScanStatus(ctx, tenantID, obj.ID, files.ScanStatusClean)
	if err != nil {
		t.Fatalf("update to clean: %v", err)
	}

	grant, err := store.CreateDownloadGrant(ctx, tenantID, obj.ID)
	if err != nil {
		t.Fatalf("create download grant: %v", err)
	}

	_, _, err = store.ValidateDownloadGrant(ctx, tenantID, grant.GrantToken)
	if err != nil {
		t.Fatalf("first use of grant: %v", err)
	}

	_, _, err = store.ValidateDownloadGrant(ctx, tenantID, grant.GrantToken)
	if err != files.ErrGrantUsed {
		t.Errorf("expected ErrGrantUsed on second use, got: %v", err)
	}
}

func isErrContains(err error, substr string) bool {
	if err == nil {
		return false
	}
	return len(err.Error()) >= len(substr) && containsSub(err.Error(), substr)
}

func containsSub(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
