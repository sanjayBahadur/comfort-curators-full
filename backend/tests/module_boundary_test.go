package tests

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

type listedPackage struct {
	ImportPath   string
	Imports      []string
	TestImports  []string
	XTestImports []string
}

func TestModuleBoundaryFixture(t *testing.T) {
	cmd := exec.Command("go", "list", "-json", "./...")
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Fatalf("go list failed: %s", string(exitErr.Stderr))
		}
		t.Fatalf("go list failed: %v", err)
	}

	dec := json.NewDecoder(strings.NewReader(string(out)))
	for dec.More() {
		var pkg listedPackage
		if err := dec.Decode(&pkg); err != nil {
			t.Fatalf("decode go list package: %v", err)
		}
		checkImports(t, pkg.ImportPath, pkg.Imports)
		checkImports(t, pkg.ImportPath, pkg.TestImports)
		checkImports(t, pkg.ImportPath, pkg.XTestImports)
	}
}

func checkImports(t *testing.T, importPath string, imports []string) {
	t.Helper()

	for _, imported := range imports {
		if strings.HasPrefix(importPath, "comfort-curators-backend/internal/platform") &&
			strings.HasPrefix(imported, "comfort-curators-backend/internal/modules") {
			t.Fatalf("platform package %s must not import business module %s", importPath, imported)
		}

		if strings.HasPrefix(importPath, "comfort-curators-backend/internal/modules") &&
			strings.HasPrefix(imported, "comfort-curators-backend/cmd/") {
			t.Fatalf("business module %s must not import command package %s", importPath, imported)
		}
	}
}
