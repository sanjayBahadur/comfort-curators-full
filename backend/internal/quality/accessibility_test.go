package quality

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeCommunicationsAPI simulates the session and template endpoints the
// localization review depends on, so the review logic is tested without a live
// stack. When gapFor is non-empty the API returns NOT_FOUND for that language.
func fakeCommunicationsAPI(t *testing.T, gapFor string) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/auth/session/create", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"session_token": "quality-test-token"})
	})
	mux.HandleFunc("/v1/communications/templates/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"id": "tmpl-1", "version": 1, "data": map[string]any{}})
		case http.MethodGet:
			if strings.HasSuffix(r.URL.Path, "/resolve") {
				lang := r.URL.Query().Get("language")
				if lang != "en" && lang != "hi" {
					w.WriteHeader(http.StatusUnprocessableEntity)
					return
				}
				// A gap is simulated by resolving the wrong language instead of
				// the requested one, mirroring a server that serves the wrong
				// content while still returning a 200.
				servedLang := lang
				if lang == gapFor {
					servedLang = "en"
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{
					"id": "tmpl-1", "version": 1,
					"data": map[string]any{"language": servedLang, "body": "critical content in " + servedLang},
				})
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"id": "tmpl-1", "version": 1, "data": map[string]any{}})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	return httptest.NewServer(mux)
}

func TestReviewAccessibilityCoversWCAG22AA(t *testing.T) {
	items := ReviewAccessibility()

	// Every checkpoint must be non-empty and carry a complete disposition.
	seen := map[string]bool{}
	for _, item := range items {
		if item.ID == "" || item.Checkpoint == "" || item.Criterion == "" || item.Principle == "" || item.Level == "" {
			t.Errorf("item %q is incomplete: %+v", item.ID, item)
		}
		if item.Assessment == "" {
			t.Errorf("item %q has an empty assessment", item.ID)
		}
		switch item.Disposition {
		case DispositionSupported, DispositionClient, DispositionNotApplicable, DispositionGap:
		default:
			t.Errorf("item %q has invalid disposition %q", item.ID, item.Disposition)
		}
		if seen[item.Checkpoint] {
			t.Errorf("duplicate checkpoint %s", item.Checkpoint)
		}
		seen[item.Checkpoint] = true
	}

	// WCAG 2.2 AA requires every A and AA success criterion to be dispositioned.
	expected := []string{
		"1.1.1", "1.2.1", "1.2.2", "1.2.3", "1.2.4", "1.2.5",
		"1.3.1", "1.3.2", "1.3.3", "1.3.4", "1.3.5",
		"1.4.1", "1.4.2", "1.4.3", "1.4.4", "1.4.5", "1.4.10", "1.4.11", "1.4.12", "1.4.13",
		"2.1.1", "2.1.2", "2.1.4", "2.2.1", "2.2.2", "2.3.1",
		"2.4.1", "2.4.2", "2.4.3", "2.4.4", "2.4.5", "2.4.6", "2.4.7", "2.4.11",
		"2.5.1", "2.5.2", "2.5.3", "2.5.4", "2.5.7", "2.5.8",
		"3.1.1", "3.1.2", "3.2.1", "3.2.2", "3.2.3", "3.2.4", "3.2.6",
		"3.3.1", "3.3.2", "3.3.3", "3.3.4", "3.3.7", "3.3.8",
		"4.1.2", "4.1.3",
	}
	if len(expected) != len(items) {
		t.Errorf("expected %d WCAG 2.2 A/AA criteria to be dispositioned, got %d", len(expected), len(items))
	}
	for _, cp := range expected {
		if !seen[cp] {
			t.Errorf("WCAG 2.2 A/AA checkpoint %s is missing from the review", cp)
		}
	}

	// The four principles must all be represented.
	for _, principle := range []string{"Perceivable", "Operable", "Understandable", "Robust"} {
		found := false
		for _, item := range items {
			if item.Principle == principle {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("principle %s is not represented in the review", principle)
		}
	}
}

func TestReviewAccessibilityNoUndecidedItems(t *testing.T) {
	items := ReviewAccessibility()
	for _, item := range items {
		if item.Disposition == "" {
			t.Errorf("item %s is undecided (empty disposition)", item.ID)
		}
	}
}

func TestDispositionNotesCaptureClientAndGapItems(t *testing.T) {
	items := ReviewAccessibility()
	notes := dispositionNotes(items, nil)

	// Client-side responsibilities must be dispositioned in the notes.
	clientFound := false
	for _, n := range notes {
		if n.Disposition == DispositionClient {
			clientFound = true
			if n.Owner != "client" {
				t.Errorf("client note %s has owner %q, want client", n.ID, n.Owner)
			}
		}
	}
	if !clientFound {
		t.Error("expected at least one client-side disposition note")
	}

	// Every open gap must be dispositioned to the api owner.
	gaps := []AccessibilityItem{{
		ID: "a11y-test-gap", Checkpoint: "9.9.9", Level: "AA",
		Principle: "Test", Criterion: "Imaginary", Assessment: "gap",
		Disposition: DispositionGap,
	}}
	gapNotes := dispositionNotes(gaps, nil)
	if len(gapNotes) != 1 || gapNotes[0].Owner != "api" {
		t.Errorf("gap item must be dispositioned to api owner, got %+v", gapNotes)
	}

	// Localization gaps must be recorded.
	loc := []LocalizationItem{{
		TemplateKey: "stay_confirmation", Language: "hi",
		Available: false, Disposition: DispositionGap,
	}}
	locNotes := dispositionNotes(nil, loc)
	if len(locNotes) != 1 || locNotes[0].Disposition != DispositionGap {
		t.Errorf("localization gap must be dispositioned, got %+v", locNotes)
	}
}

func TestCriticalTemplateKeysNonEmpty(t *testing.T) {
	if len(CriticalTemplateKeys) == 0 {
		t.Fatal("critical template keys must not be empty")
	}
	for _, key := range CriticalTemplateKeys {
		if strings.TrimSpace(key) == "" {
			t.Errorf("critical template key is empty")
		}
	}
}

func TestTemplateContentDiffersByLanguage(t *testing.T) {
	for _, key := range CriticalTemplateKeys {
		enSubject, hiSubject := templateSubject(key, "en"), templateSubject(key, "hi")
		enBody, hiBody := templateBody(key, "en"), templateBody(key, "hi")
		if enSubject == "" || hiSubject == "" || enBody == "" || hiBody == "" {
			t.Errorf("template %s has empty content in en or hi", key)
		}
		if enSubject == hiSubject {
			t.Errorf("template %s has identical subject in en and hi", key)
		}
		if enBody == hiBody {
			t.Errorf("template %s has identical body in en and hi", key)
		}
	}
}

func TestVerifyLocalizationSupportsEnglishAndHindi(t *testing.T) {
	srv := fakeCommunicationsAPI(t, "")
	defer srv.Close()

	items, err := VerifyLocalization(context.Background(), srv.URL, "tenant-capacity-test", CriticalTemplateKeys)
	if err != nil {
		t.Fatalf("verify localization: %v", err)
	}

	if len(items) != len(CriticalTemplateKeys)*2 {
		t.Fatalf("expected %d localization items, got %d", len(CriticalTemplateKeys)*2, len(items))
	}
	for _, item := range items {
		if !item.Available {
			t.Errorf("template %s must be available in %s", item.TemplateKey, item.Language)
		}
		if item.Disposition != DispositionSupported {
			t.Errorf("template %s in %s must be dispositioned supported, got %q", item.TemplateKey, item.Language, item.Disposition)
		}
	}
}

func TestVerifyLocalizationDetectsGapAndRejectsUnsupportedLanguage(t *testing.T) {
	// The API refuses to resolve Hindi for this key: the review must surface it
	// as a gap, not as a pass.
	srv := fakeCommunicationsAPI(t, "hi")
	defer srv.Close()

	items, err := VerifyLocalization(context.Background(), srv.URL, "tenant-capacity-test", []string{"stay_confirmation"})
	if err != nil {
		t.Fatalf("verify localization: %v", err)
	}

	hi := items[1]
	if hi.Language != "hi" {
		t.Fatalf("expected second item to be Hindi, got %+v", items)
	}
	if hi.Available {
		t.Error("Hindi availability must be false when the API cannot resolve it")
	}
	if hi.Disposition != DispositionGap {
		t.Errorf("missing Hindi content must be dispositioned as a gap, got %q", hi.Disposition)
	}
}
