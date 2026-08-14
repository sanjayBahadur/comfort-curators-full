package prompt

import _ "embed"

//go:embed v1.md
var v1 string

// V1 returns the governed Superhost system prompt.
func V1() string {
	return v1
}
