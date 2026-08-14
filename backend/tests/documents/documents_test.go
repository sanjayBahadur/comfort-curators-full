package documents_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"comfort-curators-backend/internal/documents"
	"comfort-curators-backend/internal/platform/audit"

	"github.com/jackc/pgx/v5/pgxpool"
)

func postgresAvailable() bool {
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

func dbConnString() string {
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

func setupPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if !postgresAvailable() {
		t.Skip("PostgreSQL not available for documents integration test")
	}
	pool, err := pgxpool.New(context.Background(), dbConnString())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := documents.EnsureSchema(context.Background(), pool); err != nil {
		t.Fatalf("ensure documents schema: %v", err)
	}
	if err := audit.EnsureSchema(context.Background(), pool); err != nil {
		t.Fatalf("ensure audit schema: %v", err)
	}

	truncateTables(t, pool)
	return pool
}

func truncateTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	for _, table := range []string{
		"submission_receipts",
		"submission_packets",
		"document_reviews",
		"document_extractions",
		"document_versions",
		"documents",
	} {
		if _, err := pool.Exec(context.Background(), "DELETE FROM "+table); err != nil {
			t.Fatalf("truncate %s: %v", table, err)
		}
	}
}

func TestCreateDocument(t *testing.T) {
	pool := setupPool(t)
	svc := documents.NewService(pool)

	doc, err := svc.CreateDocument(context.Background(), "tenant-1", documents.CreateDocumentParams{
		Title:        "Test Agreement",
		DocumentType: documents.DocTypeAgreement,
		PropertyID:   "prop-1",
	}, "actor-1")
	if err != nil {
		t.Fatalf("create document: %v", err)
	}
	if doc.ID == "" {
		t.Fatal("document id is empty")
	}
	if doc.Status != documents.DocStatusDraft {
		t.Fatalf("expected status draft, got %s", doc.Status)
	}
	if doc.CurrentVersion != 1 {
		t.Fatalf("expected current_version 1, got %d", doc.CurrentVersion)
	}

	got, err := svc.GetDocument(context.Background(), "tenant-1", doc.ID)
	if err != nil {
		t.Fatalf("get document: %v", err)
	}
	if got.Title != "Test Agreement" {
		t.Fatalf("expected title 'Test Agreement', got %s", got.Title)
	}
}

func TestCreateDocumentInvalidType(t *testing.T) {
	pool := setupPool(t)
	svc := documents.NewService(pool)

	_, err := svc.CreateDocument(context.Background(), "tenant-1", documents.CreateDocumentParams{
		Title:        "Bad",
		DocumentType: "nonexistent",
		PropertyID:   "prop-1",
	}, "actor-1")
	if !errors.Is(err, documents.ErrInvalidDocument) {
		t.Fatalf("expected ErrInvalidDocument, got %v", err)
	}
}

func TestCreateDocumentWithoutTitle(t *testing.T) {
	pool := setupPool(t)
	svc := documents.NewService(pool)

	_, err := svc.CreateDocument(context.Background(), "tenant-1", documents.CreateDocumentParams{
		DocumentType: documents.DocTypeAgreement,
		PropertyID:   "prop-1",
	}, "actor-1")
	if !errors.Is(err, documents.ErrInvalidDocument) {
		t.Fatalf("expected ErrInvalidDocument, got %v", err)
	}
}

func TestCreateVersion(t *testing.T) {
	pool := setupPool(t)
	svc := documents.NewService(pool)

	doc, err := svc.CreateDocument(context.Background(), "tenant-1", documents.CreateDocumentParams{
		Title:        "Insurance Policy",
		DocumentType: documents.DocTypeInsurancePolicy,
		PropertyID:   "prop-1",
	}, "actor-1")
	if err != nil {
		t.Fatalf("create document: %v", err)
	}

	hash1 := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	ver, updatedDoc, err := svc.CreateVersion(context.Background(), "tenant-1", doc.ID, documents.CreateVersionParams{
		ContentHash: hash1,
		ObjectKey:   "tenants/tenant-1/docs/doc/v1.pdf",
		Filename:    "policy.pdf",
		ContentType: "application/pdf",
		SizeBytes:   102400,
	}, "actor-1")
	if err != nil {
		t.Fatalf("create version: %v", err)
	}
	if ver.ContentHash != hash1 {
		t.Fatalf("expected content_hash %s, got %s", hash1, ver.ContentHash)
	}
	if ver.VersionNumber != 1 {
		t.Fatalf("expected version_number 1, got %d", ver.VersionNumber)
	}
	if updatedDoc.CurrentVersion != 2 {
		t.Fatalf("expected current_version 2, got %d", updatedDoc.CurrentVersion)
	}

	versions, err := svc.ListVersions(context.Background(), "tenant-1", doc.ID)
	if err != nil {
		t.Fatalf("list versions: %v", err)
	}
	if len(versions) != 1 {
		t.Fatalf("expected 1 version, got %d", len(versions))
	}
}

func TestVersionImmutability(t *testing.T) {
	pool := setupPool(t)
	svc := documents.NewService(pool)

	doc, err := svc.CreateDocument(context.Background(), "tenant-1", documents.CreateDocumentParams{
		Title:        "Immutability Test",
		DocumentType: documents.DocTypeComplianceCert,
		PropertyID:   "prop-1",
	}, "actor-1")
	if err != nil {
		t.Fatalf("create document: %v", err)
	}

	hash1 := "hash_v1_abcdef123456"
	_, _, err = svc.CreateVersion(context.Background(), "tenant-1", doc.ID, documents.CreateVersionParams{
		ContentHash: hash1,
		ObjectKey:   "tenants/tenant-1/docs/doc/v1.pdf",
		Filename:    "v1.pdf",
		ContentType: "application/pdf",
		SizeBytes:   1024,
	}, "actor-1")
	if err != nil {
		t.Fatalf("create version 1: %v", err)
	}

	hash2 := "hash_v2_abcdef789012"
	_, _, err = svc.CreateVersion(context.Background(), "tenant-1", doc.ID, documents.CreateVersionParams{
		ContentHash: hash2,
		ObjectKey:   "tenants/tenant-1/docs/doc/v2.pdf",
		Filename:    "v2.pdf",
		ContentType: "application/pdf",
		SizeBytes:   2048,
	}, "actor-1")
	if err != nil {
		t.Fatalf("create version 2: %v", err)
	}

	versions, err := svc.ListVersions(context.Background(), "tenant-1", doc.ID)
	if err != nil {
		t.Fatalf("list versions: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(versions))
	}

	v1 := versions[1]
	v2 := versions[0]
	if v1.ContentHash != hash1 {
		t.Fatalf("v1 content_hash must remain %s, got %s", hash1, v1.ContentHash)
	}
	if v2.ContentHash != hash2 {
		t.Fatalf("v2 content_hash must remain %s, got %s", hash2, v2.ContentHash)
	}

	// A correction must create a superseding version, not modify the original
	hash3 := "hash_v3_correction"
	_, _, err = svc.CreateVersion(context.Background(), "tenant-1", doc.ID, documents.CreateVersionParams{
		ContentHash: hash3,
		ObjectKey:   "tenants/tenant-1/docs/doc/v3.pdf",
		Filename:    "v3.pdf",
		ContentType: "application/pdf",
		SizeBytes:   4096,
	}, "actor-1")
	if err != nil {
		t.Fatalf("create version 3: %v", err)
	}

	versionsAfter, err := svc.ListVersions(context.Background(), "tenant-1", doc.ID)
	if err != nil {
		t.Fatalf("list versions after correction: %v", err)
	}
	if len(versionsAfter) != 3 {
		t.Fatalf("expected 3 versions after correction, got %d", len(versionsAfter))
	}

	for _, v := range versionsAfter {
		if v.VersionNumber == 1 && v.ContentHash != hash1 {
			t.Fatalf("version 1 must retain original bytes %s, got %s", hash1, v.ContentHash)
		}
		if v.VersionNumber == 2 && v.ContentHash != hash2 {
			t.Fatalf("version 2 must retain original bytes %s, got %s", hash2, v.ContentHash)
		}
	}
}

func TestDuplicateVersionHash(t *testing.T) {
	pool := setupPool(t)
	svc := documents.NewService(pool)

	doc, err := svc.CreateDocument(context.Background(), "tenant-1", documents.CreateDocumentParams{
		Title:        "Duplicate Test",
		DocumentType: documents.DocTypeOther,
		PropertyID:   "prop-1",
	}, "actor-1")
	if err != nil {
		t.Fatalf("create document: %v", err)
	}

	hash := "same_hash_content"
	_, _, err = svc.CreateVersion(context.Background(), "tenant-1", doc.ID, documents.CreateVersionParams{
		ContentHash: hash,
		ObjectKey:   "tenants/tenant-1/docs/doc/v1.pdf",
		Filename:    "v1.pdf",
		ContentType: "application/pdf",
		SizeBytes:   1024,
	}, "actor-1")
	if err != nil {
		t.Fatalf("create version 1: %v", err)
	}

	_, _, err = svc.CreateVersion(context.Background(), "tenant-1", doc.ID, documents.CreateVersionParams{
		ContentHash: hash,
		ObjectKey:   "tenants/tenant-1/docs/doc/v1b.pdf",
		Filename:    "v1b.pdf",
		ContentType: "application/pdf",
		SizeBytes:   2048,
	}, "actor-1")
	if !errors.Is(err, documents.ErrDuplicateVersion) {
		t.Fatalf("expected ErrDuplicateVersion, got %v", err)
	}
}

func TestCreateExtraction(t *testing.T) {
	pool := setupPool(t)
	svc := documents.NewService(pool)

	doc, err := svc.CreateDocument(context.Background(), "tenant-1", documents.CreateDocumentParams{
		Title:        "ID Document",
		DocumentType: documents.DocTypeGovernmentID,
		PropertyID:   "prop-1",
	}, "actor-1")
	if err != nil {
		t.Fatalf("create document: %v", err)
	}

	ver, _, err := svc.CreateVersion(context.Background(), "tenant-1", doc.ID, documents.CreateVersionParams{
		ContentHash: "extraction_hash",
		ObjectKey:   "tenants/tenant-1/docs/doc/id.pdf",
		Filename:    "id.pdf",
		ContentType: "application/pdf",
		SizeBytes:   1024,
	}, "actor-1")
	if err != nil {
		t.Fatalf("create version: %v", err)
	}

	ext, err := svc.CreateExtraction(context.Background(), "tenant-1", ver.ID, documents.CreateExtractionParams{
		FieldName:       "full_name",
		FieldValue:      "John Doe",
		FieldCategory:   documents.FieldCategoryIdentity,
		SourceLocation:  "page 1, paragraph 1, bbox [100, 150, 300, 30]",
		Confidence:      documents.ConfidenceHigh,
		ConfidenceScore: 0.97,
		ExtractedBy:     "ocr_v2",
	}, "actor-1")
	if err != nil {
		t.Fatalf("create extraction: %v", err)
	}

	if ext.SourceLocation != "page 1, paragraph 1, bbox [100, 150, 300, 30]" {
		t.Fatalf("expected source_location, got %s", ext.SourceLocation)
	}
	if ext.Confidence != documents.ConfidenceHigh {
		t.Fatalf("expected confidence high, got %s", ext.Confidence)
	}
	if ext.FieldName != "full_name" {
		t.Fatalf("expected field_name 'full_name', got %s", ext.FieldName)
	}
	if ext.FieldCategory != documents.FieldCategoryIdentity {
		t.Fatalf("expected field_category identity, got %s", ext.FieldCategory)
	}

	extractions, err := svc.ListExtractions(context.Background(), "tenant-1", ver.ID)
	if err != nil {
		t.Fatalf("list extractions: %v", err)
	}
	if len(extractions) != 1 {
		t.Fatalf("expected 1 extraction, got %d", len(extractions))
	}
}

func TestLowConfidenceRequiresReview(t *testing.T) {
	pool := setupPool(t)
	svc := documents.NewService(pool)

	doc, err := svc.CreateDocument(context.Background(), "tenant-1", documents.CreateDocumentParams{
		Title:        "Legal Document",
		DocumentType: documents.DocTypeTaxDocument,
		PropertyID:   "prop-1",
	}, "actor-1")
	if err != nil {
		t.Fatalf("create document: %v", err)
	}

	ver, _, err := svc.CreateVersion(context.Background(), "tenant-1", doc.ID, documents.CreateVersionParams{
		ContentHash: "legal_doc_hash",
		ObjectKey:   "tenants/tenant-1/docs/doc/legal.pdf",
		Filename:    "legal.pdf",
		ContentType: "application/pdf",
		SizeBytes:   1024,
	}, "actor-1")
	if err != nil {
		t.Fatalf("create version: %v", err)
	}

	// Create a low-confidence legal field extraction
	_, err = svc.CreateExtraction(context.Background(), "tenant-1", ver.ID, documents.CreateExtractionParams{
		FieldName:       "tax_id",
		FieldValue:      "ABCD1234",
		FieldCategory:   documents.FieldCategoryLegal,
		SourceLocation:  "page 2, paragraph 4",
		Confidence:      documents.ConfidenceLow,
		ConfidenceScore: 0.25,
		ExtractedBy:     "ocr_v2",
	}, "actor-1")
	if err != nil {
		t.Fatalf("create low-confidence extraction: %v", err)
	}

	// Create a submission packet with the low-confidence document
	pkt, err := svc.CreateSubmissionPacket(context.Background(), "tenant-1", documents.CreateSubmissionPacketParams{
		PropertyID:  "prop-1",
		DocumentIDs: []string{doc.ID},
	}, "actor-1")
	if err != nil {
		t.Fatalf("create packet: %v", err)
	}

	// Confirm without human review must fail
	_, _, err = svc.ConfirmSubmission(context.Background(), "tenant-1", pkt.ID, "actor-1")
	if !errors.Is(err, documents.ErrHumanReviewRequired) {
		t.Fatalf("expected ErrHumanReviewRequired for low-confidence legal field, got %v", err)
	}

	// Now submit a human review approving the document
	_, err = svc.ReviewDocument(context.Background(), "tenant-1", doc.ID, documents.CreateReviewParams{
		Status:   documents.ReviewStatusApproved,
		Decision: "approved",
		Comments: "Verified against official records",
	}, "human-reviewer-1")
	if err != nil {
		t.Fatalf("create review: %v", err)
	}

	// Confirmation must now succeed after review
	receipt, pkt2, err := svc.ConfirmSubmission(context.Background(), "tenant-1", pkt.ID, "actor-1")
	if err != nil {
		t.Fatalf("confirm after review must succeed: %v", err)
	}
	if receipt.ReceiptHash == "" {
		t.Fatal("receipt hash is empty")
	}
	if pkt2.Status != documents.PacketStatusSubmitted {
		t.Fatalf("expected status submitted, got %s", pkt2.Status)
	}
}

func TestSubmissionReceiptPreservesExactVersion(t *testing.T) {
	pool := setupPool(t)
	svc := documents.NewService(pool)

	doc, err := svc.CreateDocument(context.Background(), "tenant-1", documents.CreateDocumentParams{
		Title:        "Receipt Version Test",
		DocumentType: documents.DocTypeAgreement,
		PropertyID:   "prop-1",
	}, "actor-1")
	if err != nil {
		t.Fatalf("create document: %v", err)
	}

	hash1 := "receipt_version_hash_v1"
	ver, _, err := svc.CreateVersion(context.Background(), "tenant-1", doc.ID, documents.CreateVersionParams{
		ContentHash: hash1,
		ObjectKey:   "tenants/tenant-1/docs/doc/v1.pdf",
		Filename:    "v1.pdf",
		ContentType: "application/pdf",
		SizeBytes:   1024,
	}, "actor-1")
	if err != nil {
		t.Fatalf("create version: %v", err)
	}

	pkt, err := svc.CreateSubmissionPacket(context.Background(), "tenant-1", documents.CreateSubmissionPacketParams{
		PropertyID:  "prop-1",
		DocumentIDs: []string{doc.ID},
	}, "actor-1")
	if err != nil {
		t.Fatalf("create packet: %v", err)
	}

	receipt, _, err := svc.ConfirmSubmission(context.Background(), "tenant-1", pkt.ID, "actor-1")
	if err != nil {
		t.Fatalf("confirm submission: %v", err)
	}

	if receipt.ReceiptHash == "" {
		t.Fatal("receipt must have a hash")
	}
	if receipt.ConfirmedBy != "actor-1" {
		t.Fatalf("expected confirmed_by actor-1, got %s", receipt.ConfirmedBy)
	}
	if len(receipt.DocumentVersionRefs) != 1 {
		t.Fatalf("expected 1 version ref, got %d", len(receipt.DocumentVersionRefs))
	}

	ref := receipt.DocumentVersionRefs[0]
	if ref.DocumentID != doc.ID {
		t.Fatalf("expected document_id %s, got %s", doc.ID, ref.DocumentID)
	}
	if ref.VersionNumber != 1 {
		t.Fatalf("expected version_number 1, got %d", ref.VersionNumber)
	}
	if ref.ContentHash != hash1 {
		t.Fatalf("expected content_hash %s, got %s", hash1, ref.ContentHash)
	}

	// The exact version must match the created one
	if ref.DocumentVersionID != ver.ID {
		t.Fatalf("expected version_id %s, got %s", ver.ID, ref.DocumentVersionID)
	}

	// Retrieve the receipt
	gotReceipt, err := svc.GetReceipt(context.Background(), "tenant-1", pkt.ID)
	if err != nil {
		t.Fatalf("get receipt: %v", err)
	}
	if gotReceipt == nil {
		t.Fatal("receipt must be retrievable")
	}
	if gotReceipt.ReceiptHash != receipt.ReceiptHash {
		t.Fatalf("persisted receipt hash must match")
	}
}

func TestDuplicateSubmissionFails(t *testing.T) {
	pool := setupPool(t)
	svc := documents.NewService(pool)

	doc, err := svc.CreateDocument(context.Background(), "tenant-1", documents.CreateDocumentParams{
		Title:        "Duplicate Submission Test",
		DocumentType: documents.DocTypeAgreement,
		PropertyID:   "prop-1",
	}, "actor-1")
	if err != nil {
		t.Fatalf("create document: %v", err)
	}

	_, _, err = svc.CreateVersion(context.Background(), "tenant-1", doc.ID, documents.CreateVersionParams{
		ContentHash: "dup_sub_hash",
		ObjectKey:   "tenants/tenant-1/docs/doc/v1.pdf",
		Filename:    "v1.pdf",
		ContentType: "application/pdf",
		SizeBytes:   1024,
	}, "actor-1")
	if err != nil {
		t.Fatalf("create version: %v", err)
	}

	pkt, err := svc.CreateSubmissionPacket(context.Background(), "tenant-1", documents.CreateSubmissionPacketParams{
		PropertyID:  "prop-1",
		DocumentIDs: []string{doc.ID},
	}, "actor-1")
	if err != nil {
		t.Fatalf("create packet: %v", err)
	}

	_, _, err = svc.ConfirmSubmission(context.Background(), "tenant-1", pkt.ID, "actor-1")
	if err != nil {
		t.Fatalf("first confirm must succeed: %v", err)
	}

	_, _, err = svc.ConfirmSubmission(context.Background(), "tenant-1", pkt.ID, "actor-2")
	if !errors.Is(err, documents.ErrPacketAlreadySubmitted) {
		t.Fatalf("expected ErrPacketAlreadySubmitted, got %v", err)
	}
}

func TestCrossTenantIsolation(t *testing.T) {
	pool := setupPool(t)
	svc := documents.NewService(pool)

	doc, err := svc.CreateDocument(context.Background(), "tenant-a", documents.CreateDocumentParams{
		Title:        "Tenant A Doc",
		DocumentType: documents.DocTypeAgreement,
		PropertyID:   "prop-a",
	}, "actor-a")
	if err != nil {
		t.Fatalf("create document for tenant A: %v", err)
	}

	_, err = svc.GetDocument(context.Background(), "tenant-b", doc.ID)
	if !errors.Is(err, documents.ErrDocumentNotFound) {
		t.Fatalf("expected ErrDocumentNotFound for cross-tenant read, got %v", err)
	}

	_, _, err = svc.CreateVersion(context.Background(), "tenant-b", doc.ID, documents.CreateVersionParams{
		ContentHash: "cross_hash",
		ObjectKey:   "tenants/tenant-b/bad",
		Filename:    "bad.pdf",
		ContentType: "application/pdf",
		SizeBytes:   1024,
	}, "actor-b")
	if !errors.Is(err, documents.ErrDocumentNotFound) {
		t.Fatalf("expected ErrDocumentNotFound for cross-tenant version, got %v", err)
	}
}

func TestDocumentExpiryDetection(t *testing.T) {
	pool := setupPool(t)
	svc := documents.NewService(pool)

	doc, err := svc.CreateDocument(context.Background(), "tenant-1", documents.CreateDocumentParams{
		Title:        "Expiring Doc",
		DocumentType: documents.DocTypeComplianceCert,
		PropertyID:   "prop-1",
	}, "actor-1")
	if err != nil {
		t.Fatalf("create document: %v", err)
	}

	expired, err := svc.DetectExpiry(context.Background(), "tenant-1")
	if err != nil {
		t.Fatalf("detect expiry: %v", err)
	}
	_ = doc
	_ = expired

	nearing, err := svc.FindNearingExpiry(context.Background(), "tenant-1", 30)
	if err != nil {
		t.Fatalf("find nearing expiry: %v", err)
	}
	_ = nearing
}

func TestReviewDocument(t *testing.T) {
	pool := setupPool(t)
	svc := documents.NewService(pool)

	doc, err := svc.CreateDocument(context.Background(), "tenant-1", documents.CreateDocumentParams{
		Title:        "Review Test",
		DocumentType: documents.DocTypeAgreement,
		PropertyID:   "prop-1",
	}, "actor-1")
	if err != nil {
		t.Fatalf("create document: %v", err)
	}

	review, err := svc.ReviewDocument(context.Background(), "tenant-1", doc.ID, documents.CreateReviewParams{
		Status:   documents.ReviewStatusApproved,
		Decision: "approved",
		Comments: "Looks good",
	}, "reviewer-1")
	if err != nil {
		t.Fatalf("create review: %v", err)
	}
	if review.Status != documents.ReviewStatusApproved {
		t.Fatalf("expected status approved, got %s", review.Status)
	}
	if review.ReviewerID != "reviewer-1" {
		t.Fatalf("expected reviewer_id reviewer-1, got %s", review.ReviewerID)
	}

	reviews, err := svc.ListReviews(context.Background(), "tenant-1", doc.ID)
	if err != nil {
		t.Fatalf("list reviews: %v", err)
	}
	if len(reviews) != 1 {
		t.Fatalf("expected 1 review, got %d", len(reviews))
	}
	if reviews[0].Decision != "approved" {
		t.Fatalf("expected decision approved, got %s", reviews[0].Decision)
	}
}
