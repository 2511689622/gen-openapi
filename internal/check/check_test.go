package check

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gen-openapi/internal/apig"
	"gen-openapi/internal/config"
	"gen-openapi/internal/output"
)

func TestRun_UpToDate(t *testing.T) {
	root := repoRoot(t)
	contractPath := filepath.Join(root, "internal/testdata/software-package-server/api-contract.yaml")
	apigPath := filepath.Join(root, "internal/testdata/software-package-server/apig-config.yaml")
	outPath := renderExampleOpenAPI(t, contractPath, apigPath)

	res, err := Run(contractPath, apigPath, outPath)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.UpToDate {
		t.Fatalf("expected up-to-date result, diff:\n%s", res.Diff)
	}
	if res.Diff != "" {
		t.Fatalf("expected empty diff, got %q", res.Diff)
	}
}

func TestRun_Stale(t *testing.T) {
	root := repoRoot(t)
	contractPath := filepath.Join(root, "internal/testdata/software-package-server/api-contract.yaml")
	apigPath := filepath.Join(root, "internal/testdata/software-package-server/apig-config.yaml")
	outPath := renderExampleOpenAPI(t, contractPath, apigPath)

	b, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read rendered output: %v", err)
	}
	b = []byte(strings.Replace(string(b), "software-package-server", "stale-title", 1))
	if err := os.WriteFile(outPath, b, 0o644); err != nil {
		t.Fatalf("write stale output: %v", err)
	}

	res, err := Run(contractPath, apigPath, outPath)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.UpToDate {
		t.Fatal("expected stale output")
	}
	if !strings.Contains(res.Diff, "line") || !strings.Contains(res.Diff, "have:") || !strings.Contains(res.Diff, "want:") {
		t.Fatalf("expected short diff, got:\n%s", res.Diff)
	}
}

func TestRun_MissingOutput(t *testing.T) {
	root := repoRoot(t)
	contractPath := filepath.Join(root, "internal/testdata/software-package-server/api-contract.yaml")
	apigPath := filepath.Join(root, "internal/testdata/software-package-server/apig-config.yaml")
	outPath := filepath.Join(t.TempDir(), "missing-openAPI.yaml")

	res, err := Run(contractPath, apigPath, outPath)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.UpToDate {
		t.Fatal("expected missing output to be stale")
	}
	if !strings.Contains(res.Diff, "does not exist") {
		t.Fatalf("expected missing-file diff, got %q", res.Diff)
	}
}

func renderExampleOpenAPI(t *testing.T, contractPath, apigPath string) string {
	t.Helper()
	contractDoc, err := config.LoadContract(contractPath)
	if err != nil {
		t.Fatalf("load contract: %v", err)
	}
	apigCfg, err := config.LoadApigConfig(apigPath)
	if err != nil {
		t.Fatalf("load apig config: %v", err)
	}
	doc, err := apig.Render(contractDoc, apigCfg)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if err := apig.Validate(doc); err != nil {
		t.Fatalf("validate rendered OpenAPI: %v", err)
	}
	outPath := filepath.Join(t.TempDir(), "openAPI.yaml")
	if err := output.WriteYAML(outPath, doc); err != nil {
		t.Fatalf("write rendered output: %v", err)
	}
	return outPath
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
}
