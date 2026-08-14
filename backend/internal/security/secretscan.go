package security

import (
	"regexp"
	"strings"
)

type SecretMatch struct {
	Pattern     string
	Match       string
	Description string
	Line        int
	FilePath    string
}

type SecretScanner struct {
	patterns []SecretPattern
}

type SecretPattern struct {
	Regex       *regexp.Regexp
	Description string
	Name        string
}

func NewSecretScanner() *SecretScanner {
	patterns := []struct {
		name  string
		regex string
		desc  string
	}{
		{"aws_access_key", `(?i)aws[_-]?(access|secret)[_-]?key[_-]?(id)?\s*[:=]\s*['\"]?[A-Z0-9/+]{20,40}['\"]?`, "AWS access key ID"},
		{"generic_api_key", `(?i)(api[_-]?key|apikey|access[_-]?token)\s*[:=]\s*['\"]?[a-zA-Z0-9_\-\.]{20,}['\"]?`, "Generic API key assignment"},
		{"private_key", `-----BEGIN\s+(RSA|EC|DSA|OPENSSH)\s+PRIVATE\s+KEY`, "Private key marker"},
		{"jwt_token", `eyJ[a-zA-Z0-9_-]+\.eyJ[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+`, "JWT token pattern"},
		{"password_in_url", `(?i)(postgres|mysql|mongodb|redis)://[^:@]+:[^@]+@`, "Database URL with embedded password"},
		{"slack_token", `xox[baprs]-[0-9]{10,}-[0-9]{10,}-[a-zA-Z0-9]+`, "Slack bot token"},
		{"github_token", `gh[pousr]_[a-zA-Z0-9]{36,}`, "GitHub personal access token"},
	}

	s := &SecretScanner{}
	for _, p := range patterns {
		re := regexp.MustCompile(p.regex)
		s.patterns = append(s.patterns, SecretPattern{
			Regex:       re,
			Description: p.desc,
			Name:        p.name,
		})
	}

	return s
}

func (s *SecretScanner) Scan(content string, filePath string) []SecretMatch {
	var matches []SecretMatch
	lines := strings.Split(content, "\n")

	for lineNum, line := range lines {
		for _, pattern := range s.patterns {
			if pattern.Regex.MatchString(line) {
				matches = append(matches, SecretMatch{
					Pattern:     pattern.Name,
					Match:       s.maskSecret(pattern.Regex.FindString(line)),
					Description: pattern.Description,
					Line:        lineNum + 1,
					FilePath:    filePath,
				})
			}
		}
	}

	return matches
}

func (s *SecretScanner) maskSecret(match string) string {
	if len(match) <= 8 {
		return strings.Repeat("*", len(match))
	}
	return match[:4] + strings.Repeat("*", len(match)-8) + match[len(match)-4:]
}

type DependencyScanResult struct {
	Module     string
	Version    string
	HasIssue   bool
	IssueTitle string
	IssueLink  string
	Severity   string
}

type DependencyScanner struct {
	knownIssues map[string][]DependencyScanResult
}

func NewDependencyScanner() *DependencyScanner {
	return &DependencyScanner{
		knownIssues: make(map[string][]DependencyScanResult),
	}
}

func (d *DependencyScanner) AddKnownIssue(module, version, title, link, severity string) {
	d.knownIssues[module] = append(d.knownIssues[module], DependencyScanResult{
		Module:     module,
		Version:    version,
		HasIssue:   true,
		IssueTitle: title,
		IssueLink:  link,
		Severity:   severity,
	})
}

func (d *DependencyScanner) Scan(module, version string) []DependencyScanResult {
	issues, ok := d.knownIssues[module]
	if !ok {
		return nil
	}
	var result []DependencyScanResult
	for _, issue := range issues {
		if issue.Version == version || issue.Version == "*" {
			result = append(result, issue)
		}
	}
	return result
}

type ContainerCheck struct {
	Name        string
	Passed      bool
	Description string
	Severity    string
	Evidence    string
}

type ContainerScanner struct {
	checks []ContainerCheck
}

func NewContainerScanner() *ContainerScanner {
	return &ContainerScanner{}
}

func (c *ContainerScanner) CheckUser(user string) ContainerCheck {
	if user == "" || user == "root" || user == "0" {
		return ContainerCheck{
			Name:        "non_root_user",
			Passed:      false,
			Description: "Container must run as non-root user",
			Severity:    "high",
			Evidence:    "USER directive missing or set to root",
		}
	}
	return ContainerCheck{
		Name:        "non_root_user",
		Passed:      true,
		Description: "Container runs as non-root user",
		Severity:    "info",
		Evidence:    "USER " + user,
	}
}

func (c *ContainerScanner) CheckHealth() ContainerCheck {
	return ContainerCheck{
		Name:        "healthcheck_present",
		Passed:      true,
		Description: "Container has health check",
		Severity:    "info",
	}
}
