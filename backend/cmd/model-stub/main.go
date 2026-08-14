package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	httpplatform "comfort-curators-backend/internal/platform/http"
)

type response struct {
	Choices []choice `json:"choices"`
}

type choice struct {
	Message message `json:"message"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type healthResponse struct {
	Status string    `json:"status"`
	Time   time.Time `json:"time"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "model-stub: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	port := os.Getenv("CC_HTTP_PORT")
	if port == "" {
		port = "8080"
	}
	mode := os.Getenv("CC_MODEL_MODE")
	if mode == "" {
		mode = "success"
	}

	var requestCount int64

	mux := http.NewServeMux()

	mux.HandleFunc("/health/live", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(healthResponse{
			Status: "ok",
			Time:   time.Now().UTC(),
		})
	})

	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		effectiveMode := mode
		if qm := r.URL.Query().Get("mode"); qm != "" {
			effectiveMode = qm
		}

		count := atomic.AddInt64(&requestCount, 1)

		switch effectiveMode {
		case "timeout":
			select {
			case <-time.After(30 * time.Second):
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(response{
					Choices: []choice{{
						Message: message{
							Role:    "assistant",
							Content: "delayed response",
						},
					}},
				})
			case <-r.Context().Done():
				return
			}

		case "unavailable":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "provider unavailable",
			})

		case "malformed":
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte("not valid json {{{"))

		case "duplicate":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response{
				Choices: []choice{{
					Message: message{
						Role:    "assistant",
						Content: fmt.Sprintf("duplicate response id=%d", count/2),
					},
				}},
			})

		case "inject":
			w.Header().Set("Content-Type", "application/json")
			var input map[string]any
			body := map[string]any{"echo": "no input"}
			if r.Body != nil {
				_ = json.NewDecoder(r.Body).Decode(&input)
				body = map[string]any{"reflected_input": input}
			}
			json.NewEncoder(w).Encode(response{
				Choices: []choice{{
					Message: message{
						Role:    "assistant",
						Content: fmt.Sprintf("prompt injection test: %v", body),
					},
				}},
			})

		default:
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response{
				Choices: []choice{{
					Message: message{
						Role:    "assistant",
						Content: "deterministic model stub response",
					},
				}},
			})
		}
	})

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      httpplatform.CorrelationID(mux),
		ReadTimeout:  35 * time.Second,
		WriteTimeout: 35 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	idleConnsClosed := make(chan struct{})
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		signal.Stop(sigCh)

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
		close(idleConnsClosed)
	}()

	fmt.Fprintf(os.Stderr, "model-stub listening on :%s (mode=%s)\n", port, mode)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		return fmt.Errorf("http server: %w", err)
	}

	<-idleConnsClosed
	return nil
}
