package api_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"comfort-curators-backend/internal/api"
)

func TestConformanceContractLintsClean(t *testing.T) {
	spec := loadSpec(t)
	issues := spec.Lint()
	if len(issues) > 0 {
		t.Fatalf("contract must lint clean, got %d issues: %v", len(issues), issues)
	}
}

func TestConformanceLintDetectsStructuralIssues(t *testing.T) {
	broken := `openapi: 3.1.0
info:
  title: Broken Contract
  version: 0.1.0
tags:
  - name: Health
paths:
  nope:
    get:
      operationId: brokenOp
      tags: [UndefinedTag]
      responses:
        '999': {$ref: '#/components/responses/DoesNotExist'}
    post:
      operationId: brokenOp
      security: [{missingScheme: []}]
      responses:
        '200': {$ref: '#/components/responses/DoesNotExist'}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.yaml")
	if err := os.WriteFile(path, []byte(broken), 0o600); err != nil {
		t.Fatalf("write broken contract: %v", err)
	}

	spec, err := api.LoadSpec(path)
	if err != nil {
		t.Fatalf("LoadSpec: %v", err)
	}

	issues := spec.Lint()
	joined := strings.Join(issues, "\n")

	for _, want := range []string{
		`path "nope" must start with /`,
		`tag "UndefinedTag" is not declared`,
		`invalid response code "999"`,
		`reference "#/components/responses/DoesNotExist" does not resolve`,
		`operationId "brokenOp" is duplicated`,
		`security scheme "missingScheme" is referenced but not declared`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("Lint must flag %q, issues: %v", want, issues)
		}
	}
}

func TestConformanceLintRejectsNonLocalRefs(t *testing.T) {
	broken := `openapi: 3.1.0
info:
  title: Broken Contract
  version: 0.1.0
tags:
  - name: Health
paths:
  /v1/health:
    get:
      operationId: getHealth
      tags: [Health]
      security: []
      responses:
        '200':
          $ref: https://example.com/remote.yaml
`
	dir := t.TempDir()
	path := filepath.Join(dir, "remote.yaml")
	if err := os.WriteFile(path, []byte(broken), 0o600); err != nil {
		t.Fatalf("write broken contract: %v", err)
	}

	spec, err := api.LoadSpec(path)
	if err != nil {
		t.Fatalf("LoadSpec: %v", err)
	}

	issues := spec.Lint()
	if len(issues) != 1 || !strings.Contains(issues[0], "non-local reference") {
		t.Fatalf("Lint must reject the non-local reference, got: %v", issues)
	}
}
