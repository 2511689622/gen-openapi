package diff

import (
	"strings"
	"testing"

	"gen-openapi/pkg/contract"
)

func mkContract(routes ...contract.Route) *contract.ApiContract {
	return &contract.ApiContract{
		APIVersion: "infra.example.com/v1",
		Kind:       "ApiContract",
		Spec:       contract.Spec{Routes: routes},
	}
}

func route(method, path string, params ...contract.Parameter) contract.Route {
	return contract.Route{
		OperationID: strings.ToLower(method) + "_" + strings.ReplaceAll(path, "/", "_"),
		Method:      method,
		Path:        path,
		Parameters:  params,
	}
}

func TestCompare_NoDifference(t *testing.T) {
	r := Compare(
		mkContract(route("GET", "/v1/pkg")),
		mkContract(route("GET", "/v1/pkg")),
	)
	if !r.IsEmpty() {
		t.Fatalf("expected empty, got %+v", r)
	}
}

func TestCompare_AddedAndRemoved(t *testing.T) {
	r := Compare(
		mkContract(route("GET", "/v1/old")),
		mkContract(route("GET", "/v1/new")),
	)
	if len(r.Added) != 1 || r.Added[0].Key.Path != "/v1/new" {
		t.Errorf("added wrong: %+v", r.Added)
	}
	if len(r.Removed) != 1 || r.Removed[0].Key.Path != "/v1/old" {
		t.Errorf("removed wrong: %+v", r.Removed)
	}
}

func TestCompare_NotExposedRoutedToInternalBucket(t *testing.T) {
	r := Compare(
		mkContract(), // empty canonical
		mkContract(
			route("GET", "/v1/pkg"),
			route("GET", "/debug/pprof"),
			route("GET", "/metrics"),
			route("POST", "/internal/reload"),
		),
	)
	if len(r.Added) != 1 || r.Added[0].Key.Path != "/v1/pkg" {
		t.Errorf("public added wrong: %+v", r.Added)
	}
	if len(r.NotExposed) != 3 {
		t.Errorf("notExposed count: want 3, got %d (%+v)", len(r.NotExposed), r.NotExposed)
	}
}

func TestCompare_ChangedParameter(t *testing.T) {
	canonical := mkContract(route("GET", "/v1/pkg",
		contract.Parameter{Name: "page_num", In: "query", Type: "integer"},
	))
	candidate := mkContract(route("GET", "/v1/pkg",
		contract.Parameter{Name: "page_num", In: "query", Type: "integer"},
		contract.Parameter{Name: "sig", In: "query", Type: "string"},
	))
	r := Compare(canonical, candidate)
	if len(r.Changed) != 1 {
		t.Fatalf("changed count: want 1, got %d", len(r.Changed))
	}
	reasons := strings.Join(r.Changed[0].Reasons, "\n")
	if !strings.Contains(reasons, "added query parameter sig") {
		t.Errorf("missing reason for added sig: %s", reasons)
	}
}

func TestCompare_ChangedAuth(t *testing.T) {
	a := route("POST", "/v1/pkg")
	a.Auth = "none"
	b := route("POST", "/v1/pkg")
	b.Auth = "app"

	r := Compare(mkContract(a), mkContract(b))
	if len(r.Changed) != 1 {
		t.Fatalf("changed count: want 1, got %d", len(r.Changed))
	}
	if !strings.Contains(r.Changed[0].Reasons[0], "auth") {
		t.Errorf("expected auth reason, got %v", r.Changed[0].Reasons)
	}
}

func TestCompare_AuthBlankVsNoneNotChange(t *testing.T) {
	a := route("GET", "/v1/pkg")
	a.Auth = ""
	b := route("GET", "/v1/pkg")
	b.Auth = "none"
	r := Compare(mkContract(a), mkContract(b))
	if len(r.Changed) != 0 {
		t.Errorf("blank vs none should not be a change, got %v", r.Changed)
	}
}

func TestCompare_ChangedRequestBody(t *testing.T) {
	a := route("POST", "/v1/pkg")
	b := route("POST", "/v1/pkg")
	b.RequestBody = &contract.RequestBody{Schema: "PkgRequest", Required: true}

	r := Compare(mkContract(a), mkContract(b))
	if len(r.Changed) != 1 {
		t.Fatalf("changed count: want 1, got %d", len(r.Changed))
	}
	if !strings.Contains(r.Changed[0].Reasons[0], "requestBody schema") {
		t.Errorf("expected body reason, got %v", r.Changed[0].Reasons)
	}
}

func TestCompare_NilCanonicalTreatedAsEmpty(t *testing.T) {
	r := Compare(nil, mkContract(route("GET", "/v1/pkg")))
	if len(r.Added) != 1 {
		t.Errorf("want 1 added, got %d", len(r.Added))
	}
}

func TestCompare_DeterministicOrdering(t *testing.T) {
	r := Compare(
		mkContract(),
		mkContract(
			route("GET", "/v1/c"),
			route("GET", "/v1/a"),
			route("POST", "/v1/a"),
			route("GET", "/v1/b"),
		),
	)
	wantOrder := []string{"GET /v1/a", "POST /v1/a", "GET /v1/b", "GET /v1/c"}
	got := []string{}
	for _, c := range r.Added {
		got = append(got, c.Key.String())
	}
	if strings.Join(wantOrder, "|") != strings.Join(got, "|") {
		t.Errorf("ordering: want %v, got %v", wantOrder, got)
	}
}

func TestMarkdown_RendersAllSections(t *testing.T) {
	canonical := mkContract(route("GET", "/v1/old"))
	candidate := mkContract(
		route("GET", "/v1/new"),
		route("GET", "/metrics"),
	)
	r := Compare(canonical, candidate)
	md := Markdown(r)

	must := []string{
		"新增候选接口",
		"`+ GET /v1/new`",
		"移除候选接口",
		"`- GET /v1/old`",
		"未纳入候选",
		"`* GET /metrics`",
	}
	for _, s := range must {
		if !strings.Contains(md, s) {
			t.Errorf("markdown missing %q\n---\n%s", s, md)
		}
	}
}

func TestMarkdown_EmptyReport(t *testing.T) {
	md := Markdown(Compare(mkContract(), mkContract()))
	if !strings.Contains(md, "_No drift detected._") {
		t.Errorf("expected no-drift sentinel, got:\n%s", md)
	}
}

func TestIsInternalPath_CommonCases(t *testing.T) {
	// (cross-package check that the shared blacklist works as the diff package
	// expects; full coverage lives in internal/discover.)
	internal := []string{"/debug/pprof", "/metrics", "/internal/reload", "/healthz", "/livez"}
	public := []string{"/v1/pkg", "/api/orders", "/users/{id}"}
	r := Compare(mkContract(), mkContract(
		append(routesFromPaths(internal), routesFromPaths(public)...)...,
	))
	if len(r.Added) != len(public) {
		t.Errorf("public added: want %d, got %d", len(public), len(r.Added))
	}
	if len(r.NotExposed) != len(internal) {
		t.Errorf("notExposed: want %d, got %d", len(internal), len(r.NotExposed))
	}
}

func routesFromPaths(paths []string) []contract.Route {
	out := make([]contract.Route, 0, len(paths))
	for _, p := range paths {
		out = append(out, route("GET", p))
	}
	return out
}
