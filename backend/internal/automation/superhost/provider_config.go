package superhost

import "os"

// defaultSuperhostProvider and defaultSuperhostModel are the (provider,
// model) pair stamped onto a Superhost run when the request doesn't specify
// one -- which is every request today, since the frontend never sends
// provider/model. They default to the deterministic no-op stub so a plain
// checkout with no secrets configured still runs end to end; set
// CC_SUPERHOST_PROVIDER/CC_SUPERHOST_MODEL (see compose.yaml, worker's
// CC_MODEL_URL/CC_MODEL_API_KEY) to switch a real, billed model on instead.
func defaultSuperhostProvider() string {
	if v := os.Getenv("CC_SUPERHOST_PROVIDER"); v != "" {
		return v
	}
	return "model-stub"
}

func defaultSuperhostModel() string {
	if v := os.Getenv("CC_SUPERHOST_MODEL"); v != "" {
		return v
	}
	return "stub-v1"
}
