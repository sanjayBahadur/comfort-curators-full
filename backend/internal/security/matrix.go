package security

import (
	"context"
	"fmt"
)

type MatrixTest struct {
	SubjectRoles  []string
	Action        string
	ResourceType  string
	ShouldAllow   bool
	Description   string
	TenantScope   string
	PropertyScope string
}

type MatrixReport struct {
	TotalTests  int
	PassedTests int
	FailedTests int
	Failures    []MatrixFailure
}

type MatrixFailure struct {
	Test     MatrixTest
	Actual   string
	Expected string
}

func (r *MatrixReport) HasFailures() bool {
	return len(r.Failures) > 0
}

func (r *MatrixReport) Summary() string {
	return fmt.Sprintf("%d/%d passed, %d failures", r.PassedTests, r.TotalTests, r.FailedTests)
}

type AuthorizationMatrix struct {
	tests []MatrixTest
}

func NewAuthorizationMatrix() *AuthorizationMatrix {
	return &AuthorizationMatrix{
		tests: definedMatrixTests(),
	}
}

func (m *AuthorizationMatrix) Tests() []MatrixTest {
	return append([]MatrixTest(nil), m.tests...)
}

func definedMatrixTests() []MatrixTest {
	return []MatrixTest{
		{SubjectRoles: []string{"owner"}, Action: "read", ResourceType: "property", ShouldAllow: true, Description: "Owner can read own property", TenantScope: "own", PropertyScope: "own"},
		{SubjectRoles: []string{"owner"}, Action: "write", ResourceType: "property", ShouldAllow: true, Description: "Owner can write own property", TenantScope: "own", PropertyScope: "own"},
		{SubjectRoles: []string{"owner"}, Action: "read", ResourceType: "property", ShouldAllow: false, Description: "Owner cannot read cross-tenant property", TenantScope: "other", PropertyScope: "other"},
		{SubjectRoles: []string{"owner"}, Action: "write", ResourceType: "property", ShouldAllow: false, Description: "Owner cannot write cross-tenant property", TenantScope: "other", PropertyScope: "other"},
		{SubjectRoles: []string{"owner"}, Action: "read", ResourceType: "document", ShouldAllow: true, Description: "Owner can read own document", TenantScope: "own"},
		{SubjectRoles: []string{"owner"}, Action: "read", ResourceType: "document", ShouldAllow: false, Description: "Owner cannot read cross-tenant document", TenantScope: "other"},
		{SubjectRoles: []string{"worker"}, Action: "read", ResourceType: "ticket", ShouldAllow: true, Description: "Worker can read assigned ticket", TenantScope: "own"},
		{SubjectRoles: []string{"worker"}, Action: "read", ResourceType: "property_access", ShouldAllow: true, Description: "Worker can read assigned property access"},
		{SubjectRoles: []string{"worker"}, Action: "read", ResourceType: "property", ShouldAllow: false, Description: "Worker cannot read unrestricted property details"},
		{SubjectRoles: []string{"admin"}, Action: "read", ResourceType: "property", ShouldAllow: true, Description: "Admin can read all properties", TenantScope: "any"},
		{SubjectRoles: []string{"admin"}, Action: "write", ResourceType: "property", ShouldAllow: false, Description: "Admin cannot write properties without support grant"},
		{SubjectRoles: []string{"admin"}, Action: "delete", ResourceType: "property", ShouldAllow: false, Description: "Admin cannot delete properties"},
		{SubjectRoles: []string{"viewer"}, Action: "read", ResourceType: "document", ShouldAllow: true, Description: "Viewer can read documents"},
		{SubjectRoles: []string{"viewer"}, Action: "write", ResourceType: "document", ShouldAllow: false, Description: "Viewer cannot write documents"},
		{SubjectRoles: []string{}, Action: "read", ResourceType: "property", ShouldAllow: false, Description: "Unauthenticated cannot read property"},
		{SubjectRoles: []string{}, Action: "read", ResourceType: "document", ShouldAllow: false, Description: "Unauthenticated cannot read document"},
		{SubjectRoles: []string{"owner"}, Action: "read", ResourceType: "billing", ShouldAllow: true, Description: "Owner can read own billing", TenantScope: "own"},
		{SubjectRoles: []string{"owner"}, Action: "read", ResourceType: "billing", ShouldAllow: false, Description: "Owner cannot read cross-tenant billing", TenantScope: "other"},
		{SubjectRoles: []string{"worker"}, Action: "read", ResourceType: "billing", ShouldAllow: false, Description: "Worker cannot read billing"},
		{SubjectRoles: []string{"admin"}, Action: "read", ResourceType: "audit_log", ShouldAllow: true, Description: "Admin can read audit logs"},
		{SubjectRoles: []string{"owner"}, Action: "read", ResourceType: "audit_log", ShouldAllow: false, Description: "Owner cannot read audit logs"},
		{SubjectRoles: []string{"worker"}, Action: "read", ResourceType: "audit_log", ShouldAllow: false, Description: "Worker cannot read audit logs"},
	}
}

func RunMatrixTests(ctx context.Context, matrix *AuthorizationMatrix, authChecker func(ctx context.Context, roles []string, tenantScope, action, resourceType string) error) *MatrixReport {
	report := &MatrixReport{}
	for _, test := range matrix.tests {
		report.TotalTests++
		var err error
		if authChecker != nil {
			err = authChecker(ctx, test.SubjectRoles, test.TenantScope, test.Action, test.ResourceType)
		}
		allowed := err == nil

		if allowed == test.ShouldAllow {
			report.PassedTests++
			continue
		}

		report.FailedTests++
		failure := MatrixFailure{Test: test}
		if allowed {
			failure.Actual = "allowed"
		} else {
			failure.Actual = fmt.Sprintf("denied: %v", err)
		}
		if test.ShouldAllow {
			failure.Expected = "allowed"
		} else {
			failure.Expected = "denied"
		}
		report.Failures = append(report.Failures, failure)
	}
	return report
}
