package apply

import (
	"testing"

	"gen-openapi/internal/diff"
	"gen-openapi/pkg/contract"
)

// ---------------------------------------------------------------------------
// test helpers
// ---------------------------------------------------------------------------

func mkContract(routes ...contract.Route) *contract.ApiContract {
	return &contract.ApiContract{
		APIVersion: "infra.example.com/v1",
		Kind:       "ApiContract",
		Metadata: contract.Metadata{
			Name:    "test",
			Title:   "Test Service",
			Version: "1.0.0",
		},
		Spec: contract.Spec{
			BasePath: "/api",
			Routes:   routes,
			Schemas:  map[string]*contract.Schema{},
		},
	}
}

func route(method, path string, params ...contract.Parameter) contract.Route {
	r := contract.Route{
		OperationID: method + "_" + path,
		Method:      method,
		Path:        path,
		Auth:        "none",
	}
	if len(params) > 0 {
		r.Parameters = params
	}
	return r
}

func pathParam(name string) contract.Parameter {
	return contract.Parameter{
		Name:     name,
		In:       "path",
		Type:     "string",
		Required: true,
	}
}

func queryParam(name string) contract.Parameter {
	return contract.Parameter{
		Name: name,
		In:   "query",
		Type: "string",
	}
}

func withBackendPath(r contract.Route, backendPath string) contract.Route {
	r.BackendPath = backendPath
	return r
}

func withAuth(r contract.Route, auth string) contract.Route {
	r.Auth = auth
	return r
}

func withSummary(r contract.Route, summary string) contract.Route {
	r.Summary = summary
	return r
}

func withDescription(r contract.Route, desc string) contract.Route {
	r.Description = desc
	return r
}

func withRequestBody(r contract.Route, schema string, required bool) contract.Route {
	r.RequestBody = &contract.RequestBody{
		Schema:   schema,
		Required: required,
	}
	return r
}

func withSchemas(c *contract.ApiContract, schemas map[string]*contract.Schema) *contract.ApiContract {
	if c.Spec.Schemas == nil {
		c.Spec.Schemas = map[string]*contract.Schema{}
	}
	for k, v := range schemas {
		c.Spec.Schemas[k] = v
	}
	return c
}

func mustMerge(t *testing.T, canonical, detected *contract.ApiContract, opts Options) (*contract.ApiContract, *Report) {
	t.Helper()
	merged, rep, err := Merge(canonical, detected, opts)
	if err != nil {
		t.Fatalf("Merge failed: %v", err)
	}
	return merged, rep
}

// ---------------------------------------------------------------------------
// route lookup helper for assertions
// ---------------------------------------------------------------------------

func findRoute(routes []contract.Route, method, path string) *contract.Route {
	for i := range routes {
		if routes[i].Method == method && routes[i].Path == path {
			return &routes[i]
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// 1. Detected public routes appear in merged output.
func TestMerge_AddedRoutes(t *testing.T) {
	canonical := mkContract(route("GET", "/v1/existing"))
	detected := mkContract(
		route("GET", "/v1/existing"),
		route("POST", "/v1/added"),
	)
	merged, rep := mustMerge(t, canonical, detected, Options{})

	if findRoute(merged.Spec.Routes, "POST", "/v1/added") == nil {
		t.Fatal("expected added route POST /v1/added to be present")
	}
	if len(rep.Added) != 1 {
		t.Fatalf("expected 1 added, got %d", len(rep.Added))
	}
}

// 2. New params from detected appended; canonical params preserved.
func TestMerge_ChangedRoute_MergeParameters(t *testing.T) {
	canonical := mkContract(
		route("GET", "/v1/pkgs", queryParam("page")),
	)
	detected := mkContract(
		route("GET", "/v1/pkgs", queryParam("page"), queryParam("limit")),
	)
	merged, rep := mustMerge(t, canonical, detected, Options{})

	r := findRoute(merged.Spec.Routes, "GET", "/v1/pkgs")
	if r == nil {
		t.Fatal("expected GET /v1/pkgs to exist")
	}
	if len(r.Parameters) != 2 {
		t.Fatalf("expected 2 parameters, got %d", len(r.Parameters))
	}
	if len(rep.Merged) != 1 {
		t.Fatalf("expected 1 merged, got %d", len(rep.Merged))
	}
}

// 3. Canonical auth not overwritten.
func TestMerge_ChangedRoute_PreserveAuth(t *testing.T) {
	canonical := mkContract(
		withAuth(route("POST", "/v1/secure"), "app"),
	)
	detected := mkContract(
		withAuth(route("POST", "/v1/secure"), "none"),
	)
	merged, _ := mustMerge(t, canonical, detected, Options{})

	r := findRoute(merged.Spec.Routes, "POST", "/v1/secure")
	if r == nil {
		t.Fatal("expected POST /v1/secure to exist")
	}
	if r.Auth != "app" {
		t.Fatalf("expected auth 'app' to be preserved, got %q", r.Auth)
	}
}

// 4. Canonical summary/description/backendPath preserved.
func TestMerge_ChangedRoute_PreserveCuratedFields(t *testing.T) {
	canonical := mkContract(
		withBackendPath(
			withDescription(
				withSummary(route("GET", "/v1/item"), "My summary"),
				"My description",
			),
			"/api/v1/item",
		),
	)
	detected := mkContract(
		route("GET", "/v1/item"),
	)
	merged, _ := mustMerge(t, canonical, detected, Options{})

	r := findRoute(merged.Spec.Routes, "GET", "/v1/item")
	if r.Summary != "My summary" {
		t.Fatalf("expected summary 'My summary', got %q", r.Summary)
	}
	if r.Description != "My description" {
		t.Fatalf("expected description 'My description', got %q", r.Description)
	}
	if r.BackendPath != "/api/v1/item" {
		t.Fatalf("expected backendPath '/api/v1/item', got %q", r.BackendPath)
	}
}

// 5. Canonical has no body, detected has one → body added.
func TestMerge_ChangedRoute_RequestBodyFromDetected(t *testing.T) {
	canonical := mkContract(route("POST", "/v1/item"))
	detected := mkContract(withRequestBody(route("POST", "/v1/item"), "ItemRequest", true))
	merged, _ := mustMerge(t, canonical, detected, Options{})

	r := findRoute(merged.Spec.Routes, "POST", "/v1/item")
	if r.RequestBody == nil {
		t.Fatal("expected requestBody to be added")
	}
	if r.RequestBody.Schema != "ItemRequest" {
		t.Fatalf("expected schema 'ItemRequest', got %q", r.RequestBody.Schema)
	}
}

// 6. Both have body → canonical body preserved.
func TestMerge_ChangedRoute_KeepCanonicalRequestBody(t *testing.T) {
	canonical := mkContract(
		withRequestBody(route("POST", "/v1/item"), "CanonicalRequest", true),
	)
	detected := mkContract(
		withRequestBody(route("POST", "/v1/item"), "DetectedRequest", false),
	)
	merged, _ := mustMerge(t, canonical, detected, Options{})

	r := findRoute(merged.Spec.Routes, "POST", "/v1/item")
	if r.RequestBody.Schema != "CanonicalRequest" {
		t.Fatalf("expected schema 'CanonicalRequest', got %q", r.RequestBody.Schema)
	}
	if !r.RequestBody.Required {
		t.Fatal("expected required=true to be preserved")
	}
}

// 7. Removed route stays in output without --prune.
func TestMerge_RemovedSkippedByDefault(t *testing.T) {
	canonical := mkContract(
		route("GET", "/v1/old"),
		route("GET", "/v1/keep"),
	)
	detected := mkContract(route("GET", "/v1/keep"))
	merged, rep := mustMerge(t, canonical, detected, Options{})

	if findRoute(merged.Spec.Routes, "GET", "/v1/old") == nil {
		t.Fatal("expected removed route GET /v1/old to be kept")
	}
	if len(rep.Skipped) != 1 {
		t.Fatalf("expected 1 skipped, got %d", len(rep.Skipped))
	}
	if len(rep.Pruned) != 0 {
		t.Fatalf("expected 0 pruned, got %d", len(rep.Pruned))
	}
}

// 8. With --prune, removed route is gone.
func TestMerge_RemovedPruned(t *testing.T) {
	canonical := mkContract(
		route("GET", "/v1/old"),
		route("GET", "/v1/keep"),
	)
	detected := mkContract(route("GET", "/v1/keep"))
	merged, rep := mustMerge(t, canonical, detected, Options{Prune: true})

	if findRoute(merged.Spec.Routes, "GET", "/v1/old") != nil {
		t.Fatal("expected removed route GET /v1/old to be pruned")
	}
	if len(rep.Pruned) != 1 {
		t.Fatalf("expected 1 pruned, got %d", len(rep.Pruned))
	}
	if findRoute(merged.Spec.Routes, "GET", "/v1/keep") == nil {
		t.Fatal("expected route GET /v1/keep to remain")
	}
}

// 9. Internal path not in output without --include-internal.
func TestMerge_InternalSkippedByDefault(t *testing.T) {
	canonical := mkContract()
	detected := mkContract(
		route("GET", "/v1/public"),
		route("GET", "/debug/pprof"),
	)
	merged, rep := mustMerge(t, canonical, detected, Options{})

	if findRoute(merged.Spec.Routes, "GET", "/debug/pprof") != nil {
		t.Fatal("expected internal path /debug/pprof to be skipped")
	}
	if findRoute(merged.Spec.Routes, "GET", "/v1/public") == nil {
		t.Fatal("expected public path /v1/public to be present")
	}
	if len(rep.Skipped) != 1 {
		t.Fatalf("expected 1 skipped (internal), got %d", len(rep.Skipped))
	}
}

// 10. With --include-internal, internal path is added.
func TestMerge_InternalIncluded(t *testing.T) {
	canonical := mkContract()
	detected := mkContract(
		route("GET", "/debug/pprof"),
	)
	merged, rep := mustMerge(t, canonical, detected, Options{IncludeInternal: true})

	if findRoute(merged.Spec.Routes, "GET", "/debug/pprof") == nil {
		t.Fatal("expected internal path /debug/pprof to be included")
	}
	// Included internal routes are recorded in Added
	found := false
	for _, a := range rep.Added {
		if a.Key.Method == "GET" && a.Key.Path == "/debug/pprof" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected /debug/pprof in rep.Added")
	}
}

// 11. Path parameter auto-declaration.
func TestMerge_PathParamAutoDeclaration(t *testing.T) {
	canonical := mkContract()
	detected := mkContract(
		route("GET", "/v1/items/{id}"),
	)
	merged, _ := mustMerge(t, canonical, detected, Options{})

	r := findRoute(merged.Spec.Routes, "GET", "/v1/items/{id}")
	if r == nil {
		t.Fatal("expected GET /v1/items/{id} to exist")
	}
	found := false
	for _, p := range r.Parameters {
		if p.Name == "id" && p.In == "path" && p.Type == "string" && p.Required {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected path param 'id' declared; got %+v", r.Parameters)
	}
}

// 12. Schema referenced by new route copied from detected.
func TestMerge_SchemaCopied(t *testing.T) {
	canonical := mkContract()
	detected := withSchemas(
		mkContract(
			withRequestBody(route("POST", "/v1/item"), "ItemRequest", true),
		),
		map[string]*contract.Schema{
			"ItemRequest": {Type: "object", Properties: map[string]*contract.Schema{
				"name": {Type: "string"},
			}},
		},
	)
	merged, rep := mustMerge(t, canonical, detected, Options{})

	if _, ok := merged.Spec.Schemas["ItemRequest"]; !ok {
		t.Fatal("expected schema 'ItemRequest' to be copied")
	}
	if len(rep.AddedSchema) != 1 || rep.AddedSchema[0] != "ItemRequest" {
		t.Fatalf("expected AddedSchema to include 'ItemRequest', got %v", rep.AddedSchema)
	}
}

// 13. Schema name exists in canonical → not overwritten.
func TestMerge_SchemaConflictPreservesCanonical(t *testing.T) {
	canonical := withSchemas(
		mkContract(),
		map[string]*contract.Schema{
			"SharedSchema": {Type: "object", Description: "canonical version"},
		},
	)
	detected := withSchemas(
		mkContract(
			withRequestBody(route("POST", "/v1/item"), "SharedSchema", true),
		),
		map[string]*contract.Schema{
			"SharedSchema": {Type: "object", Description: "detected version"},
		},
	)
	merged, _ := mustMerge(t, canonical, detected, Options{})

	s, ok := merged.Spec.Schemas["SharedSchema"]
	if !ok {
		t.Fatal("expected schema 'SharedSchema' to exist")
	}
	if s.Description != "canonical version" {
		t.Fatalf("expected canonical description, got %q", s.Description)
	}
}

// 14. Nil canonical → treated as empty contract.
func TestMerge_NilCanonical(t *testing.T) {
	detected := mkContract(route("GET", "/v1/item"))
	merged, rep := mustMerge(t, nil, detected, Options{})

	if findRoute(merged.Spec.Routes, "GET", "/v1/item") == nil {
		t.Fatal("expected route to be added from nil canonical")
	}
	if len(rep.Added) != 1 {
		t.Fatalf("expected 1 added, got %d", len(rep.Added))
	}
}

// 15. No drift → unchanged contract.
func TestMerge_NoDrift(t *testing.T) {
	canonical := mkContract(route("GET", "/v1/item"))
	detected := mkContract(route("GET", "/v1/item"))
	merged, rep := mustMerge(t, canonical, detected, Options{})

	if len(merged.Spec.Routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(merged.Spec.Routes))
	}
	if rep.routeCount() != 0 {
		t.Fatalf("expected 0 changes, got %d", rep.routeCount())
	}
}

// 16. Added route gets backendPath from basePath.
func TestMerge_AddedRouteBackendPath(t *testing.T) {
	canonical := mkContract()
	detected := mkContract(route("POST", "/v1/create"))
	merged, _ := mustMerge(t, canonical, detected, Options{})

	r := findRoute(merged.Spec.Routes, "POST", "/v1/create")
	if r.BackendPath != "/api/v1/create" {
		t.Fatalf("expected backendPath '/api/v1/create', got %q", r.BackendPath)
	}
}

// 17. Added route with existing backendPath is preserved.
func TestMerge_AddedRoutePreservesExistingBackendPath(t *testing.T) {
	detected := mkContract()
	detected.Spec.Routes = append(detected.Spec.Routes,
		withBackendPath(route("POST", "/v1/custom"), "/custom/backend"))

	canonical := mkContract()
	merged, _ := mustMerge(t, canonical, detected, Options{})

	r := findRoute(merged.Spec.Routes, "POST", "/v1/custom")
	if r.BackendPath != "/custom/backend" {
		t.Fatalf("expected backendPath '/custom/backend', got %q", r.BackendPath)
	}
}

// 18. Detected nil → error.
func TestMerge_DetectedNil(t *testing.T) {
	_, _, err := Merge(mkContract(), nil, Options{})
	if err == nil {
		t.Fatal("expected error for nil detected")
	}
}

// 19. Route with path params from detected gets automatically declared.
func TestMerge_MultiplePathParamsDeclared(t *testing.T) {
	canonical := mkContract()
	detected := mkContract(route("GET", "/v1/users/{userId}/posts/{postId}"))
	merged, _ := mustMerge(t, canonical, detected, Options{})

	r := findRoute(merged.Spec.Routes, "GET", "/v1/users/{userId}/posts/{postId}")
	if r == nil {
		t.Fatal("expected route to exist")
	}
	params := map[string]bool{}
	for _, p := range r.Parameters {
		if p.In == "path" {
			params[p.Name] = true
		}
	}
	if !params["userId"] {
		t.Fatal("expected path param userId")
	}
	if !params["postId"] {
		t.Fatal("expected path param postId")
	}
}

// 20. Schema chain: route references schema that references another schema.
func TestMerge_TransitiveSchemaCopy(t *testing.T) {
	canonical := mkContract()
	detected := withSchemas(
		mkContract(
			withRequestBody(route("POST", "/v1/review"), "ReviewRequest", true),
		),
		map[string]*contract.Schema{
			"ReviewRequest": {
				Type: "object",
				Properties: map[string]*contract.Schema{
					"item": {Ref: "#/components/schemas/Item"},
				},
			},
			"Item": {Type: "object"},
		},
	)
	merged, rep := mustMerge(t, canonical, detected, Options{})

	if _, ok := merged.Spec.Schemas["ReviewRequest"]; !ok {
		t.Fatal("expected schema 'ReviewRequest' to be copied")
	}
	if _, ok := merged.Spec.Schemas["Item"]; !ok {
		t.Fatalf("expected schema 'Item' to be transitively copied")
	}
	_ = rep
}

// 21. Report counters are correct for mixed scenario.
func TestMerge_ReportCounters(t *testing.T) {
	canonical := mkContract(
		route("GET", "/v1/keep"),
		route("GET", "/v1/remove"),
	)
	detected := mkContract(
		route("GET", "/v1/keep"),
		route("POST", "/v1/new"),
		route("GET", "/debug/pprof"),
	)
	merged, rep := mustMerge(t, canonical, detected, Options{Prune: true})

	if len(rep.Added) != 1 {
		t.Fatalf("expected 1 added, got %d", len(rep.Added))
	}
	if len(rep.Pruned) != 1 {
		t.Fatalf("expected 1 pruned, got %d", len(rep.Pruned))
	}
	if len(rep.Skipped) != 1 {
		t.Fatalf("expected 1 skipped (internal), got %d", len(rep.Skipped))
	}
	_ = merged
}

// 22. BackendPath on changed route is preserved even if detected has a different one.
func TestMerge_ChangedRoute_PreservesBackendPath(t *testing.T) {
	canonical := mkContract(
		withBackendPath(route("PUT", "/v1/item"), "/api/v1/item"),
	)
	detected := mkContract(
		withBackendPath(route("PUT", "/v1/item"), "/different/backend"),
	)
	merged, _ := mustMerge(t, canonical, detected, Options{})

	r := findRoute(merged.Spec.Routes, "PUT", "/v1/item")
	if r.BackendPath != "/api/v1/item" {
		t.Fatalf("expected canonical backendPath preserved, got %q", r.BackendPath)
	}
}

// 23. Multiple changed routes all processed.
func TestMerge_MultipleChangedRoutes(t *testing.T) {
	canonical := mkContract(
		route("GET", "/v1/a"),
		route("GET", "/v1/b"),
	)
	detected := mkContract(
		route("GET", "/v1/a", queryParam("new_param")),
		route("GET", "/v1/b", queryParam("another_param")),
	)
	merged, rep := mustMerge(t, canonical, detected, Options{})

	if len(rep.Merged) != 2 {
		t.Fatalf("expected 2 merged, got %d", len(rep.Merged))
	}
	_ = merged
}

// 24. Changed route: canonical already has requestBody, detected has same → no change.
func TestMerge_ChangedRoute_SameRequestBody(t *testing.T) {
	canonical := mkContract(
		withRequestBody(route("POST", "/v1/item"), "ItemRequest", true),
	)
	detected := mkContract(
		withRequestBody(route("POST", "/v1/item"), "ItemRequest", true),
	)
	merged, _ := mustMerge(t, canonical, detected, Options{})

	r := findRoute(merged.Spec.Routes, "POST", "/v1/item")
	if r.RequestBody.Schema != "ItemRequest" {
		t.Fatalf("expected schema 'ItemRequest', got %q", r.RequestBody.Schema)
	}
}

// 25. Schema-only contract (no route changes) produces empty report.
func TestMerge_SchemaOnlyContract(t *testing.T) {
	canonical := withSchemas(
		mkContract(route("GET", "/v1/item")),
		map[string]*contract.Schema{
			"Item": {Type: "object"},
		},
	)
	detected := mkContract(route("GET", "/v1/item"))
	merged, rep := mustMerge(t, canonical, detected, Options{})

	if len(merged.Spec.Routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(merged.Spec.Routes))
	}
	if rep.routeCount() != 0 {
		t.Fatalf("expected empty report, got %d changes", rep.routeCount())
	}
}

// 26. Route with no method should trigger validation error.
func TestMerge_InvalidRoute_FailsValidation(t *testing.T) {
	canonical := mkContract()
	detected := mkContract(
		contract.Route{OperationID: "bad", Method: "", Path: "/v1/bad"},
	)
	_, _, err := Merge(canonical, detected, Options{})
	if err == nil {
		t.Fatal("expected validation error for route with empty method")
	}
}

// 27. Empty detected (no routes) with --prune removes everything but
// fails validation. Without --prune, canonical routes are kept.
func TestMerge_EmptyDetected(t *testing.T) {
	t.Run("prune all fails validation", func(t *testing.T) {
		canonical := mkContract(route("GET", "/v1/item"))
		detected := mkContract()
		_, _, err := Merge(canonical, detected, Options{Prune: true})
		if err == nil {
			t.Fatal("expected validation error when pruning all routes")
		}
	})

	t.Run("keep all without prune", func(t *testing.T) {
		canonical := mkContract(route("GET", "/v1/item"))
		detected := mkContract()
		merged, _ := mustMerge(t, canonical, detected, Options{})
		if findRoute(merged.Spec.Routes, "GET", "/v1/item") == nil {
			t.Fatal("expected canonical route to be kept")
		}
	})
}

// 28. Multiple Added routes with schemas.
func TestMerge_MultipleAddedWithSchemas(t *testing.T) {
	canonical := mkContract()
	detected := withSchemas(
		mkContract(
			withRequestBody(route("POST", "/v1/a"), "ARequest", true),
			withRequestBody(route("POST", "/v1/b"), "BRequest", true),
		),
		map[string]*contract.Schema{
			"ARequest": {Type: "object"},
			"BRequest": {Type: "object"},
		},
	)
	merged, rep := mustMerge(t, canonical, detected, Options{})

	if len(merged.Spec.Routes) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(merged.Spec.Routes))
	}
	if len(rep.AddedSchema) != 2 {
		t.Fatalf("expected 2 schemas copied, got %d", len(rep.AddedSchema))
	}
}

// Helper verification: diff.RouteKey is accessible
var _ = diff.RouteKey{}
