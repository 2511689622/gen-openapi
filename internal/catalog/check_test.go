package catalog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckCatalogEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.yaml")
	writeCatalog(t, path, `apiVersion: infra.example.com/v1
kind: ApiCatalog
spec:
  services:
    - name: demo
      repo: https://github.com/example/demo
      contract: api/api-contract.yaml
      apigConfig: api/apig-config.yaml
      apigYaml: api/openAPI.yaml
`)

	err := Check(CheckOptions{
		CatalogPath: path,
		Service:     "demo",
		Repo:        "https://github.com/example/demo",
		Contract:    "api/api-contract.yaml",
		ApigConfig:  "api/apig-config.yaml",
		OpenAPI:     "api/openAPI.yaml",
	})
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestLoadServiceIncludesImportTarget(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.yaml")
	writeCatalog(t, path, `apiVersion: infra.example.com/v1
kind: ApiCatalog
spec:
  services:
    - name: demo
      repo: https://github.com/example/demo
      contract: apig/api-contract.yaml
      apigConfig: apig/apig-config.yaml
      apigYaml: apig/openAPI.yaml
      importTarget:
        region: cn-north-4
        projectId: project-id
        instanceId: instance-id
        groupId: group-id
        apiMode: merge
        extendMode: merge
`)

	svc, err := LoadService(path, "demo")
	if err != nil {
		t.Fatalf("LoadService: %v", err)
	}
	if svc.ImportTarget == nil {
		t.Fatal("expected importTarget")
	}
	if svc.ImportTarget.ProjectID != "project-id" {
		t.Fatalf("projectId = %q", svc.ImportTarget.ProjectID)
	}
}

func TestCheckCatalogMissingService(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.yaml")
	writeCatalog(t, path, `apiVersion: infra.example.com/v1
kind: ApiCatalog
spec:
  services: []
`)
	if err := Check(CheckOptions{CatalogPath: path, Service: "demo"}); err == nil {
		t.Fatal("expected missing service error")
	}
}

func TestCheckCatalogMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.yaml")
	writeCatalog(t, path, `apiVersion: infra.example.com/v1
kind: ApiCatalog
spec:
  services:
    - name: demo
      repo: https://github.com/example/demo
      contract: api/api-contract.yaml
      apigConfig: api/apig-config.yaml
      apigYaml: api/openAPI.yaml
`)
	if err := Check(CheckOptions{CatalogPath: path, Service: "demo", OpenAPI: "api/wrong.yaml"}); err == nil {
		t.Fatal("expected mismatch error")
	}
}

func writeCatalog(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
