package prompt_test

import (
	"strings"
	"testing"

	"comfort-curators-backend/internal/automation/superhost"
	"comfort-curators-backend/internal/automation/superhost/prompt"
)

func TestV1ContainsEveryAllowedTool(t *testing.T) {
	text := prompt.V1()
	if strings.TrimSpace(text) == "" {
		t.Fatal("V1 returned empty prompt")
	}

	for _, name := range superhost.AllowedToolNames() {
		if !strings.Contains(text, "`"+name+"`") {
			t.Errorf("V1 does not list allowed tool %q", name)
		}
	}
}
