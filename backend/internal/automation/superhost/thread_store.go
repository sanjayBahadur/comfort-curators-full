package superhost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"comfort-curators-backend/internal/automation"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

type Thread struct {
	ThreadID       string    `json:"thread_id"`
	RunID          string    `json:"run_id"`
	TenantID       string    `json:"tenant_id"`
	PropertyID     string    `json:"property_id"`
	ActorID        string    `json:"actor_id"`
	Purpose        string    `json:"purpose"`
	IdempotencyKey string    `json:"idempotency_key"`
	CreatedAt      time.Time `json:"created_at"`
}

type ThreadStore struct {
	pool      *pgxpool.Pool
	runStore  *automation.AgentRunStore
	assembler *ContextAssembler
}

func NewThreadStore(pool *pgxpool.Pool, runStore *automation.AgentRunStore, assembler *ContextAssembler) *ThreadStore {
	return &ThreadStore{pool: pool, runStore: runStore, assembler: assembler}
}

// CreateThread is idempotent per (tenant, actor, idempotency_key) -- not
// per (tenant, idempotency_key) alone. The frontend's idempotency key
// (see SuperhostMount.tsx) only encodes routeKey and propertyId, with no
// actor component, so scoping by actor here is what keeps two different
// accounts on the same property (two guests, or a guest and staff) from
// resolving to and sharing the same thread.
func (s *ThreadStore) CreateThread(ctx context.Context, tenantID, propertyID, actorID, actorRole, purpose, idempotencyKey string) (*Thread, bool, error) {
	if existing, err := s.GetThreadByIdempotencyKey(ctx, tenantID, actorID, idempotencyKey); err == nil && existing != nil {
		return existing, true, nil
	}

	// propertyID == "" is a portfolio-scoped thread -- see
	// ContextAssembler.AssemblePortfolio. Assemble requires a real
	// property_id, so branch here rather than relaxing that requirement.
	var contextJSON []byte
	if propertyID == "" {
		pc, err := s.assembler.AssemblePortfolio(ctx, tenantID, actorID, actorRole)
		if err != nil {
			return nil, false, err
		}
		contextJSON, err = json.Marshal(pc)
		if err != nil {
			return nil, false, fmt.Errorf("superhost: marshal context: %w", err)
		}
	} else {
		pc, err := s.assembler.Assemble(ctx, tenantID, propertyID, actorID, actorRole)
		if err != nil {
			return nil, false, err
		}
		contextJSON, err = json.Marshal(pc)
		if err != nil {
			return nil, false, fmt.Errorf("superhost: marshal context: %w", err)
		}
	}

	// A thread's very first run used to receive raw context and nothing
	// else -- no message, no instruction. handleSendMessage always wraps
	// context with a real user message ({"context":...,"message":...});
	// this is the same envelope, so a fresh thread's opening turn is a
	// real instruction too, not context with nothing to do with it. This
	// is the actual fix for "there isn't really anything I immediately
	// know to do" -- every new thread, for every role, now opens with
	// Superhost looking at real account data and proposing something
	// concrete, instead of sitting there waiting for the human to think
	// of a prompt.
	// Guest kickoff is deliberately a different message, not just the same
	// prompt applied to a role that happens to see less data. The owner/
	// staff kickoff below surfaces open tickets, stock, and compliance
	// holds -- exactly right for someone operating the property, and
	// exactly the wrong first thing to say to a guest, who has no reason
	// to know or care what the stock balance is. Confirmed live: a guest's
	// own thread was opening with that same operational summary.
	var kickoffContent string
	if actorRole == "guest" {
		kickoffContent = "A guest just opened this during their stay. Look at the real reservation and " +
			"property context you were given -- their actual stay window, real house access info, " +
			"anything genuinely useful to someone staying there right now. In one or two short, plain " +
			"sentences, either mention the one concrete thing worth knowing (their stay dates, a real " +
			"access note) or simply invite them to ask for what they need. Never mention operational " +
			"details meant for the property's own staff -- stock counts, ticket queues, compliance " +
			"holds, account_tasks. Close by inviting them to ask for local recommendations, house " +
			"info, or to report something that needs attention."
	} else {
		kickoffContent = "A new session just started. Look at the real account context you were given " +
			"(open tickets, low stock, pending approvals, upcoming reservations, prior account_tasks -- " +
			"whatever is actually there) and name the single most useful thing you notice, in one short " +
			"sentence -- not a numbered list, not a paragraph per item, no explanation of your reasoning " +
			"yet. If a second thing is genuinely just as pressing, a second short sentence is fine; stop " +
			"there. If nothing genuinely needs attention, say so plainly in one sentence instead of " +
			"manufacturing busywork. Close with a short, plain invitation to ask for more detail or " +
			"anything else -- not a menu of what you could do."
	}
	kickoffInput := map[string]any{
		"type":    "system_kickoff",
		"content": kickoffContent,
	}
	kickoffRaw, err := json.Marshal(kickoffInput)
	if err != nil {
		return nil, false, fmt.Errorf("superhost: marshal kickoff message: %w", err)
	}
	combined := fmt.Sprintf(`{"context":%s,"message":%s}`, string(contextJSON), string(kickoffRaw))
	inputData := json.RawMessage(combined)

	// agent_runs' own idempotency uniqueness is (run_kind, idempotency_key)
	// -- not actor-scoped. The frontend's raw idempotencyKey has no actor
	// component (see the doc comment above), so two different actors
	// opening the same property would make Submit hand back the exact
	// same pre-existing run, whoever asked first -- while
	// superhost_threads IS actor-scoped, so the second actor could never
	// insert a thread row for that shared run: a real collision, found
	// live testing the guest flow. Combining the actor into the key
	// passed to Submit keeps every actor's runs (and, via CreateThread's
	// own scoping above, threads) genuinely separate, while the thread
	// row itself still stores the original frontend-supplied key so
	// GetThreadByIdempotencyKey's (tenant, actor, idempotencyKey) lookups
	// above are unaffected.
	runIdempotencyKey := actorID + ":" + idempotencyKey
	run, duplicate, err := s.runStore.Submit(ctx, automation.SubmitRequest{
		RunKind:        AgentKindSuperhost,
		TenantID:       tenantID,
		PropertyID:     propertyID,
		ActorID:        actorID,
		Provider:       defaultSuperhostProvider(),
		Model:          defaultSuperhostModel(),
		IdempotencyKey: runIdempotencyKey,
		InputData:      inputData,
	})
	if err != nil {
		return nil, false, fmt.Errorf("superhost: create thread run: %w", err)
	}
	if duplicate {
		if existing, err := s.GetThreadByIdempotencyKey(ctx, tenantID, actorID, idempotencyKey); err == nil && existing != nil {
			return existing, true, nil
		}
	}

	threadID := run.RunID
	now := time.Now().UTC()
	_, err = s.pool.Exec(ctx,
		`INSERT INTO superhost_threads (thread_id, run_id, tenant_id, property_id, actor_id, purpose, idempotency_key, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		threadID, run.RunID, tenantID, propertyID, actorID, purpose, idempotencyKey, now,
	)
	if err != nil {
		// A second concurrent call for the same (tenant, actor,
		// idempotency_key) can race past the GetThreadByIdempotencyKey
		// check above before either has inserted -- both then call
		// runStore.Submit, which itself dedupes by idempotency key and
		// hands back the *same* run to both callers, so both attempt to
		// insert a superhost_threads row with that same thread_id (the
		// run's own id, see above). The idx_superhost_threads_idempotency_v2
		// unique index catches this; recover by returning the row the
		// other caller just inserted instead of erroring the request.
		if isUniqueViolation(err) {
			if existing, getErr := s.GetThreadByIdempotencyKey(ctx, tenantID, actorID, idempotencyKey); getErr == nil && existing != nil {
				return existing, true, nil
			}
		}
		return nil, false, fmt.Errorf("superhost: insert thread: %w", err)
	}

	return &Thread{
		ThreadID:       threadID,
		RunID:          run.RunID,
		TenantID:       tenantID,
		PropertyID:     propertyID,
		ActorID:        actorID,
		Purpose:        purpose,
		IdempotencyKey: idempotencyKey,
		CreatedAt:      now,
	}, false, nil
}

func (s *ThreadStore) GetThread(ctx context.Context, threadID string) (*Thread, error) {
	var t Thread
	err := s.pool.QueryRow(ctx,
		`SELECT thread_id, run_id, tenant_id, property_id, actor_id, purpose, idempotency_key, created_at
		 FROM superhost_threads WHERE thread_id = $1`,
		threadID,
	).Scan(&t.ThreadID, &t.RunID, &t.TenantID, &t.PropertyID, &t.ActorID, &t.Purpose, &t.IdempotencyKey, &t.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("superhost: get thread: %w", err)
	}
	return &t, nil
}

func (s *ThreadStore) UpdateThreadRun(ctx context.Context, threadID, runID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE superhost_threads SET run_id = $1 WHERE thread_id = $2`,
		runID, threadID,
	)
	if err != nil {
		return fmt.Errorf("superhost: update thread run: %w", err)
	}
	return nil
}

func (s *ThreadStore) GetThreadByIdempotencyKey(ctx context.Context, tenantID, actorID, idempotencyKey string) (*Thread, error) {
	var t Thread
	err := s.pool.QueryRow(ctx,
		`SELECT thread_id, run_id, tenant_id, property_id, actor_id, purpose, idempotency_key, created_at
		 FROM superhost_threads WHERE tenant_id = $1 AND actor_id = $2 AND idempotency_key = $3
		 ORDER BY created_at DESC LIMIT 1`,
		tenantID, actorID, idempotencyKey,
	).Scan(&t.ThreadID, &t.RunID, &t.TenantID, &t.PropertyID, &t.ActorID, &t.Purpose, &t.IdempotencyKey, &t.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("superhost: get thread by idempotency: %w", err)
	}
	return &t, nil
}
