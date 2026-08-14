package documents

import (
	"context"
	"errors"
	"fmt"
	"time"

	"comfort-curators-backend/internal/platform/audit"
	"comfort-curators-backend/internal/platform/database"
	"comfort-curators-backend/internal/platform/logging"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	pool       *pgxpool.Pool
	store      *DocumentStore
	auditStore *audit.AuditStore
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{
		pool:       pool,
		store:      NewDocumentStore(pool),
		auditStore: audit.NewAuditStore(pool),
	}
}

func (s *Service) WithAudit(a *audit.AuditStore) *Service {
	s.auditStore = a
	return s
}

func (s *Service) appendAudit(ctx context.Context, event audit.AuditEvent) {
	if s.auditStore == nil {
		return
	}
	if event.ID == "" {
		event.ID = newID("aud")
	}
	if err := s.auditStore.Append(ctx, event); err != nil {
		logging.Error(ctx, "failed to append audit event", "error", err)
	}
}

// CreateDocument creates a new document in draft status.
func (s *Service) CreateDocument(ctx context.Context, tenantID string, params CreateDocumentParams, actorID string) (*Document, error) {
	if params.DocumentType == "" || !ValidDocType(params.DocumentType) {
		return nil, fmt.Errorf("%w: invalid document type", ErrInvalidDocument)
	}
	if params.Title == "" {
		return nil, fmt.Errorf("%w: title is required", ErrInvalidDocument)
	}

	doc := &Document{
		TenantID:       tenantID,
		PropertyID:     params.PropertyID,
		Title:          params.Title,
		DocumentType:   params.DocumentType,
		Status:         DocStatusDraft,
		CurrentVersion: 1,
	}

	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertDocument(ctx, tx, doc); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "document.created",
			ResourceType: "document",
			ResourceID:   doc.ID,
			NewState:     marshalJSON(doc),
		})
	}); err != nil {
		return nil, err
	}

	return doc, nil
}

func (s *Service) GetDocument(ctx context.Context, tenantID, documentID string) (*Document, error) {
	return s.store.GetDocument(ctx, tenantID, documentID)
}

func (s *Service) ListDocuments(ctx context.Context, tenantID, propertyID string) ([]Document, error) {
	return s.store.ListDocuments(ctx, tenantID, propertyID)
}

// CreateVersion creates a new immutable version of a document.
// This is the ONLY way to add content to a document - versions cannot be modified after creation.
func (s *Service) CreateVersion(ctx context.Context, tenantID, documentID string, params CreateVersionParams, actorID string) (*DocumentVersion, *Document, error) {
	doc, err := s.store.GetDocument(ctx, tenantID, documentID)
	if err != nil {
		return nil, nil, err
	}

	if doc.Status == DocStatusExpired || doc.Status == DocStatusRevoked || doc.Status == DocStatusSuperseded {
		return nil, nil, fmt.Errorf("%w: document is %s", ErrDocumentExpired, doc.Status)
	}

	if params.ContentHash == "" {
		return nil, nil, fmt.Errorf("%w: content_hash is required", ErrInvalidVersion)
	}
	if params.ObjectKey == "" {
		return nil, nil, fmt.Errorf("%w: object_key is required", ErrInvalidVersion)
	}

	var ver *DocumentVersion
	var updatedDoc *Document
	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		ver = &DocumentVersion{
			DocumentID:    documentID,
			TenantID:      tenantID,
			VersionNumber: doc.CurrentVersion,
			ContentHash:   params.ContentHash,
			ObjectKey:     params.ObjectKey,
			Filename:      params.Filename,
			ContentType:   params.ContentType,
			SizeBytes:     params.SizeBytes,
			UploadedBy:    actorID,
			Metadata:      params.Metadata,
		}

		if err := s.store.InsertVersion(ctx, tx, ver); err != nil {
			return err
		}

		updatedDoc, err = s.store.BumpDocumentVersion(ctx, tx, tenantID, documentID)
		if err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "document.version.created",
			ResourceType: "document_version",
			ResourceID:   ver.ID,
			NewState:     marshalJSON(ver),
		})
	})
	if err != nil {
		return nil, nil, err
	}

	return ver, updatedDoc, nil
}

func (s *Service) GetVersion(ctx context.Context, tenantID, versionID string) (*DocumentVersion, error) {
	return s.store.GetVersion(ctx, tenantID, versionID)
}

func (s *Service) ListVersions(ctx context.Context, tenantID, documentID string) ([]DocumentVersion, error) {
	return s.store.ListVersions(ctx, tenantID, documentID)
}

// CreateExtraction records a field extraction with source location and confidence.
func (s *Service) CreateExtraction(ctx context.Context, tenantID, versionID string, params CreateExtractionParams, actorID string) (*Extraction, error) {
	ver, err := s.store.GetVersion(ctx, tenantID, versionID)
	if err != nil {
		return nil, err
	}

	if params.FieldName == "" {
		return nil, fmt.Errorf("%w: field_name is required", ErrInvalidExtraction)
	}

	ext := &Extraction{
		DocumentVersionID: versionID,
		TenantID:          tenantID,
		FieldName:         params.FieldName,
		FieldValue:        params.FieldValue,
		FieldCategory:     params.FieldCategory,
		SourceLocation:    params.SourceLocation,
		Confidence:        params.Confidence,
		ConfidenceScore:   params.ConfidenceScore,
		ExtractedBy:       params.ExtractedBy,
		ExtractedAt:       time.Now().UTC(),
	}

	if ext.Confidence == "" {
		ext.Confidence = ConfidenceHigh
	}
	if ext.FieldCategory == "" {
		ext.FieldCategory = FieldCategoryGeneral
	}

	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertExtraction(ctx, tx, ext); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "document.extraction.created",
			ResourceType: "document_extraction",
			ResourceID:   ext.ID,
			NewState:     marshalJSON(ext),
		})
	}); err != nil {
		return nil, err
	}

	_ = ver
	return ext, nil
}

func (s *Service) ListExtractions(ctx context.Context, tenantID, versionID string) ([]Extraction, error) {
	return s.store.ListExtractions(ctx, tenantID, versionID)
}

// ReviewDocument creates a human review for a document version.
// Low confidence extraction or legal category requires human review before submission.
func (s *Service) ReviewDocument(ctx context.Context, tenantID, documentID string, params CreateReviewParams, reviewerID string) (*Review, error) {
	doc, err := s.store.GetDocument(ctx, tenantID, documentID)
	if err != nil {
		return nil, err
	}

	versionID := ""
	if doc.CurrentVersion > 1 {
		ver, err := s.store.GetVersionByNumber(ctx, tenantID, documentID, doc.CurrentVersion)
		if err != nil {
			return nil, err
		}
		versionID = ver.ID
	}

	if reviewerID == "" {
		return nil, fmt.Errorf("%w: reviewer_id is required", ErrInvalidReview)
	}

	now := time.Now().UTC()
	review := &Review{
		DocumentID:        documentID,
		DocumentVersionID: versionID,
		TenantID:          tenantID,
		ReviewerID:        reviewerID,
		Status:            params.Status,
		Decision:          params.Decision,
		Comments:          params.Comments,
		ReviewedAt:        now,
	}

	if review.Status == "" {
		review.Status = ReviewStatusPending
	}

	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertReview(ctx, tx, review); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      reviewerID,
			Action:       "document.review.created",
			ResourceType: "document_review",
			ResourceID:   review.ID,
			NewState:     marshalJSON(review),
		})
	}); err != nil {
		return nil, err
	}

	return review, nil
}

func (s *Service) GetReview(ctx context.Context, tenantID, reviewID string) (*Review, error) {
	return s.store.GetReview(ctx, tenantID, reviewID)
}

func (s *Service) ListReviews(ctx context.Context, tenantID, documentID string) ([]Review, error) {
	return s.store.ListReviews(ctx, tenantID, documentID)
}

// CreateSubmissionPacket creates a collection of documents for submission.
func (s *Service) CreateSubmissionPacket(ctx context.Context, tenantID string, params CreateSubmissionPacketParams, actorID string) (*SubmissionPacket, error) {
	if len(params.DocumentIDs) == 0 {
		return nil, fmt.Errorf("%w: at least one document is required", ErrInvalidSubmissionPacket)
	}

	packet := &SubmissionPacket{
		TenantID:    tenantID,
		PropertyID:  params.PropertyID,
		Status:      PacketStatusDraft,
		DocumentIDs: params.DocumentIDs,
		CreatedBy:   actorID,
	}

	if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.store.InsertSubmissionPacket(ctx, tx, packet); err != nil {
			return err
		}
		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      actorID,
			Action:       "submission_packet.created",
			ResourceType: "submission_packet",
			ResourceID:   packet.ID,
			NewState:     marshalJSON(packet),
		})
	}); err != nil {
		return nil, err
	}

	return packet, nil
}

func (s *Service) GetSubmissionPacket(ctx context.Context, tenantID, packetID string) (*SubmissionPacket, error) {
	return s.store.GetSubmissionPacket(ctx, tenantID, packetID)
}

// ConfirmSubmission requires an authorized human to confirm.
// AI cannot confirm submissions (acceptance criteria).
// Low confidence / legal fields must have a human review.
// The exact submitted version and receipt are preserved.
func (s *Service) ConfirmSubmission(ctx context.Context, tenantID, packetID string, confirmedBy string) (*SubmissionReceipt, *SubmissionPacket, error) {
	packet, err := s.store.GetSubmissionPacket(ctx, tenantID, packetID)
	if err != nil {
		return nil, nil, err
	}

	if packet.Status == PacketStatusSubmitted || packet.Status == PacketStatusConfirmed {
		return nil, nil, ErrPacketAlreadySubmitted
	}

	if confirmedBy == "" {
		return nil, nil, fmt.Errorf("%w: human confirmation required", ErrHumanReviewRequired)
	}

	// Gather current version refs for all documents in the packet
	var versionRefs []DocumentVersionRef
	for _, docID := range packet.DocumentIDs {
		doc, err := s.store.GetDocument(ctx, tenantID, docID)
		if err != nil {
			return nil, nil, fmt.Errorf("packet document %s: %w", docID, err)
		}

		var versionID string
		var versionNumber int
		var contentHash string

		if doc.CurrentVersion > 0 {
			ver, err := s.store.GetVersionByNumber(ctx, tenantID, docID, doc.CurrentVersion)
			if err != nil {
				if errors.Is(err, ErrDocumentVersionNotFound) {
					// Document has no versions yet
					versionNumber = 0
				} else {
					return nil, nil, fmt.Errorf("packet document %s version: %w", docID, err)
				}
			} else {
				versionID = ver.ID
				versionNumber = ver.VersionNumber
				contentHash = ver.ContentHash
			}
		}

		versionRefs = append(versionRefs, DocumentVersionRef{
			DocumentID:        docID,
			DocumentVersionID: versionID,
			VersionNumber:     versionNumber,
			ContentHash:       contentHash,
		})

		// Check for low confidence or legal fields requiring human review
		if versionID != "" {
			needsReview, err := s.store.HasLowConfidenceOrLegalExtractions(ctx, tenantID, versionID)
			if err != nil {
				return nil, nil, fmt.Errorf("checking review requirements: %w", err)
			}
			if needsReview {
				// Check if there's a completed review
				reviews, err := s.store.ListReviews(ctx, tenantID, docID)
				if err != nil {
					return nil, nil, fmt.Errorf("checking reviews: %w", err)
				}
				hasApproved := false
				for _, r := range reviews {
					if r.Status == ReviewStatusApproved {
						hasApproved = true
						break
					}
				}
				if !hasApproved {
					return nil, nil, fmt.Errorf("%w: document %s has low-confidence or legal fields requiring human review", ErrHumanReviewRequired, docID)
				}
			}
		}
	}

	now := time.Now().UTC()
	receipt := &SubmissionReceipt{
		PacketID:            packetID,
		TenantID:            tenantID,
		ConfirmedBy:         confirmedBy,
		DocumentVersionRefs: versionRefs,
		ConfirmedAt:         now,
	}
	receipt.ReceiptHash = computeReceiptHash(packetID, versionRefs, confirmedBy, now)

	var updatedPacket *SubmissionPacket
	err = database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		updatedPacket, err = s.store.UpdatePacketStatus(ctx, tx, tenantID, packetID, PacketStatusSubmitted)
		if err != nil {
			return err
		}

		if err := s.store.InsertReceipt(ctx, tx, receipt); err != nil {
			return err
		}

		return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
			EventType:    audit.EventTypeMutation,
			TenantID:     tenantID,
			ActorID:      confirmedBy,
			Action:       "submission_packet.confirmed",
			ResourceType: "submission_packet",
			ResourceID:   packetID,
			NewState:     marshalJSON(receipt),
		})
	})
	if err != nil {
		return nil, nil, err
	}

	return receipt, updatedPacket, nil
}

func (s *Service) GetReceipt(ctx context.Context, tenantID, packetID string) (*SubmissionReceipt, error) {
	return s.store.GetReceipt(ctx, tenantID, packetID)
}

// DetectExpiry finds expired documents and marks them.
func (s *Service) DetectExpiry(ctx context.Context, tenantID string) ([]Document, error) {
	expired, err := s.store.FindExpiredDocuments(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	for i := range expired {
		docID := expired[i].ID
		docStatus := DocStatusExpired
		if err := database.WithTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
			if _, err := s.store.UpdateDocumentStatus(ctx, tx, tenantID, docID, docStatus); err != nil {
				return err
			}
			return s.auditStore.AppendTx(ctx, tx, audit.AuditEvent{
				EventType:    audit.EventTypeSystem,
				TenantID:     tenantID,
				Action:       "document.expired",
				ResourceType: "document",
				ResourceID:   docID,
				NewState:     marshalJSON(expired[i]),
			})
		}); err != nil {
			logging.Error(ctx, "failed to mark document as expired", "document_id", docID, "error", err)
		} else {
			expired[i].Status = DocStatusExpired
		}
	}

	return expired, nil
}

func (s *Service) FindNearingExpiry(ctx context.Context, tenantID string, withinDays int) ([]Document, error) {
	return s.store.FindDocumentsNearingExpiry(ctx, tenantID, withinDays)
}
