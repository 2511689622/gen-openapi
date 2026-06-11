package apply

import (
	"path/filepath"
	"runtime"
	"testing"

	"gen-openapi/internal/apig"
	"gen-openapi/internal/config"
	"gen-openapi/internal/diff"
	"gen-openapi/pkg/contract"
)

func TestWorkflow_Examples_DiffApplyRenderValidate(t *testing.T) {
	root := repoRoot(t)
	canonicalPath := filepath.Join(root, "internal/testdata/software-package-server/api-contract.yaml")
	detectedPath := filepath.Join(root, "internal/testdata/software-package-server/api-contract.detected.yaml")
	apigPath := filepath.Join(root, "internal/testdata/software-package-server/apig-config.yaml")

	canonical, err := config.LoadContract(canonicalPath)
	if err != nil {
		t.Fatalf("load canonical: %v", err)
	}
	detected, err := config.LoadContractLoose(detectedPath)
	if err != nil {
		t.Fatalf("load detected: %v", err)
	}
	apigCfg, err := config.LoadApigConfig(apigPath)
	if err != nil {
		t.Fatalf("load apig config: %v", err)
	}

	report := diff.Compare(canonical, detected)
	if report.IsEmpty() {
		t.Fatal("expected example canonical and detected contracts to differ")
	}
	if len(report.Changed) == 0 {
		t.Fatalf("expected changed routes in example diff, got %+v", report)
	}

	merged, mergeReport, err := Merge(canonical, detected, Options{})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if err := config.ValidateContract(merged); err != nil {
		t.Fatalf("merged contract should validate: %v", err)
	}
	if len(mergeReport.Merged) == 0 {
		t.Fatalf("expected merge report to include changed routes, got %+v", mergeReport)
	}

	assertRouteAuth(t, merged, "GET", "/v1/cla", "app")
	assertRouteAuth(t, merged, "POST", "/v1/softwarepkg", "app")
	assertRequestBodySchema(t, merged, "POST", "/v1/softwarepkg", "SoftwarePkgRequest")
	assertPathParam(t, merged, "PUT", "/v1/softwarepkg/{id}", "id")

	doc, err := apig.Render(merged, apigCfg)
	if err != nil {
		t.Fatalf("render merged contract: %v", err)
	}
	if err := apig.Validate(doc); err != nil {
		t.Fatalf("validate rendered OpenAPI: %v", err)
	}
}

func assertRouteAuth(t *testing.T, c *contract.ApiContract, method, path, want string) {
	t.Helper()
	r := findRoute(c.Spec.Routes, method, path)
	if r == nil {
		t.Fatalf("route %s %s not found", method, path)
	}
	if r.Auth != want {
		t.Fatalf("route %s %s auth: want %q, got %q", method, path, want, r.Auth)
	}
}

func assertRequestBodySchema(t *testing.T, c *contract.ApiContract, method, path, want string) {
	t.Helper()
	r := findRoute(c.Spec.Routes, method, path)
	if r == nil {
		t.Fatalf("route %s %s not found", method, path)
	}
	if r.RequestBody == nil {
		t.Fatalf("route %s %s missing requestBody", method, path)
	}
	if r.RequestBody.Schema != want {
		t.Fatalf("route %s %s requestBody.schema: want %q, got %q", method, path, want, r.RequestBody.Schema)
	}
}

func assertPathParam(t *testing.T, c *contract.ApiContract, method, path, name string) {
	t.Helper()
	r := findRoute(c.Spec.Routes, method, path)
	if r == nil {
		t.Fatalf("route %s %s not found", method, path)
	}
	for _, p := range r.Parameters {
		if p.Name == name && p.In == "path" && p.Required {
			return
		}
	}
	t.Fatalf("route %s %s missing required path parameter %q; params=%+v", method, path, name, r.Parameters)
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
}
