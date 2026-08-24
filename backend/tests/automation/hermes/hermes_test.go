package automation_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"comfort-curators-backend/internal/automation/hermes"
	"comfort-curators-backend/internal/platform/testdb"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestIntegrationHermesDurableDraftReviewDelivery(t *testing.T) {
	if !hermesPostgresReady() {
		t.Skip("PostgreSQL not available")
	}

	pool, err := hermesTestPool()
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	defer pool.Close()

	if err := hermes.EnsureSchema(context.Background(), pool); err != nil {
		t.Fatalf("ensure hermes schema: %v", err)
	}
	defer func() {
		for _, table := range []string{"hermes_deliveries", "hermes_reviews", "hermes_drafts"} {
			if _, err := pool.Exec(context.Background(), "TRUNCATE TABLE "+table+" CASCADE"); err != nil {
				t.Logf("cleanup %s: %v", table, err)
			}
		}
	}()

	store := hermes.NewPGStore(pool)
	svc := hermes.NewService(store)
	ctx := context.Background()

	params := hermes.DraftParams{
		RunID:        "run-hermes-pg",
		TenantID:     "tenant-pg",
		PropertyID:   "prop-pg",
		ActorID:      "actor-pg",
		Audience:     hermes.AudienceOwner,
		Purpose:      "owner exception notice",
		ReviewPolicy: hermes.ReviewPolicyHumanReview,
		Language:     "en",
		Channel:      "push",
		Subject:      "Follow-up on your issue",
		Body:         "We are resolving the reported water pressure problem.",
		Facts: []hermes.ApprovedFact{{
			Source:      "tickets",
			RecordID:    "ticket-pg-1",
			RecordKind:  "ticket",
			Audience:    hermes.AudienceOwner,
			EffectiveAt: time.Now().UTC().Add(-time.Hour),
		}},
	}

	t.Run("free-form draft is durable and requires review", func(t *testing.T) {
		draft, err := svc.Draft(ctx, params)
		if err != nil {
			t.Fatalf("draft: %v", err)
		}
		if draft.DraftID == "" {
			t.Fatal("draft ID must be persisted")
		}
		if draft.State != hermes.DraftStateUnderReview {
			t.Fatalf("expected under_review, got %s", draft.State)
		}

		_, err = svc.Deliver(ctx, hermes.DeliveryParams{
			TenantID:    params.TenantID,
			DraftID:     draft.DraftID,
			RecipientID: "owner-pg",
			ActorID:     "actor-pg",
		})
		if !errors.Is(err, hermes.ErrDraftRequiresReview) {
			t.Fatalf("unreviewed free-form draft must not deliver, got %v", err)
		}

		approved, err := svc.Review(ctx, params.TenantID, draft.DraftID, hermes.ReviewParams{
			ReviewerID: "reviewer-pg",
			Decision:   hermes.ReviewDecisionApproved,
		})
		if err != nil {
			t.Fatalf("review: %v", err)
		}
		if approved.State != hermes.DraftStateApproved {
			t.Fatalf("expected approved, got %s", approved.State)
		}

		first, err := svc.Deliver(ctx, hermes.DeliveryParams{
			TenantID:       params.TenantID,
			DraftID:        draft.DraftID,
			RecipientID:    "owner-pg",
			ActorID:        "actor-pg",
			IdempotencyKey: "pg-delivery-key-1",
		})
		if err != nil {
			t.Fatalf("deliver: %v", err)
		}

		second, err := svc.Deliver(ctx, hermes.DeliveryParams{
			TenantID:       params.TenantID,
			DraftID:        draft.DraftID,
			RecipientID:    "owner-pg",
			ActorID:        "actor-pg",
			IdempotencyKey: "pg-delivery-key-1",
		})
		if err != nil {
			t.Fatalf("delivery replay: %v", err)
		}
		if second.DeliveryID != first.DeliveryID {
			t.Fatalf("delivery replay must return the same delivery: %s vs %s", first.DeliveryID, second.DeliveryID)
		}

		deliveries, err := svc.ListDeliveries(ctx, params.TenantID)
		if err != nil {
			t.Fatalf("list deliveries: %v", err)
		}
		if len(deliveries) != 1 {
			t.Fatalf("delivery replay must not create duplicates, got %d", len(deliveries))
		}
	})
}

func hermesPostgresReady() bool {
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

func hermesTestPool() (*pgxpool.Pool, error) {
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
	name := testdb.MustName()
	return pgxpool.New(context.Background(), fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, pass, host, port, name))
}
