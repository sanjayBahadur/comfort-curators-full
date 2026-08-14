package security

import (
	"context"
	"regexp"
	"strings"
)

type InjectionSeverity string

const (
	InjectionNone    InjectionSeverity = "none"
	InjectionSuspect InjectionSeverity = "suspect"
	InjectionBlocked InjectionSeverity = "blocked"
)

type InjectionResult struct {
	Severity  InjectionSeverity
	Patterns  []string
	Sanitized string
	Blocked   bool
}

type PromptInjectionDetector struct {
	blockedPatterns []*regexp.Regexp
	suspectPatterns []*regexp.Regexp
}

func NewPromptInjectionDetector() *PromptInjectionDetector {
	return &PromptInjectionDetector{
		blockedPatterns: compilePatterns(blockedInjectionPatterns),
		suspectPatterns: compilePatterns(suspectInjectionPatterns),
	}
}

func (d *PromptInjectionDetector) Scan(ctx context.Context, input string) InjectionResult {
	result := InjectionResult{Severity: InjectionNone, Sanitized: input}

	for _, p := range d.blockedPatterns {
		if p.MatchString(strings.ToLower(input)) {
			result.Patterns = append(result.Patterns, p.String())
			result.Severity = InjectionBlocked
			result.Blocked = true
		}
	}

	if !result.Blocked {
		for _, p := range d.suspectPatterns {
			if p.MatchString(strings.ToLower(input)) {
				result.Patterns = append(result.Patterns, p.String())
				result.Severity = InjectionSuspect
			}
		}
	}

	return result
}

func compilePatterns(raw []string) []*regexp.Regexp {
	var result []*regexp.Regexp
	for _, r := range raw {
		re, err := regexp.Compile(r)
		if err != nil {
			continue
		}
		result = append(result, re)
	}
	return result
}

var blockedInjectionPatterns = []string{
	`(?i)ignore\s+.*(instructions|prompts|system|rules|guidelines)`,
	`(?i)you\s+are\s+(?:now\s+)?(?:a\s+)?different\s+(role|persona|assistant|identity)`,
	`(?i)forget\s+(all|your|previous)\s+(instructions|training|rules|context)`,
	`(?i)system\s*prompt\s*:`,
	`\[system\].*\[/system\]`,
	`(?i)override\s+.*(rules|policy|instructions|safety)`,
	`(?i)pretend\s+(you|to\s+be|you['\s]*re)\s+`,
	`(?i)new\s+instructions?\s*:`,
	`(?i)your\s+new\s+goal\s+(is|:)`,
}

var suspectInjectionPatterns = []string{
	`delete\s+(from|all|every)\s+(table|record|database)`,
	`drop\s+table`,
	`insert\s+into\s+`,
	`union\s+select`,
	`;\s*--`,
	`1\s*=\s*1`,
	`<script`,
	`eval\s*\(`,
	`document\.cookie`,
	`\$\(.*\)`,
	`process\.env`,
}
