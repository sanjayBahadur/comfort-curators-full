package superhost

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"comfort-curators-backend/internal/automation"
)

func TestRegisterRoutes_RouteRenameAndRedirect(t *testing.T) {
	store := &automation.AgentRunStore{}
	h := NewHandler(store, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// POST /v1/superhost/runs — primary route works directly.
	// With no auth header, handleCreateRun deterministically returns 401 from
	// subjectFromRequest (handler.go's first statement) before touching the
	// body or the store. An exact 401 check is strictly stronger than "not
	// 404 / not 308": it still catches a missing route (404) and a shim that
	// failed to redirect (308), and now also any other unexpected status.
	resp, err := http.Post(srv.URL+"/v1/superhost/runs", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("POST /v1/superhost/runs: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("POST /v1/superhost/runs got %d, want %d (route not registered, or unauthenticated request mishandled)", resp.StatusCode, http.StatusUnauthorized)
	}

	// POST /v1/jarvis/runs — redirect shim returns 308 to /v1/superhost/runs.
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/jarvis/runs", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	resp2, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST /v1/jarvis/runs: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusPermanentRedirect {
		t.Errorf("POST /v1/jarvis/runs expected 308, got %d", resp2.StatusCode)
	}
	loc := resp2.Header.Get("Location")
	if loc != "/v1/superhost/runs" {
		t.Errorf("POST /v1/jarvis/runs Location header: got %q, want %q", loc, "/v1/superhost/runs")
	}

	// Confirm redirect is followed transparently with default client.
	// The followed request reaches the same handler without an auth header,
	// so it must deterministically end at 401. An exact 401 check catches a
	// redirect that was not followed (would stay 308) and a missing target
	// route (404), as well as any other unexpected status.
	defaultClient := &http.Client{}
	resp3, err := defaultClient.Post(srv.URL+"/v1/jarvis/runs", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("POST /v1/jarvis/runs (follow redirect): %v", err)
	}
	defer resp3.Body.Close()
	_, _ = io.ReadAll(resp3.Body)
	if resp3.StatusCode != http.StatusUnauthorized {
		t.Errorf("POST /v1/jarvis/runs (follow redirect) got %d, want %d (redirect not followed, or target route missing)", resp3.StatusCode, http.StatusUnauthorized)
	}
}

// TestPostBodyPreservedAcross308Redirect proves the body-preservation
// mechanism generically, independent of handleCreateRun's auth gate. This
// test's redirect target is a plain handler that echoes the request body
// back, so a 200 + echoed body is direct evidence the POST body survived the
// 308. handleCreateRun itself cannot demonstrate this: it returns 401 before
// reading the body, so handler_test.go's route test only relies on Go's
// documented http.Client redirect behavior (method + body preserved for
// 307/308 when the request has a GetBody, which strings.NewReader provides).
func TestPostBodyPreservedAcross308Redirect(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/echo", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read body: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	})
	mux.HandleFunc("POST /v1/old", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/v1/echo", http.StatusPermanentRedirect)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	want := `{"ping":"pong"}`
	resp, err := http.Post(srv.URL+"/v1/old", "application/json", strings.NewReader(want))
	if err != nil {
		t.Fatalf("POST /v1/old: %v", err)
	}
	defer resp.Body.Close()
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read echoed body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("POST /v1/old (follow redirect) got %d, want 200", resp.StatusCode)
	}
	if string(got) != want {
		t.Errorf("body lost across 308 redirect: got %q, want %q", got, want)
	}
}

func TestHandleSendMessageUISurfacesDeserialization(t *testing.T) {
	body := `{
		"idempotency_key": "test-key-12345678",
		"content": "hello",
		"ui_surfaces": [
			{"id": "btn-submit", "label": "Submit", "actions": ["ui_click", "ui_focus"]},
			{"id": "input-name", "label": "Name", "actions": ["ui_set_value"]}
		]
	}`

	var req struct {
		IdempotencyKey string           `json:"idempotency_key"`
		Content        string           `json:"content"`
		UISurfaces     []UISurfaceInput `json:"ui_surfaces,omitempty"`
	}
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("failed to unmarshal ui_surfaces payload: %v", err)
	}
	if len(req.UISurfaces) != 2 {
		t.Fatalf("expected 2 surfaces, got %d", len(req.UISurfaces))
	}
	if req.UISurfaces[0].ID != "btn-submit" {
		t.Errorf("surface[0].ID: got %q, want %q", req.UISurfaces[0].ID, "btn-submit")
	}
	if req.UISurfaces[0].Label != "Submit" {
		t.Errorf("surface[0].Label: got %q, want %q", req.UISurfaces[0].Label, "Submit")
	}
	if len(req.UISurfaces[0].Actions) != 2 {
		t.Errorf("surface[0].Actions len: got %d, want 2", len(req.UISurfaces[0].Actions))
	}
	if req.UISurfaces[1].ID != "input-name" {
		t.Errorf("surface[1].ID: got %q, want %q", req.UISurfaces[1].ID, "input-name")
	}
	if req.Content != "hello" {
		t.Errorf("content: got %q, want %q", req.Content, "hello")
	}
}

func TestHandleSendMessageUISurfacesEmptyNotRequired(t *testing.T) {
	body := `{
		"idempotency_key": "test-key-12345678",
		"content": "hello"
	}`

	var req struct {
		IdempotencyKey string           `json:"idempotency_key"`
		Content        string           `json:"content"`
		UISurfaces     []UISurfaceInput `json:"ui_surfaces,omitempty"`
	}
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("failed to unmarshal without ui_surfaces: %v", err)
	}
	if req.UISurfaces != nil {
		t.Errorf("expected nil ui_surfaces when absent, got %v", req.UISurfaces)
	}
	if req.Content != "hello" {
		t.Errorf("content: got %q, want %q", req.Content, "hello")
	}
}

func TestHandleSendMessageUISurfacesSentInMsgInput(t *testing.T) {
	surfaces := []UISurfaceInput{
		{ID: "btn-1", Label: "Button 1", Actions: []string{"ui_click"}},
	}
	content := "check page"

	msgInput := map[string]any{
		"type":        "user_message",
		"content":     content,
		"ui_surfaces": surfaces,
	}

	msgRaw, err := json.Marshal(msgInput)
	if err != nil {
		t.Fatalf("failed to marshal msgInput: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(msgRaw, &parsed); err != nil {
		t.Fatalf("failed to unmarshal msgInput: %v", err)
	}

	if parsed["type"] != "user_message" {
		t.Errorf("type: got %v, want user_message", parsed["type"])
	}
	if parsed["content"] != "check page" {
		t.Errorf("content: got %v, want 'check page'", parsed["content"])
	}

	surfacesRaw, ok := parsed["ui_surfaces"]
	if !ok {
		t.Fatal("ui_surfaces key missing from msgInput")
	}
	surfacesList, ok := surfacesRaw.([]interface{})
	if !ok {
		t.Fatalf("ui_surfaces not a list, got %T", surfacesRaw)
	}
	if len(surfacesList) != 1 {
		t.Errorf("expected 1 surface, got %d", len(surfacesList))
	}
}

func TestRenderUISurfacesEmpty(t *testing.T) {
	got := renderUISurfaces(nil)
	want := "Available UI surfaces: none registered on the current page."
	if got != want {
		t.Errorf("empty: got %q, want %q", got, want)
	}

	got = renderUISurfaces([]UISurfaceInput{})
	if got != want {
		t.Errorf("zero-len: got %q, want %q", got, want)
	}
}

func TestRenderUISurfacesNonEmpty(t *testing.T) {
	surfaces := []UISurfaceInput{
		{ID: "btn-submit", Label: "Submit order", Actions: []string{"click", "focus"}},
		{ID: "input-search", Label: "Search properties", Actions: []string{"focus", "set"}},
	}

	got := renderUISurfaces(surfaces)

	if !strings.Contains(got, "Available UI surfaces:") {
		t.Errorf("missing heading: %q", got)
	}
	if !strings.Contains(got, `id: "btn-submit"`) {
		t.Errorf("missing btn-submit: %q", got)
	}
	if !strings.Contains(got, `label: "Submit order"`) {
		t.Errorf("missing Submit order label: %q", got)
	}
	if !strings.Contains(got, `actions: [click, focus]`) {
		t.Errorf("missing actions for btn-submit: %q", got)
	}
	if !strings.Contains(got, `id: "input-search"`) {
		t.Errorf("missing input-search: %q", got)
	}
	if !strings.Contains(got, `label: "Search properties"`) {
		t.Errorf("missing Search properties label: %q", got)
	}
	if !strings.Contains(got, `actions: [focus, set]`) {
		t.Errorf("missing actions for input-search: %q", got)
	}
}
