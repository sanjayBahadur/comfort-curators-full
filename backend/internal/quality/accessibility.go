package quality

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Disposition is the outcome of reviewing one WCAG 2.2 AA or localization item.
type Disposition string

const (
	// DispositionSupported means the API itself satisfies or directly enables
	// the checkpoint through structured data, errors or content.
	DispositionSupported Disposition = "supported"
	// DispositionClient means the checkpoint is a rendering or interaction
	// responsibility of the consuming web client; the API provides the data.
	DispositionClient Disposition = "client-side"
	// DispositionNotApplicable means the checkpoint cannot apply to a JSON API
	// (for example criteria that only concern rendered audio or video media).
	DispositionNotApplicable Disposition = "not-applicable"
	// DispositionGap means an open issue that the API itself must address.
	DispositionGap Disposition = "gap"
)

// AccessibilityItem is one dispositioned WCAG 2.2 A/AA success criterion for
// the server API surface.
type AccessibilityItem struct {
	ID          string      `json:"id"`
	Checkpoint  string      `json:"checkpoint"`
	Level       string      `json:"level"`
	Principle   string      `json:"principle"`
	Criterion   string      `json:"criterion"`
	Assessment  string      `json:"assessment"`
	Disposition Disposition `json:"disposition"`
}

// ReviewAccessibility dispositioned every WCAG 2.2 A/AA success criterion for
// the backend API. The API serves structured JSON with machine-readable error
// codes, tenant-scoped resources and English/Hindi content; visual and pointer
// criteria are the consuming web client's responsibility and are recorded as
// such rather than left undecided.
func ReviewAccessibility() []AccessibilityItem {
	items := []AccessibilityItem{
		// Principle 1: Perceivable
		{ID: "a11y-1.1.1", Checkpoint: "1.1.1", Level: "A", Principle: "Perceivable", Criterion: "Non-text Content",
			Assessment:  "API returns descriptive text and structured metadata (labels, alt_text, descriptions) for referenced non-text assets such as evidence files, photos and catalog images.",
			Disposition: DispositionSupported},
		{ID: "a11y-1.2.1", Checkpoint: "1.2.1", Level: "A", Principle: "Perceivable", Criterion: "Audio-only and Video-only (Prerecorded)",
			Assessment:  "The MVP API serves no prerecorded audio or video media.",
			Disposition: DispositionNotApplicable},
		{ID: "a11y-1.2.2", Checkpoint: "1.2.2", Level: "A", Principle: "Perceivable", Criterion: "Captions (Prerecorded)",
			Assessment:  "The MVP API serves no prerecorded audio or video media.",
			Disposition: DispositionNotApplicable},
		{ID: "a11y-1.2.3", Checkpoint: "1.2.3", Level: "A", Principle: "Perceivable", Criterion: "Audio Description or Media Alternative (Prerecorded)",
			Assessment:  "The MVP API serves no prerecorded audio or video media.",
			Disposition: DispositionNotApplicable},
		{ID: "a11y-1.2.4", Checkpoint: "1.2.4", Level: "AA", Principle: "Perceivable", Criterion: "Captions (Live)",
			Assessment:  "The MVP API serves no live audio or video media.",
			Disposition: DispositionNotApplicable},
		{ID: "a11y-1.2.5", Checkpoint: "1.2.5", Level: "AA", Principle: "Perceivable", Criterion: "Audio Description (Prerecorded)",
			Assessment:  "The MVP API serves no prerecorded audio or video media.",
			Disposition: DispositionNotApplicable},
		{ID: "a11y-1.3.1", Checkpoint: "1.3.1", Level: "A", Principle: "Perceivable", Criterion: "Info and Relationships",
			Assessment:  "API resources are structured JSON with typed fields, stable identifiers, versions and explicit relationships; the client renders semantic markup from them.",
			Disposition: DispositionSupported},
		{ID: "a11y-1.3.2", Checkpoint: "1.3.2", Level: "A", Principle: "Perceivable", Criterion: "Meaningful Sequence",
			Assessment:  "List and cursor-paginated resources preserve a deterministic ordering (id-ordered, cursor-based) so the client can present a meaningful sequence.",
			Disposition: DispositionSupported},
		{ID: "a11y-1.3.3", Checkpoint: "1.3.3", Level: "A", Principle: "Perceivable", Criterion: "Sensory Characteristics",
			Assessment:  "API responses identify items by id, code and text label rather than by colour, shape or position.",
			Disposition: DispositionSupported},
		{ID: "a11y-1.3.4", Checkpoint: "1.3.4", Level: "AA", Principle: "Perceivable", Criterion: "Orientation",
			Assessment:  "API is a data contract with no fixed orientation; rendering orientation is a client responsibility.",
			Disposition: DispositionClient},
		{ID: "a11y-1.3.5", Checkpoint: "1.3.5", Level: "AA", Principle: "Perceivable", Criterion: "Identify Input Purpose",
			Assessment:  "Typed form fields in the API contract (address, contact method, dates, money in minor units and currency) let the client expose the correct input purpose.",
			Disposition: DispositionSupported},
		{ID: "a11y-1.4.1", Checkpoint: "1.4.1", Level: "A", Principle: "Perceivable", Criterion: "Use of Color",
			Assessment:  "API conveys state through text codes and structured fields, not colour; colour rendering is a client responsibility.",
			Disposition: DispositionClient},
		{ID: "a11y-1.4.2", Checkpoint: "1.4.2", Level: "A", Principle: "Perceivable", Criterion: "Audio Control",
			Assessment:  "The MVP API serves no audio.",
			Disposition: DispositionNotApplicable},
		{ID: "a11y-1.4.3", Checkpoint: "1.4.3", Level: "AA", Principle: "Perceivable", Criterion: "Contrast (Minimum)",
			Assessment:  "Contrast is a visual rendering property of the consuming web client; the API provides no colour presentation.",
			Disposition: DispositionClient},
		{ID: "a11y-1.4.4", Checkpoint: "1.4.4", Level: "AA", Principle: "Perceivable", Criterion: "Resize Text",
			Assessment:  "Text rendering and resize behaviour is a client responsibility; API content is plain text without fixed rendering size.",
			Disposition: DispositionClient},
		{ID: "a11y-1.4.5", Checkpoint: "1.4.5", Level: "AA", Principle: "Perceivable", Criterion: "Images of Text",
			Assessment:  "API serves text as text; images of text, where referenced, carry alt text. Rendering is a client responsibility.",
			Disposition: DispositionSupported},
		{ID: "a11y-1.4.10", Checkpoint: "1.4.10", Level: "AA", Principle: "Perceivable", Criterion: "Reflow",
			Assessment:  "Reflow behaviour applies to rendered web pages in the consuming client.",
			Disposition: DispositionClient},
		{ID: "a11y-1.4.11", Checkpoint: "1.4.11", Level: "AA", Principle: "Perceivable", Criterion: "Non-text Contrast",
			Assessment:  "Visual contrast of interface components is a client rendering responsibility.",
			Disposition: DispositionClient},
		{ID: "a11y-1.4.12", Checkpoint: "1.4.12", Level: "AA", Principle: "Perceivable", Criterion: "Text Spacing",
			Assessment:  "Text spacing is applied by the consuming client; API content does not constrain line height or letter spacing.",
			Disposition: DispositionClient},
		{ID: "a11y-1.4.13", Checkpoint: "1.4.13", Level: "AA", Principle: "Perceivable", Criterion: "Content on Hover or Focus",
			Assessment:  "Hover and focus behaviour is implemented and owned by the consuming web client.",
			Disposition: DispositionClient},

		// Principle 2: Operable
		{ID: "a11y-2.1.1", Checkpoint: "2.1.1", Level: "A", Principle: "Operable", Criterion: "Keyboard",
			Assessment:  "Keyboard operation is a client interaction responsibility; the API exposes deterministic, addressable resources.",
			Disposition: DispositionClient},
		{ID: "a11y-2.1.2", Checkpoint: "2.1.2", Level: "A", Principle: "Operable", Criterion: "No Keyboard Trap",
			Assessment:  "Keyboard trap prevention is a client rendering responsibility.",
			Disposition: DispositionClient},
		{ID: "a11y-2.1.4", Checkpoint: "2.1.4", Level: "A", Principle: "Operable", Criterion: "Character Key Shortcuts",
			Assessment:  "Shortcut handling is a client responsibility; the API defines no character-key shortcuts.",
			Disposition: DispositionClient},
		{ID: "a11y-2.2.1", Checkpoint: "2.2.1", Level: "A", Principle: "Operable", Criterion: "Timing Adjustable",
			Assessment:  "API-driven time constraints are adjustable: secure links and support grants expose explicit TTLs, and operations either set adjustable windows or are extendable by design.",
			Disposition: DispositionSupported},
		{ID: "a11y-2.2.2", Checkpoint: "2.2.2", Level: "A", Principle: "Operable", Criterion: "Pause, Stop, Hide",
			Assessment:  "The API serves no moving, blinking or scrolling content.",
			Disposition: DispositionNotApplicable},
		{ID: "a11y-2.3.1", Checkpoint: "2.3.1", Level: "A", Principle: "Operable", Criterion: "Three Flashes or Below Threshold",
			Assessment:  "The API serves no flashing content.",
			Disposition: DispositionNotApplicable},
		{ID: "a11y-2.4.1", Checkpoint: "2.4.1", Level: "A", Principle: "Operable", Criterion: "Bypass Blocks",
			Assessment:  "Navigation structure and skip mechanisms are client rendering responsibilities.",
			Disposition: DispositionClient},
		{ID: "a11y-2.4.2", Checkpoint: "2.4.2", Level: "A", Principle: "Operable", Criterion: "Page Titled",
			Assessment:  "Page titles are rendered by the consuming client from resource metadata.",
			Disposition: DispositionClient},
		{ID: "a11y-2.4.3", Checkpoint: "2.4.3", Level: "A", Principle: "Operable", Criterion: "Focus Order",
			Assessment:  "Focus order is a client rendering responsibility; API list ordering is deterministic.",
			Disposition: DispositionClient},
		{ID: "a11y-2.4.4", Checkpoint: "2.4.4", Level: "A", Principle: "Operable", Criterion: "Link Purpose (In Context)",
			Assessment:  "API link targets carry explicit purpose-bearing identifiers and context (secure links with purpose, reference_type/reference_id on movements).",
			Disposition: DispositionSupported},
		{ID: "a11y-2.4.5", Checkpoint: "2.4.5", Level: "AA", Principle: "Operable", Criterion: "Multiple Ways",
			Assessment:  "The API exposes multiple deterministic lookup paths (list with cursor, get by id, resolve by key) so clients can offer multiple navigation routes.",
			Disposition: DispositionSupported},
		{ID: "a11y-2.4.6", Checkpoint: "2.4.6", Level: "AA", Principle: "Operable", Criterion: "Headings and Labels",
			Assessment:  "API resources carry stable human-readable labels and codes that the client can surface as headings and labels.",
			Disposition: DispositionSupported},
		{ID: "a11y-2.4.7", Checkpoint: "2.4.7", Level: "AA", Principle: "Operable", Criterion: "Focus Visible",
			Assessment:  "Focus visibility is a client rendering responsibility.",
			Disposition: DispositionClient},
		{ID: "a11y-2.4.11", Checkpoint: "2.4.11", Level: "AA", Principle: "Operable", Criterion: "Focus Not Obscured (Minimum)",
			Assessment:  "Focus overlay behaviour is a client rendering responsibility.",
			Disposition: DispositionClient},
		{ID: "a11y-2.5.1", Checkpoint: "2.5.1", Level: "A", Principle: "Operable", Criterion: "Pointer Gestures",
			Assessment:  "Pointer gesture handling is a client responsibility.",
			Disposition: DispositionClient},
		{ID: "a11y-2.5.2", Checkpoint: "2.5.2", Level: "A", Principle: "Operable", Criterion: "Pointer Cancellation",
			Assessment:  "Pointer cancellation is a client interaction responsibility.",
			Disposition: DispositionClient},
		{ID: "a11y-2.5.3", Checkpoint: "2.5.3", Level: "A", Principle: "Operable", Criterion: "Label in Name",
			Assessment:  "API field labels and codes are text that matches accessible names when the client renders them.",
			Disposition: DispositionSupported},
		{ID: "a11y-2.5.4", Checkpoint: "2.5.4", Level: "A", Principle: "Operable", Criterion: "Motion Actuation",
			Assessment:  "Motion-actuated interaction is a client responsibility; the API defines no motion-triggered behaviour.",
			Disposition: DispositionClient},
		{ID: "a11y-2.5.7", Checkpoint: "2.5.7", Level: "AA", Principle: "Operable", Criterion: "Dragging Movements",
			Assessment:  "Dragging is a client interaction responsibility.",
			Disposition: DispositionClient},
		{ID: "a11y-2.5.8", Checkpoint: "2.5.8", Level: "AA", Principle: "Operable", Criterion: "Target Size (Minimum)",
			Assessment:  "Target size is a client rendering responsibility.",
			Disposition: DispositionClient},

		// Principle 3: Understandable
		{ID: "a11y-3.1.1", Checkpoint: "3.1.1", Level: "A", Principle: "Understandable", Criterion: "Language of Page",
			Assessment:  "API template and draft content is versioned per language (en/hi) so clients can render a page in a declared default language.",
			Disposition: DispositionSupported},
		{ID: "a11y-3.1.2", Checkpoint: "3.1.2", Level: "AA", Principle: "Understandable", Criterion: "Language of Parts",
			Assessment:  "API content is resolved by language on a per-template basis; a client can mark each part with the resolved language.",
			Disposition: DispositionSupported},
		{ID: "a11y-3.2.1", Checkpoint: "3.2.1", Level: "A", Principle: "Understandable", Criterion: "On Focus",
			Assessment:  "Focus-driven context changes are a client responsibility; API state changes are explicit operations.",
			Disposition: DispositionClient},
		{ID: "a11y-3.2.2", Checkpoint: "3.2.2", Level: "A", Principle: "Understandable", Criterion: "On Input",
			Assessment:  "API mutations are explicit, idempotency-keyed operations; no implicit context change occurs on input.",
			Disposition: DispositionSupported},
		{ID: "a11y-3.2.3", Checkpoint: "3.2.3", Level: "AA", Principle: "Understandable", Criterion: "Consistent Navigation",
			Assessment:  "Navigation consistency is a client rendering responsibility; the API exposes stable resource paths.",
			Disposition: DispositionClient},
		{ID: "a11y-3.2.4", Checkpoint: "3.2.4", Level: "AA", Principle: "Understandable", Criterion: "Consistent Identification",
			Assessment:  "The API uses stable identifiers, keys and codes consistently across resources.",
			Disposition: DispositionSupported},
		{ID: "a11y-3.2.6", Checkpoint: "3.2.6", Level: "A", Principle: "Understandable", Criterion: "Consistent Help",
			Assessment:  "Help placement is a client responsibility; API responses include actionable error messages and codes.",
			Disposition: DispositionClient},
		{ID: "a11y-3.3.1", Checkpoint: "3.3.1", Level: "A", Principle: "Understandable", Criterion: "Error Identification",
			Assessment:  "API errors carry a machine-readable code, a human-readable message and (where relevant) a request_id and details so the client can identify the failing input.",
			Disposition: DispositionSupported},
		{ID: "a11y-3.3.2", Checkpoint: "3.3.2", Level: "A", Principle: "Understandable", Criterion: "Labels or Instructions",
			Assessment:  "API request fields are typed and documented with explicit labels; critical flows expose plain-language instructions.",
			Disposition: DispositionSupported},
		{ID: "a11y-3.3.3", Checkpoint: "3.3.3", Level: "AA", Principle: "Understandable", Criterion: "Error Suggestion",
			Assessment:  "Validation failures return descriptive messages that identify the field and the expected shape so the client can suggest corrections.",
			Disposition: DispositionSupported},
		{ID: "a11y-3.3.4", Checkpoint: "3.3.4", Level: "AA", Principle: "Understandable", Criterion: "Error Prevention (Legal, Financial, Data)",
			Assessment:  "Financial and legal actions are reversible or require review: quotes are deterministic, billing requires approval, and submissions require human review.",
			Disposition: DispositionSupported},
		{ID: "a11y-3.3.7", Checkpoint: "3.3.7", Level: "A", Principle: "Understandable", Criterion: "Redundant Entry",
			Assessment:  "API operations accept stable identifiers and idempotency keys so clients can avoid re-entering information; server-side resolution reuses existing context.",
			Disposition: DispositionSupported},
		{ID: "a11y-3.3.8", Checkpoint: "3.3.8", Level: "AA", Principle: "Understandable", Criterion: "Accessible Authentication (Minimum)",
			Assessment:  "Authentication relies on tokens and session creation rather than cognitive tasks such as transcribing images; no memory-based puzzle is required.",
			Disposition: DispositionSupported},

		// Principle 4: Robust
		{ID: "a11y-4.1.2", Checkpoint: "4.1.2", Level: "A", Principle: "Robust", Criterion: "Name, Role, Value",
			Assessment:  "API resources are structured JSON with stable names, roles and values that assistive technologies can map to native semantics.",
			Disposition: DispositionSupported},
		{ID: "a11y-4.1.3", Checkpoint: "4.1.3", Level: "AA", Principle: "Robust", Criterion: "Status Messages",
			Assessment:  "Status is conveyed through structured fields and HTTP status codes, enabling accessible status announcements in the client.",
			Disposition: DispositionSupported},
	}

	return items
}

// CriticalTemplateKeys are the owner and worker critical-flow template keys
// that must be usable in English and Hindi (NFR-011).
var CriticalTemplateKeys = []string{
	"access_code_disclosure",
	"incident_alert",
	"stay_confirmation",
}

// LocalizationItem records whether a critical template key is available in a
// supported language.
type LocalizationItem struct {
	TemplateKey string      `json:"template_key"`
	Language    string      `json:"language"`
	Available   bool        `json:"available"`
	Disposition Disposition `json:"disposition"`
}

// VerifyLocalization proves the live API can create and resolve critical
// templates in English and Hindi and rejects unsupported languages. It creates
// an owner session and one template per critical key with en and hi versions.
func VerifyLocalization(ctx context.Context, baseURL, tenantID string, keys []string) ([]LocalizationItem, error) {
	authHeader, err := createOwnerSession(ctx, baseURL, tenantID)
	if err != nil {
		return nil, err
	}

	items := []LocalizationItem{}
	for _, key := range keys {
		if _, err := apiCall(ctx, baseURL, http.MethodPost, "/v1/communications/templates",
			map[string]any{
				"template_key":  key,
				"audience":      "owner",
				"consent_class": "transactional",
				"channel":       "push",
				"severity":      "normal",
			}, authHeader); err != nil {
			return nil, fmt.Errorf("create template %s: %w", key, err)
		}

		for _, lang := range []string{"en", "hi"} {
			if _, err := apiCall(ctx, baseURL, http.MethodPost,
				"/v1/communications/templates/"+key+"/versions",
				map[string]any{
					"language": lang,
					"subject":  templateSubject(key, lang),
					"body":     templateBody(key, lang),
				}, authHeader); err != nil {
				return nil, fmt.Errorf("add %s version for %s: %w", lang, key, err)
			}
		}

		for _, lang := range []string{"en", "hi"} {
			resolveBody, err := apiCall(ctx, baseURL, http.MethodGet,
				"/v1/communications/templates/"+key+"/resolve?language="+lang, nil, authHeader)
			if err != nil {
				return nil, fmt.Errorf("resolve %s in %s: %w", key, lang, err)
			}

			var resolved struct {
				Data struct {
					Language string `json:"language"`
					Body     string `json:"body"`
				} `json:"data"`
			}
			if err := json.Unmarshal([]byte(resolveBody), &resolved); err != nil {
				return nil, fmt.Errorf("parse resolve %s in %s: %w", key, lang, err)
			}

			available := resolved.Data.Language == lang && resolved.Data.Body != ""
			disposition := DispositionSupported
			if !available {
				disposition = DispositionGap
			}
			items = append(items, LocalizationItem{
				TemplateKey: key,
				Language:    lang,
				Available:   available,
				Disposition: disposition,
			})
		}
	}

	// Unsupported languages must be rejected so a client cannot silently
	// degrade critical content.
	if _, err := apiCallExpectStatus(ctx, baseURL, http.MethodGet,
		"/v1/communications/templates/"+keys[0]+"/resolve?language=fr", nil, authHeader,
		http.StatusUnprocessableEntity); err != nil {
		return nil, fmt.Errorf("unsupported language must be rejected: %w", err)
	}

	return items, nil
}

func templateSubject(key, lang string) string {
	if lang == "hi" {
		switch key {
		case "access_code_disclosure":
			return "आपका एक्सेस कोड"
		case "incident_alert":
			return "घटना सूचना"
		default:
			return "स्टे पुष्टि"
		}
	}
	switch key {
	case "access_code_disclosure":
		return "Your access code"
	case "incident_alert":
		return "Incident alert"
	default:
		return "Stay confirmation"
	}
}

func templateBody(key, lang string) string {
	if lang == "hi" {
		switch key {
		case "access_code_disclosure":
			return "प्रवेश के लिए आपका एक्सेस कोड उपलब्ध है।"
		case "incident_alert":
			return "आपकी संपत्ति पर एक घटना दर्ज की गई है।"
		default:
			return "आपका स्टे पुष्ट हो गया है।"
		}
	}
	switch key {
	case "access_code_disclosure":
		return "Your access code for the stay is available."
	case "incident_alert":
		return "An incident has been recorded at your property."
	default:
		return "Your stay is confirmed."
	}
}

func apiCall(ctx context.Context, baseURL, method, path string, body any, authHeader string) (string, error) {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return "", err
		}
		reader = strings.NewReader(string(payload))
	}

	url := strings.TrimRight(baseURL, "/") + path
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return "", err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("%s %s: expected 2xx, got %d: %s", method, path, resp.StatusCode, string(respBody))
	}
	return string(respBody), nil
}

func apiCallExpectStatus(ctx context.Context, baseURL, method, path string, body any, authHeader string, expected int) (string, error) {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return "", err
		}
		reader = strings.NewReader(string(payload))
	}

	url := strings.TrimRight(baseURL, "/") + path
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return "", err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != expected {
		return "", fmt.Errorf("%s %s: expected %d, got %d: %s", method, path, expected, resp.StatusCode, string(respBody))
	}
	return string(respBody), nil
}
