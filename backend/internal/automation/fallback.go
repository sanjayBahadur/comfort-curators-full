package automation

import (
	"encoding/json"
	"strings"
	"time"
)

const (
	providerCallTimeout = 20 * time.Second
	fallbackMarker      = "OFFLINE FALLBACK"
)

const (
	fallbackIntentTurnover = "turnover_ticket"
	fallbackIntentStatus   = "property_status"
	fallbackIntentGeneral  = "general"
)

type cannedFallback struct {
	Intent  string
	Message string
}

var cannedFallbacks = map[string]cannedFallback{
	fallbackIntentTurnover: {
		Intent:  fallbackIntentTurnover,
		Message: "I could not reach the live model. I cannot create or approve a turnover ticket offline; please review the property queue manually.",
	},
	fallbackIntentStatus: {
		Intent:  fallbackIntentStatus,
		Message: "I could not reach the live model. A live property status read is unavailable offline; please check the property record directly.",
	},
	fallbackIntentGeneral: {
		Intent:  fallbackIntentGeneral,
		Message: "I could not reach the live model. This is a canned offline response; please retry when the model service is available.",
	},
}

type fallbackOutput struct {
	Message        string `json:"message"`
	IsFallback     bool   `json:"is_fallback"`
	FallbackMarker string `json:"fallback_marker"`
	Intent         string `json:"intent"`
}

func chooseFallback(run *AgentRun) cannedFallback {
	return cannedFallbacks[fallbackIntent(run.InputData)]
}

func fallbackIntent(input json.RawMessage) string {
	var fields map[string]any
	if json.Unmarshal(input, &fields) == nil {
		for _, key := range []string{"intent", "task", "action"} {
			if value, ok := fields[key].(string); ok {
				if intent := normalizeFallbackIntent(value); intent != "" {
					return intent
				}
			}
		}
	}

	lower := strings.ToLower(string(input))
	switch {
	case strings.Contains(lower, "turnover") && strings.Contains(lower, "ticket"):
		return fallbackIntentTurnover
	case strings.Contains(lower, "property") && strings.Contains(lower, "status"):
		return fallbackIntentStatus
	default:
		return fallbackIntentGeneral
	}
}

func normalizeFallbackIntent(value string) string {
	lower := strings.ToLower(strings.TrimSpace(value))
	switch {
	case strings.Contains(lower, "turnover") && strings.Contains(lower, "ticket"), lower == "turnover":
		return fallbackIntentTurnover
	case strings.Contains(lower, "property") && strings.Contains(lower, "status"), lower == "status":
		return fallbackIntentStatus
	default:
		return ""
	}
}
