package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gen-openapi/internal/config"
	"gen-openapi/pkg/contract"
)

func runCmd(t *testing.T, args ...string) error {
	t.Helper()
	root := NewRootCommand()
	root.SetArgs(args)
	return root.ExecuteContext(context.Background())
}

func TestApigImportLoadTargetFromCatalog(t *testing.T) {
	tmp := t.TempDir()
	catalogPath := filepath.Join(tmp, "catalog.yaml")
	writeFile(t, catalogPath, `apiVersion: infra.example.com/v1
kind: ApiCatalog
spec:
  services:
    - name: smoke
      importTarget:
        region: cn-north-4
        projectId: project-id
        instanceId: instance-id
        groupId: group-id
`)

	target, err := loadImportTarget(&apigImportOptions{catalogPath: catalogPath, serviceName: "smoke"}, &config.HuaweiApigConfig{})
	if err != nil {
		t.Fatalf("loadImportTarget: %v", err)
	}
	if target.ProjectID != "project-id" || target.APIMode != "merge" || target.ExtendMode != "merge" {
		t.Fatalf("unexpected target: %+v", target)
	}
}

func TestApigImportLoadTargetRequiresCatalogAndServiceTogether(t *testing.T) {
	_, err := loadImportTarget(&apigImportOptions{catalogPath: "catalog.yaml"}, &config.HuaweiApigConfig{})
	if err == nil || !strings.Contains(err.Error(), "must be provided together") {
		t.Fatalf("expected paired flag error, got %v", err)
	}
}

func TestApigImportLoadTargetRequiresConfigTargetWithoutCatalog(t *testing.T) {
	_, err := loadImportTarget(&apigImportOptions{}, &config.HuaweiApigConfig{})
	if err == nil || !strings.Contains(err.Error(), "--catalog/--service") {
		t.Fatalf("expected missing config target error, got %v", err)
	}
}

func TestCommandSmoke_InitRenderValidateCheck(t *testing.T) {
	tmp := t.TempDir()
	contractPath := filepath.Join(tmp, "api-contract.yaml")
	apigPath := filepath.Join(tmp, "apig-config.yaml")
	openAPIPath := filepath.Join(tmp, "openAPI.yaml")

	if err := runCmd(t, "init", "--service", "smoke", "--out", tmp); err != nil {
		t.Fatalf("init: %v", err)
	}
	assertFileExists(t, contractPath)
	assertFileExists(t, apigPath)
	assertFileContains(t, contractPath, "schemas: {}")

	if err := runCmd(t, "render", "--contract", contractPath, "--apig-config", apigPath, "--out", openAPIPath); err != nil {
		t.Fatalf("render: %v", err)
	}
	assertNonEmptyFile(t, openAPIPath)

	if err := runCmd(t, "validate", openAPIPath); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if err := runCmd(t, "check", "--contract", contractPath, "--apig-config", apigPath, "--output", openAPIPath); err != nil {
		t.Fatalf("check: %v", err)
	}
}

func TestCommandSmoke_ApigImportDryRunRendersOpenAPI(t *testing.T) {
	tmp := t.TempDir()
	contractPath := filepath.Join(tmp, "api-contract.yaml")
	apigPath := filepath.Join(tmp, "apig-config.yaml")
	openAPIPath := filepath.Join(tmp, "openAPI.yaml")
	writeFile(t, contractPath, minimalContractYAML(`    - operationId: listPets
      method: GET
      path: /v1/pets
      auth: none
`))
	writeFile(t, apigPath, `apiVersion: infra.example.com/v1
kind: HuaweiApigConfig
metadata:
  name: smoke-apig
spec:
  gatewayUrl: https://example.apic.cn-north-4.huaweicloudapis.com
  importTarget:
    region: cn-north-4
    projectId: project-1
    instanceId: instance-1
    groupId: group-1
  backend:
    type: HTTP
    scheme: https
    address: backend.example.com
    timeout: 5000
    retryCount: "0"
  defaults:
    cors: false
    sendFgBodyBase64: true
    matchMode: NORMAL
    requestType: public
    securityScheme: apig-auth-app-header
  securitySchemes:
    apig-auth-app-header:
      type: AppSigv1
      in: header
      name: Authorization
      appcodeAuthType: header
`)

	if err := runCmd(t, "apig-import", "--contract", contractPath, "--apig-config", apigPath, "--out", openAPIPath, "--dry-run"); err != nil {
		t.Fatalf("apig-import dry-run: %v", err)
	}
	assertNonEmptyFile(t, openAPIPath)
}

func TestCommandSmoke_ImportOpenAPILocalFile(t *testing.T) {
	tmp := t.TempDir()
	input := filepath.Join(tmp, "openapi.yaml")
	out := filepath.Join(tmp, "openapi.detected.yaml")
	writeFile(t, input, smokeOpenAPIYAML("/v1/pets"))

	if err := runCmd(t, "import-openapi", "--input", input, "--service", "smoke", "--base-path", "/api", "--out", out, "--default-auth", "none"); err != nil {
		t.Fatalf("import-openapi: %v", err)
	}

	c := loadLooseContract(t, out)
	if c.Metadata.Name != "smoke" {
		t.Fatalf("metadata name = %q, want smoke", c.Metadata.Name)
	}
	if c.Spec.BasePath != "/api" {
		t.Fatalf("basePath = %q, want /api", c.Spec.BasePath)
	}
	assertHasRoute(t, c, "GET", "/v1/pets")
	if _, ok := c.Spec.Schemas["Pet"]; !ok {
		t.Fatalf("expected Pet schema to be copied")
	}
}

func TestCommandSmoke_DiscoverGoGin(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	writeGinRoutes(t, src, "/v1/pets", "/v1/pets")
	out := filepath.Join(tmp, "api-contract.detected.yaml")

	if err := runCmd(t, "discover", "--lang", "go", "--framework", "gin", "--source", src, "--service", "smoke", "--out", out); err != nil {
		t.Fatalf("discover: %v", err)
	}

	c := loadLooseContract(t, out)
	assertHasRoute(t, c, "GET", "/v1/pets")
	assertHasRoute(t, c, "POST", "/v1/pets")
	assertLacksRoute(t, c, "GET", "/debug/vars")
}

func TestCommandSmoke_DiscoverStaticRejectsNonGoFallback(t *testing.T) {
	tmp := t.TempDir()
	out := filepath.Join(tmp, "api-contract.detected.yaml")

	err := runCmd(t, "discover", "--lang", "python", "--framework", "fastapi", "--source", tmp, "--service", "smoke", "--out", out)
	if err == nil {
		t.Fatal("expected static discovery to reject non-Go fallback")
	}
	if !strings.Contains(err.Error(), "supports only --lang go --framework gin") {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
		t.Fatalf("expected no output file after rejected static discovery, stat err: %v", statErr)
	}
}

func TestCommandSmoke_DiscoverGoGinGroupedRoutesWithPathParam(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(src, "routes.go"), `package smoke

func register(r *gin.Engine) {
	api := r.Group("/api")
	api.GET("/pets", listPets)
	api.GET("/pets/:id", getPets)
}
`)
	out := filepath.Join(tmp, "api-contract.detected.yaml")

	if err := runCmd(t, "discover", "--lang", "go", "--framework", "gin", "--source", src, "--service", "smoke", "--out", out); err != nil {
		t.Fatalf("discover: %v", err)
	}

	c := loadLooseContract(t, out)
	assertHasRoute(t, c, "GET", "/api/pets")
	assertHasRoute(t, c, "GET", "/api/pets/{id}")

	for _, r := range c.Spec.Routes {
		if r.Path == "/api/pets/{id}" {
			if len(r.Parameters) == 0 {
				t.Fatal("expected path parameters on /api/pets/{id}, got none")
			}
			found := false
			for _, p := range r.Parameters {
				if p.Name == "id" && p.In == "path" && p.Type == "string" && p.Required {
					found = true
				}
			}
			if !found {
				t.Fatalf("expected path param id, got %+v", r.Parameters)
			}
		}
	}
}

func TestCommandSmoke_DiscoverAutoOpenAPIURL(t *testing.T) {
	tmp := t.TempDir()
	out := filepath.Join(tmp, "api-contract.detected.yaml")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openapi.json" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(smokeOpenAPIJSON("/v1/runtime")))
	}))
	defer server.Close()

	if err := runCmd(t, "discover", "--mode", "auto", "--openapi-url", server.URL+"/openapi.json", "--service", "smoke", "--out", out); err != nil {
		t.Fatalf("discover auto openapi-url: %v", err)
	}

	c := loadLooseContract(t, out)
	assertHasRoute(t, c, "GET", "/v1/runtime")
}

func TestCommandSmoke_DiscoverAutoBaseURLProbe(t *testing.T) {
	tmp := t.TempDir()
	out := filepath.Join(tmp, "api-contract.detected.yaml")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openapi.json" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(smokeOpenAPIJSON("/v1/fastapi")))
	}))
	defer server.Close()

	if err := runCmd(t, "discover", "--mode", "auto", "--lang", "python", "--framework", "fastapi", "--base-url", server.URL, "--service", "smoke", "--out", out); err != nil {
		t.Fatalf("discover auto base-url: %v", err)
	}

	c := loadLooseContract(t, out)
	assertHasRoute(t, c, "GET", "/v1/fastapi")
}

func TestCommandSmoke_DiscoverAutoLocalOpenAPI(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	docs := filepath.Join(src, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(docs, "openapi.yaml"), smokeOpenAPIYAML("/v1/local"))
	writeGinRoutes(t, src, "/v1/fallback", "/v1/fallback")
	out := filepath.Join(tmp, "api-contract.detected.yaml")

	if err := runCmd(t, "discover", "--mode", "auto", "--source", src, "--service", "smoke", "--out", out); err != nil {
		t.Fatalf("discover auto local openapi: %v", err)
	}

	c := loadLooseContract(t, out)
	assertHasRoute(t, c, "GET", "/v1/local")
	assertLacksRoute(t, c, "GET", "/v1/fallback")
}

func TestCommandSmoke_DiscoverAutoFallsBackToStatic(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	writeGinRoutes(t, src, "/v1/fallback", "/v1/fallback")
	out := filepath.Join(tmp, "api-contract.detected.yaml")

	if err := runCmd(t, "discover", "--mode", "auto", "--lang", "go", "--framework", "gin", "--source", src, "--service", "smoke", "--out", out); err != nil {
		t.Fatalf("discover auto fallback: %v", err)
	}

	c := loadLooseContract(t, out)
	assertHasRoute(t, c, "GET", "/v1/fallback")
	assertHasRoute(t, c, "POST", "/v1/fallback")
	assertLacksRoute(t, c, "GET", "/debug/vars")
}

func TestCommandSmoke_DiscoverAutoFiltersInternalOpenAPIPaths(t *testing.T) {
	tmp := t.TempDir()
	input := filepath.Join(tmp, "src", "docs", "openapi.yaml")
	out := filepath.Join(tmp, "api-contract.detected.yaml")
	writeFile(t, input, `openapi: 3.0.3
info:
  title: Smoke OpenAPI
  version: 1.0.0
paths:
  /v1/pets:
    get:
      operationId: listPets
  /debug/vars:
    get:
      operationId: debugVars
`)

	if err := runCmd(t, "discover", "--mode", "auto", "--source", filepath.Join(tmp, "src"), "--service", "smoke", "--out", out); err != nil {
		t.Fatalf("discover auto filter internal: %v", err)
	}

	c := loadLooseContract(t, out)
	assertHasRoute(t, c, "GET", "/v1/pets")
	assertLacksRoute(t, c, "GET", "/debug/vars")
}

func TestCommandSmoke_DiscoverAutoOpenAPIURLFailureIsFatal(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	writeGinRoutes(t, src, "/v1/fallback", "/v1/fallback")
	out := filepath.Join(tmp, "api-contract.detected.yaml")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	err := runCmd(t, "discover", "--mode", "auto", "--openapi-url", server.URL+"/openapi.json", "--lang", "go", "--framework", "gin", "--source", src, "--service", "smoke", "--out", out)
	if err == nil {
		t.Fatal("expected explicit --openapi-url failure to be fatal")
	}
	if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
		t.Fatalf("expected no output file after fatal OpenAPI URL failure, stat err: %v", statErr)
	}
}

func TestCommandSmoke_DiffWritesReport(t *testing.T) {
	tmp := t.TempDir()
	canonical := filepath.Join(tmp, "api-contract.yaml")
	detected := filepath.Join(tmp, "api-contract.detected.yaml")
	report := filepath.Join(tmp, "diff.md")
	writeFile(t, canonical, minimalContractYAML(`    - operationId: listPets
      method: GET
      path: /v1/pets
      auth: none
`))
	writeFile(t, detected, minimalContractYAML(`    - operationId: listPets
      method: GET
      path: /v1/pets
      auth: none
    - operationId: createPet
      method: POST
      path: /v1/pets
      auth: none
`))

	if err := runCmd(t, "diff", "--contract", canonical, "--detected", detected, "--out", report); err != nil {
		t.Fatalf("diff: %v", err)
	}
	assertNonEmptyFile(t, report)
	assertFileContains(t, report, "POST /v1/pets")
}

func TestActionsInitWritesWorkflows(t *testing.T) {
	tmp := t.TempDir()
	written, err := runActionsInit(actionsInitOptions{outDir: tmp})
	if err != nil {
		t.Fatalf("runActionsInit: %v", err)
	}
	if len(written) != 3 {
		t.Fatalf("written files = %d, want 3", len(written))
	}
	assertFileContains(t, filepath.Join(tmp, ".github", "workflows", "apig-check.yml"), "gen-openapi catalog-check")
	assertFileContains(t, filepath.Join(tmp, ".github", "workflows", "apig-drift-scan.yml"), "gen-openapi pr")
	assertFileContains(t, filepath.Join(tmp, ".github", "workflows", "apig-deploy.yml"), "gen-openapi apig-import")
}

func TestActionsInitRefusesOverwriteWithoutForce(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, ".github", "workflows", "apig-check.yml")
	writeFile(t, path, "existing")
	if _, err := runActionsInit(actionsInitOptions{outDir: tmp}); err == nil {
		t.Fatal("expected overwrite error")
	}
}

func TestRepoBootstrapWritesDetectedAndAdoptsWhenNoContractExists(t *testing.T) {
	tmp := t.TempDir()
	detected := loadTestContract()
	apiDir, adopted, err := writeBootstrapFiles(tmp, repoBootstrapOptions{apiDir: "apig"}, detected, loadTestApigConfig())
	if err != nil {
		t.Fatalf("writeBootstrapFiles: %v", err)
	}
	if !adopted {
		t.Fatal("expected detected contract to be adopted as api-contract.yaml")
	}
	assertFileExists(t, filepath.Join(apiDir, "api-contract.detected.yaml"))
	assertFileExists(t, filepath.Join(apiDir, "api-contract.yaml"))
	assertFileExists(t, filepath.Join(apiDir, "apig-config.yaml"))
	assertFileExists(t, filepath.Join(apiDir, "openAPI.yaml"))
}

func TestRepoBootstrapDoesNotOverwriteExistingContract(t *testing.T) {
	tmp := t.TempDir()
	apiDir := filepath.Join(tmp, "apig")
	contractPath := filepath.Join(apiDir, "api-contract.yaml")
	writeFile(t, contractPath, minimalContractYAML(`    - operationId: existing
      method: GET
      path: /v1/existing
      auth: none
`))

	gotDir, adopted, err := writeBootstrapFiles(tmp, repoBootstrapOptions{apiDir: "apig"}, loadTestContract(), loadTestApigConfig())
	if err != nil {
		t.Fatalf("writeBootstrapFiles: %v", err)
	}
	if adopted {
		t.Fatal("expected existing api-contract.yaml to be preserved")
	}
	if gotDir != apiDir {
		t.Fatalf("api dir = %q, want %q", gotDir, apiDir)
	}
	assertFileExists(t, filepath.Join(apiDir, "api-contract.detected.yaml"))
	assertFileContains(t, contractPath, "operationId: existing")
}

func TestRepoBootstrapDefaultGeneratedDirAvoidsGenericAPIName(t *testing.T) {
	tmp := t.TempDir()
	opts := repoBootstrapOptions{outDir: tmp, apiDir: "apig"}
	if got, want := filepath.Join(bootstrapOutputDir(opts), opts.apiDir), filepath.Join(tmp, "apig"); got != want {
		t.Fatalf("generated dir = %q, want %q", got, want)
	}
}

func TestRepoBootstrapInfersServiceNameFromRepo(t *testing.T) {
	cases := map[string]string{
		"https://github.com/org/my-service":     "my-service",
		"https://github.com/org/my-service.git": "my-service",
		"org/my-service":                        "my-service",
		"git@github.com:org/my-service.git":     "my-service",
	}

	for repo, want := range cases {
		got, err := serviceNameFromRepo(repo)
		if err != nil {
			t.Fatalf("serviceNameFromRepo(%q): %v", repo, err)
		}
		if got != want {
			t.Fatalf("serviceNameFromRepo(%q) = %q, want %q", repo, got, want)
		}
	}
}

func TestCommandSmoke_FreshCommandsDoNotLeakFlags(t *testing.T) {
	tmp := t.TempDir()
	one := filepath.Join(tmp, "one")
	two := filepath.Join(tmp, "two")

	if err := runCmd(t, "init", "--service", "one", "--out", one); err != nil {
		t.Fatalf("first init: %v", err)
	}
	if err := runCmd(t, "init", "--service", "two", "--out", two); err != nil {
		t.Fatalf("second init: %v", err)
	}

	assertFileContains(t, filepath.Join(one, "api-contract.yaml"), "name: one")
	assertFileContains(t, filepath.Join(two, "api-contract.yaml"), "name: two")
}

func loadTestContract() *contract.ApiContract {
	return &contract.ApiContract{
		APIVersion: "infra.example.com/v1",
		Kind:       "ApiContract",
		Metadata: contract.Metadata{
			Name:    "smoke",
			Title:   "Smoke API",
			Version: "1.0.0",
		},
		Spec: contract.Spec{
			BasePath: "/api",
			Routes: []contract.Route{
				{OperationID: "listPets", Method: "GET", Path: "/v1/pets", Auth: "none"},
			},
			Schemas: map[string]*contract.Schema{},
		},
	}
}

func loadTestApigConfig() *config.HuaweiApigConfig {
	return &config.HuaweiApigConfig{
		APIVersion: "infra.example.com/v1",
		Kind:       "HuaweiApigConfig",
		Metadata:   config.Metadata{Name: "smoke-apig"},
		Spec: config.HuaweiSpec{
			GatewayURL: "https://example.apic.cn-north-4.huaweicloudapis.com",
			Backend:    config.Backend{Type: "HTTP", Scheme: "https", Address: "backend.example.com", Timeout: 5000, RetryCount: "0"},
			Defaults:   config.Defaults{Cors: false, SendFgBodyBase64: true, MatchMode: "NORMAL", RequestType: "public", SecurityScheme: "apig-auth-app-header"},
			SecuritySchemes: map[string]config.SecurityScheme{
				"apig-auth-app-header": {Type: "AppSigv1", In: "header", Name: "Authorization", AppcodeAuthType: "header"},
			},
		},
	}
}

func writeGinRoutes(t *testing.T, src, getPath, postPath string) {
	t.Helper()
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(src, "routes.go"), `package smoke

func register() {
	r.GET("`+getPath+`", h)
	r.POST("`+postPath+`", h)
	r.GET("/debug/vars", h)
}
`)
}

func smokeOpenAPIYAML(path string) string {
	return `openapi: 3.0.3
info:
  title: Smoke OpenAPI
  version: 1.0.0
paths:
  ` + path + `:
    get:
      operationId: listPets
      summary: List pets
      parameters:
        - name: page
          in: query
          schema:
            type: integer
components:
  schemas:
    Pet:
      type: object
      properties:
        name:
          type: string
`
}

func smokeOpenAPIJSON(path string) string {
	return `{
  "openapi": "3.0.3",
  "info": { "title": "Smoke OpenAPI", "version": "1.0.0" },
  "paths": {
    "` + path + `": {
      "get": {
        "operationId": "listPets",
        "summary": "List pets",
        "parameters": [
          { "name": "page", "in": "query", "schema": { "type": "integer" } }
        ]
      }
    }
  },
  "components": {
    "schemas": {
      "Pet": {
        "type": "object",
        "properties": { "name": { "type": "string" } }
      }
    }
  }
}`
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file %s: %v", path, err)
	}
}

func assertNonEmptyFile(t *testing.T, path string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if len(b) == 0 {
		t.Fatalf("expected %s to be non-empty", path)
	}
}

func assertFileContains(t *testing.T, path, want string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !strings.Contains(string(b), want) {
		t.Fatalf("expected %s to contain %q\n%s", path, want, string(b))
	}
}

func loadLooseContract(t *testing.T, path string) *contract.ApiContract {
	t.Helper()
	c, err := config.LoadContractLoose(path)
	if err != nil {
		t.Fatalf("load contract %s: %v", path, err)
	}
	return c
}

func assertHasRoute(t *testing.T, c *contract.ApiContract, method, path string) {
	t.Helper()
	if !hasRoute(c, method, path) {
		t.Fatalf("expected route %s %s in %#v", method, path, c.Spec.Routes)
	}
}

func assertLacksRoute(t *testing.T, c *contract.ApiContract, method, path string) {
	t.Helper()
	if hasRoute(c, method, path) {
		t.Fatalf("did not expect route %s %s in %#v", method, path, c.Spec.Routes)
	}
}

func hasRoute(c *contract.ApiContract, method, path string) bool {
	for _, r := range c.Spec.Routes {
		if r.Method == method && r.Path == path {
			return true
		}
	}
	return false
}

func minimalContractYAML(routes string) string {
	return `apiVersion: infra.example.com/v1
kind: ApiContract
metadata:
  name: smoke
  title: Smoke API
  version: 1.0.0
spec:
  basePath: /api
  routes:
` + routes + `  schemas: {}
`
}
