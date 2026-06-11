package gin

import (
	"strings"
	"testing"
)

func TestAnalyzeFiles_DirectRoutes(t *testing.T) {
	routes := analyzeFixture(t, `
package server

import "github.com/gin-gonic/gin"

func register(r *gin.Engine) {
	r.GET("/v1/pkg", listPkgs)
	r.POST("/v1/pkg", createPkg)
}
`)
	if len(routes) != 2 {
		t.Fatalf("want 2 routes, got %d (%+v)", len(routes), routes)
	}
	if routes[0].Method != "GET" || routes[0].Path != "/v1/pkg" {
		t.Errorf("first route wrong: %+v", routes[0])
	}
	if routes[1].Method != "POST" || routes[1].Path != "/v1/pkg" {
		t.Errorf("second route wrong: %+v", routes[1])
	}
}

func TestAnalyzeFiles_GroupPrefix(t *testing.T) {
	routes := analyzeFixture(t, `
package server

func register(r *gin.Engine) {
	api := r.Group("/api")
	api.GET("/pets", h)
}
`)
	if len(routes) != 1 {
		t.Fatalf("want 1 route, got %d (%+v)", len(routes), routes)
	}
	if routes[0].Path != "/api/pets" {
		t.Fatalf("path: want /api/pets, got %q", routes[0].Path)
	}
}

func TestAnalyzeFiles_NestedGroupPrefixAndPathParam(t *testing.T) {
	routes := analyzeFixture(t, `
package server

func register(r *gin.Engine) {
	api := r.Group("/api")
	v1 := api.Group("/v1")
	v1.GET("/pets/:id", h)
}
`)
	if len(routes) != 1 {
		t.Fatalf("want 1 route, got %d (%+v)", len(routes), routes)
	}
	if routes[0].Path != "/api/v1/pets/{id}" {
		t.Fatalf("path: want /api/v1/pets/{id}, got %q", routes[0].Path)
	}
	assertRouteParam(t, routes[0], "id")
}

func TestAnalyzeFiles_MultiplePathParams(t *testing.T) {
	routes := analyzeFixture(t, `
package server

func register(r *gin.Engine) {
	v1 := r.Group("/api/v1")
	v1.DELETE("/packages/:pkg/files/:file", h)
}
`)
	if len(routes) != 1 {
		t.Fatalf("want 1 route, got %d (%+v)", len(routes), routes)
	}
	if routes[0].Path != "/api/v1/packages/{pkg}/files/{file}" {
		t.Fatalf("path: want /api/v1/packages/{pkg}/files/{file}, got %q", routes[0].Path)
	}
	assertRouteParam(t, routes[0], "pkg")
	assertRouteParam(t, routes[0], "file")
}

func TestAnalyzeFiles_InlineGroupRoute(t *testing.T) {
	routes := analyzeFixture(t, `
package server

func register(r *gin.Engine) {
	r.Group("/v1").GET("/ping", h)
}
`)
	if len(routes) != 1 {
		t.Fatalf("want 1 route, got %d (%+v)", len(routes), routes)
	}
	if routes[0].Path != "/v1/ping" {
		t.Fatalf("path: want /v1/ping, got %q", routes[0].Path)
	}
}

func TestAnalyzeFiles_VarDeclaredGroupPrefix(t *testing.T) {
	routes := analyzeFixture(t, `
package server

func register(r *gin.Engine) {
	var api = r.Group("/api")
	api.GET("/pets", h)
}
`)
	if len(routes) != 1 {
		t.Fatalf("want 1 route, got %d (%+v)", len(routes), routes)
	}
	if routes[0].Path != "/api/pets" {
		t.Fatalf("path: want /api/pets, got %q", routes[0].Path)
	}
}

func TestAnalyzeFiles_RouteCallInAssignment(t *testing.T) {
	routes := analyzeFixture(t, `
package server

func register(r *gin.Engine) {
	_ = r.GET("/assigned", h)
}
`)
	if len(routes) != 1 {
		t.Fatalf("want 1 route, got %d (%+v)", len(routes), routes)
	}
	if routes[0].Path != "/assigned" {
		t.Fatalf("path: want /assigned, got %q", routes[0].Path)
	}
}

func TestAnalyzeFiles_UnquotesInterpretedStrings(t *testing.T) {
	routes := analyzeFixture(t, `
package server

func register(r *gin.Engine) {
	r.GET("/v1/\x70ets", h)
}
`)
	if len(routes) != 1 {
		t.Fatalf("want 1 route, got %d (%+v)", len(routes), routes)
	}
	if routes[0].Path != "/v1/pets" {
		t.Fatalf("path: want /v1/pets, got %q", routes[0].Path)
	}
}

func TestAnalyzeFiles_IgnoresNonMethodCalls(t *testing.T) {
	routes := analyzeFixture(t, `
package server

func register(r *gin.Engine) {
	r.Use(middleware)
	r.Group("/v1")
	r.Static("/assets", "./assets")
}
`)
	if len(routes) != 0 {
		t.Fatalf("want 0 routes, got %d (%+v)", len(routes), routes)
	}
}

func TestAnalyzeFiles_IgnoresNonGinReceiver(t *testing.T) {
	routes := analyzeFixture(t, `
package server

type client struct{}

func (c client) GET(path string, h any) {}

func register(r *gin.Engine) {
	client{}.GET("/not-a-route", h)
	r.GET("/real", h)
}
`)
	if len(routes) != 1 {
		t.Fatalf("want 1 route, got %d (%+v)", len(routes), routes)
	}
	if routes[0].Path != "/real" {
		t.Fatalf("path: want /real, got %q", routes[0].Path)
	}
}

func TestAnalyzeFiles_LocalGinConstructorRouter(t *testing.T) {
	routes := analyzeFixture(t, `
package server

func register() {
	r := gin.New()
	r.GET("/local", h)
}
`)
	if len(routes) != 1 {
		t.Fatalf("want 1 route, got %d (%+v)", len(routes), routes)
	}
	if routes[0].Path != "/local" {
		t.Fatalf("path: want /local, got %q", routes[0].Path)
	}
}

func TestAnalyzeFiles_RouterGroupParameter(t *testing.T) {
	routes := analyzeFixture(t, `
package server

func register(r *gin.RouterGroup) {
	r.GET("/group", h)
}
`)
	if len(routes) != 1 {
		t.Fatalf("want 1 route, got %d (%+v)", len(routes), routes)
	}
	if routes[0].Path != "/group" {
		t.Fatalf("path: want /group, got %q", routes[0].Path)
	}
}

func TestAnalyzeFiles_GinAliasImport(t *testing.T) {
	routes, err := AnalyzeFiles(map[string][]byte{"routes.go": []byte(`
package server

import g "github.com/gin-gonic/gin"

func register(r *g.Engine) {
	r.GET("/alias", h)
}
`)})
	if err != nil {
		t.Fatalf("AnalyzeFiles: %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("want 1 route, got %d (%+v)", len(routes), routes)
	}
	if routes[0].Path != "/alias" {
		t.Fatalf("path: want /alias, got %q", routes[0].Path)
	}
}

func TestAnalyzeFiles_DirectGinConstructorReceiver(t *testing.T) {
	routes := analyzeFixture(t, `
package server

func register() {
	gin.Default().GET("/direct", h)
}
`)
	if len(routes) != 1 {
		t.Fatalf("want 1 route, got %d (%+v)", len(routes), routes)
	}
	if routes[0].Path != "/direct" {
		t.Fatalf("path: want /direct, got %q", routes[0].Path)
	}
}

func TestExtractRoutes_BasicGetPost(t *testing.T) {
	src := []byte(`
	package server

	func register(r *gin.Engine) {
		r.GET("/v1/pkg", listPkgs)
		r.POST("/v1/pkg", createPkg)
	}
	`)
	routes := ExtractRoutes(src)
	if len(routes) != 2 {
		t.Fatalf("want 2 routes, got %d (%+v)", len(routes), routes)
	}
	if routes[0].Method != "GET" || routes[0].Path != "/v1/pkg" {
		t.Errorf("first route wrong: %+v", routes[0])
	}
	if routes[1].Method != "POST" || routes[1].Path != "/v1/pkg" {
		t.Errorf("second route wrong: %+v", routes[1])
	}
}

func TestExtractRoutes_ColonParamToBraces(t *testing.T) {
	src := []byte(`r.GET("/v1/pkg/:id", getPkg)
g.DELETE("/v1/pkg/:id/files/:fn", deleteFile)`)
	routes := ExtractRoutes(src)
	if len(routes) != 2 {
		t.Fatalf("want 2 routes, got %d", len(routes))
	}
	if routes[0].Path != "/v1/pkg/{id}" {
		t.Errorf("first path: want /v1/pkg/{id}, got %q", routes[0].Path)
	}
	if routes[1].Path != "/v1/pkg/{id}/files/{fn}" {
		t.Errorf("second path: want /v1/pkg/{id}/files/{fn}, got %q", routes[1].Path)
	}
	assertRouteParam(t, routes[0], "id")
	assertRouteParam(t, routes[1], "id")
	assertRouteParam(t, routes[1], "fn")
}

func TestExtractRoutes_AllMethods(t *testing.T) {
	src := []byte(`
api.GET("/a", h)
api.POST("/b", h)
api.PUT("/c", h)
api.DELETE("/d", h)
api.PATCH("/e", h)
`)
	routes := ExtractRoutes(src)
	if len(routes) != 5 {
		t.Fatalf("want 5 routes, got %d", len(routes))
	}
	wantMethods := []string{"GET", "POST", "PUT", "DELETE", "PATCH"}
	for i, want := range wantMethods {
		if routes[i].Method != want {
			t.Errorf("route %d method: want %s, got %s", i, want, routes[i].Method)
		}
	}
}

func TestExtractRoutes_IgnoresCommentedOutCode(t *testing.T) {
	src := []byte(`
// r.GET("/dead/path", h)
/* r.POST("/also/dead", h) */
r.GET("/alive", h)
`)
	routes := ExtractRoutes(src)
	if len(routes) != 1 {
		t.Fatalf("want 1 live route, got %d (%+v)", len(routes), routes)
	}
	if routes[0].Path != "/alive" {
		t.Errorf("path: want /alive, got %q", routes[0].Path)
	}
}

func TestExtractRoutes_DifferentRouterVarNames(t *testing.T) {
	src := []byte(`
r.GET("/a", h)
router.GET("/b", h)
v1.GET("/c", h)
api.GET("/d", h)
rg.GET("/e", h)
group.GET("/f", h)
`)
	routes := ExtractRoutes(src)
	if len(routes) != 6 {
		t.Fatalf("want 6 routes, got %d", len(routes))
	}
}

func TestExtractRoutes_IgnoresNonMethodCalls(t *testing.T) {
	src := []byte(`
r.Use(middleware)
r.Group("/v1")
r.Static("/assets", "./assets")
r.GET("/real", h)
`)
	routes := ExtractRoutes(src)
	if len(routes) != 1 {
		t.Fatalf("want 1 route, got %d (%+v)", len(routes), routes)
	}
}

func TestNormalizePath(t *testing.T) {
	cases := map[string]string{
		"/v1/pkg":          "/v1/pkg",
		"/v1/pkg/:id":      "/v1/pkg/{id}",
		"/v1/:a/:b/:c":     "/v1/{a}/{b}/{c}",
		"/static-path-123": "/static-path-123",
	}
	for in, want := range cases {
		if got := normalizePath(in); got != want {
			t.Errorf("normalizePath(%q): want %q, got %q", in, want, got)
		}
	}
}

func analyzeFixture(t *testing.T, src string) []RouteCandidate {
	t.Helper()
	if !strings.Contains(src, "github.com/gin-gonic/gin") {
		src = strings.Replace(src, "package server\n", "package server\n\nimport \"github.com/gin-gonic/gin\"\n", 1)
	}
	routes, err := AnalyzeFiles(map[string][]byte{"routes.go": []byte(src)})
	if err != nil {
		t.Fatalf("AnalyzeFiles: %v", err)
	}
	return routes
}

func assertRouteParam(t *testing.T, r RouteCandidate, name string) {
	t.Helper()
	for _, p := range r.Parameters {
		if p.Name == name && p.In == "path" && p.Type == "string" && p.Required {
			return
		}
	}
	t.Fatalf("expected path parameter %q in %+v", name, r.Parameters)
}
