package automation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"
)

const streamPollInterval = 500 * time.Millisecond

func (h *AgentRunHandler) handleStream(w http.ResponseWriter, r *http.Request) {
	tenantID, _, _, err := subjectFromRequest(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	threadID := r.PathValue("thread_id")

	var threadFollowMode bool
	runID, err := h.store.GetRunIDByThreadID(r.Context(), threadID)
	if err != nil {
		runID = threadID
	} else {
		threadFollowMode = true
	}

	run, err := h.store.Get(r.Context(), runID)
	if err != nil {
		if errors.Is(err, ErrRunNotFound) {
			writeError(w, r, http.StatusNotFound, "NOT_FOUND", "run not found")
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if run.TenantID != tenantID {
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "not authorized for this run")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, r, http.StatusServiceUnavailable, "STREAM_UNAVAILABLE", "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	var cursorTime time.Time
	cursorEventID := "00000000-0000-0000-0000-000000000000"
	lastStreamingText := ""

	terminal, err := h.streamDeliver(r.Context(), w, flusher, runID, &cursorTime, &cursorEventID)
	if err != nil {
		return
	}
	if terminal {
		if _, err := h.streamDeliver(r.Context(), w, flusher, runID, &cursorTime, &cursorEventID); err != nil {
			return
		}
		if threadFollowMode {
			switched, ok := h.tryFollowThreadRun(r.Context(), threadID, tenantID, &runID, &cursorTime, &cursorEventID)
			if !ok {
				return
			}
			if !switched {
				fmt.Fprintf(w, "data: [DONE]\n\n")
				flusher.Flush()
				return
			}
			lastStreamingText = ""
		} else {
			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
			return
		}
	}

	if err := h.streamDeliverText(r.Context(), w, flusher, runID, &lastStreamingText); err != nil {
		return
	}

	ticker := time.NewTicker(streamPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if threadFollowMode {
				switched, ok := h.tryFollowThreadRun(r.Context(), threadID, tenantID, &runID, &cursorTime, &cursorEventID)
				if !ok {
					return
				}
				if switched {
					lastStreamingText = ""
				}
			}

			if err := h.streamDeliverText(r.Context(), w, flusher, runID, &lastStreamingText); err != nil {
				return
			}

			terminal, err := h.streamDeliver(r.Context(), w, flusher, runID, &cursorTime, &cursorEventID)
			if err != nil {
				return
			}
			if terminal {
				if _, err := h.streamDeliver(r.Context(), w, flusher, runID, &cursorTime, &cursorEventID); err != nil {
					return
				}
				if threadFollowMode {
					switched, ok := h.tryFollowThreadRun(r.Context(), threadID, tenantID, &runID, &cursorTime, &cursorEventID)
					if !ok {
						return
					}
					if switched {
						lastStreamingText = ""
						continue
					}
				}
				fmt.Fprintf(w, "data: [DONE]\n\n")
				flusher.Flush()
				return
			}
		}
	}
}

// streamDeliverText is the live-typing counterpart to streamDeliver: it
// writes a synthetic, non-persisted AgentRunToken.v1 frame carrying the
// run's current in-progress narrative text (see
// AgentRunStore.UpdateStreamingText) whenever that text has changed since
// the last tick. This never advances the real event cursor and is not a
// row in agent_run_events -- it's a live view of state that's about to be
// superseded by a real, persisted AgentRunCompleted.v1 (or similar) event,
// not a record in its own right.
func (h *AgentRunHandler) streamDeliverText(ctx context.Context, w http.ResponseWriter, flusher http.Flusher, runID string, lastSent *string) error {
	text, err := h.store.GetStreamingText(ctx, runID)
	if err != nil {
		// Best-effort: a run that no longer exists or a transient read
		// error here shouldn't tear down the whole SSE connection the way
		// a streamDeliver failure does -- the real events still will.
		return nil
	}
	if text == "" || text == *lastSent {
		return nil
	}
	*lastSent = text

	eventData, err := json.Marshal(map[string]any{"text": text})
	if err != nil {
		return nil
	}
	ev := AgentRunEvent{
		EventID:    "token-" + runID,
		RunID:      runID,
		EventName:  "AgentRunToken.v1",
		EventData:  eventData,
		OccurredAt: time.Now().UTC(),
	}
	if err := writeSSEEvent(w, ev); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func (h *AgentRunHandler) tryFollowThreadRun(ctx context.Context, threadID, tenantID string, runID *string, cursorTime *time.Time, cursorEventID *string) (switched bool, ok bool) {
	newRunID, err := h.store.GetRunIDByThreadID(ctx, threadID)
	if err != nil || newRunID == *runID {
		return false, true
	}
	run, err := h.store.Get(ctx, newRunID)
	if err != nil {
		log.Printf("automation: stream: cannot load new run %s: %v", newRunID, err)
		return false, false
	}
	if run.TenantID != tenantID {
		return false, false
	}
	*runID = newRunID
	*cursorTime = time.Time{}
	*cursorEventID = "00000000-0000-0000-0000-000000000000"
	return true, true
}

// streamDeliver fetches and writes every event after the cursor, advances
// the cursor past what it sent, and reports whether the run is now in a
// terminal state. It checks terminal state *after* fetching events (see
// handleStream's comment) so a terminal-event write racing the run's state
// UPDATE cannot be missed.
func (h *AgentRunHandler) streamDeliver(ctx context.Context, w http.ResponseWriter, flusher http.Flusher, runID string, cursorTime *time.Time, cursorEventID *string) (terminal bool, err error) {
	newEvents, err := ListEventsAfter(ctx, h.store.pool, runID, *cursorTime, *cursorEventID)
	if err != nil {
		log.Printf("automation: stream: list events after cursor: %v", err)
		return false, err
	}

	for _, ev := range newEvents {
		if err := writeSSEEvent(w, ev); err != nil {
			return false, err
		}
		*cursorTime = ev.OccurredAt
		*cursorEventID = ev.EventID
	}
	if len(newEvents) > 0 {
		flusher.Flush()
	}

	run, err := h.store.Get(ctx, runID)
	if err != nil {
		return false, err
	}
	return IsTerminal(run.State), nil
}

func writeSSEEvent(w http.ResponseWriter, ev AgentRunEvent) error {
	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "data: %s\n\n", string(data))
	return err
}
