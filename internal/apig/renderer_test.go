package apig

import (
	"strings"
	"testing"

	"gen-openapi/internal/config"
	"gen-openapi/pkg/contract"
)

func testApigConfig() *config.HuaweiApigConfig {
	return &config.HuaweiApigConfig{
		APIVersion: "infra.example.com/v1",
		Kind:       "HuaweiApigConfig",
		Spec: config.HuaweiSpec{
			GatewayURL: "https://gw.example.huaweicloudapis.com",
			Backend: config.Backend{
				Type:       "HTTP",
				Scheme:     "https",
				Address:    "backend.example.com",
				Timeout:    5000,
				RetryCount: "0",
			},
			Defaults: config.Defaults{
				MatchMode:      "NORMAL",
				RequestType:    "public",
				SecurityScheme: "apig-auth-app-header",
			},
			SecuritySchemes: map[string]config.SecurityScheme{
				"apig-auth-app-header": {
					Type:            "AppSigv1",
					In:              "header",
					Name:            "Authorization",
					AppcodeAuthType: "header",
				},
			},
		},
	}
}

func testContract() *contract.ApiContract {
	return &contract.ApiContract{
		APIVersion: "infra.example.com/v1",
		Kind:       "ApiContract",
		Metadata: contract.Metadata{
			Name:    "demo",
			Title:   "Demo",
			Version: "1.0.0",
		},
		Spec: contract.Spec{
			BasePath: "/api",
			Routes: []contract.Route{
				{
					OperationID: "listPkgs",
					Method:      "GET",
					Path:        "/v1/pkg",
					Auth:        "none",
				},
				{
					OperationID: "getPkg",
					Method:      "GET",
					Path:        "/v1/pkg/{id}",
					Auth:        "app",
					Parameters: []contract.Parameter{
						{Name: "id", In: "path", Type: "string", Required: true},
					},
				},
			},
		},
	}
}

func TestRender_RoundTripsThroughValidate(t *testing.T) {
	doc, err := Render(testContract(), testApigConfig())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if err := Validate(doc); err != nil {
		t.Fatalf("validate rendered doc: %v", err)
	}
	if doc.OpenAPI != "3.0.3" {
		t.Errorf("openapi version: want 3.0.3, got %q", doc.OpenAPI)
	}
	if len(doc.Paths) != 2 {
		t.Errorf("paths count: want 2, got %d", len(doc.Paths))
	}
}

func TestRender_BackendPathFromContractRoute(t *testing.T) {
	c := testContract()
	c.Spec.Routes[0].BackendPath = "/api/v1/pkg-listing"

	doc, err := Render(c, testApigConfig())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	op := doc.Paths["/v1/pkg"].Get
	if op.XApigatewayBackend.HttpEndpoints.Path != "/api/v1/pkg-listing" {
		t.Errorf("backend path: want /api/v1/pkg-listing, got %q",
			op.XApigatewayBackend.HttpEndpoints.Path)
	}
}

func TestRender_BackendPathFallsBackToBasePathJoin(t *testing.T) {
	doc, err := Render(testContract(), testApigConfig())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	op := doc.Paths["/v1/pkg"].Get
	if op.XApigatewayBackend.HttpEndpoints.Path != "/api/v1/pkg" {
		t.Errorf("backend path: want /api/v1/pkg, got %q",
			op.XApigatewayBackend.HttpEndpoints.Path)
	}
}

func TestRender_AuthAppEmitsSecurity(t *testing.T) {
	doc, err := Render(testContract(), testApigConfig())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	op := doc.Paths["/v1/pkg/{id}"].Get
	if len(op.Security) != 1 {
		t.Fatalf("security: want 1 entry, got %d", len(op.Security))
	}
	if _, ok := op.Security[0]["apig-auth-app-header"]; !ok {
		t.Errorf("security scheme not attached: %v", op.Security)
	}
}

func TestRender_MapsBackendParametersFromStringRequestParameters(t *testing.T) {
	c := testContract()
	c.Spec.Routes[1].Parameters = append(c.Spec.Routes[1].Parameters,
		contract.Parameter{Name: "count", In: "query", Type: "boolean"},
		contract.Parameter{Name: "page_num", In: "query", Type: "integer"},
	)

	doc, err := Render(c, testApigConfig())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	op := doc.Paths["/v1/pkg/{id}"].Get
	for _, p := range op.Parameters {
		if p.Schema.Type != "string" {
			t.Errorf("request parameter %s type: want string for APIG backend mapping, got %q", p.Name, p.Schema.Type)
		}
	}
	if got := len(op.XApigatewayBackend.Parameters); got != 3 {
		t.Fatalf("backend parameters: want 3, got %d", got)
	}
}

func TestRender_CustomAuthorizerScheme(t *testing.T) {
	cfg := testApigConfig()
	cfg.Spec.Defaults.SecurityScheme = "apig-auth-custom"
	cfg.Spec.SecuritySchemes = map[string]config.SecurityScheme{
		"apig-auth-custom": {
			Type: "AUTHORIZER",
			In:   "header",
			Name: "unused",
			Authorizer: &config.Authorizer{
				AuthDowngradeEnabled: false,
				AuthorizerAliasURI:   "",
				AuthorizerType:       "FUNC",
				AuthorizerURI:        "urn:fss:cn-north-4:25f40abeecb84d3e90731de258ca71ec:function:default:custom-auth",
				AuthorizerVersion:    "latest",
				NeedBody:             false,
				NetworkType:          "V1",
				RetryAttempts:        0,
				Timeout:              30000,
				TTL:                  0,
				Type:                 "FRONTEND",
			},
		},
	}

	doc, err := Render(testContract(), cfg)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	scheme := doc.Components.SecuritySchemes["apig-auth-custom"]
	if scheme.XApigatewayAuthType != "AUTHORIZER" {
		t.Errorf("auth type: want AUTHORIZER, got %q", scheme.XApigatewayAuthType)
	}
	if scheme.Name != "unused" {
		t.Errorf("scheme name: want unused, got %q", scheme.Name)
	}
	if scheme.XApigatewayAuthorizer == nil {
		t.Fatal("expected x-apigateway-authorizer")
	}
	if scheme.XApigatewayAuthorizer.Timeout != 30000 {
		t.Errorf("authorizer timeout: want 30000, got %d", scheme.XApigatewayAuthorizer.Timeout)
	}
	if scheme.XApigatewayAuthorizer.TTL != 0 {
		t.Errorf("authorizer ttl: want 0, got %d", scheme.XApigatewayAuthorizer.TTL)
	}
	op := doc.Paths["/v1/pkg/{id}"].Get
	if len(op.Security) != 1 {
		t.Fatalf("security: want 1 entry, got %d", len(op.Security))
	}
	if _, ok := op.Security[0]["apig-auth-custom"]; !ok {
		t.Errorf("custom security scheme not attached: %v", op.Security)
	}
}

func TestRender_AuthNoneOmitsSecurity(t *testing.T) {
	doc, err := Render(testContract(), testApigConfig())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	op := doc.Paths["/v1/pkg"].Get
	if len(op.Security) != 0 {
		t.Errorf("auth=none should omit security; got %v", op.Security)
	}
}

func TestRender_AuthRequiredButNoDefaultSchemeErrors(t *testing.T) {
	cfg := testApigConfig()
	cfg.Spec.Defaults.SecurityScheme = ""
	_, err := Render(testContract(), cfg)
	if err == nil || !strings.Contains(err.Error(), "securityScheme") {
		t.Fatalf("expected securityScheme error, got %v", err)
	}
}

func TestRender_OperationIDPrefix(t *testing.T) {
	doc, err := Render(testContract(), testApigConfig())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for path, item := range doc.Paths {
		for _, op := range []struct {
			method string
			op     any
		}{
			{"GET", item.Get},
		} {
			if op.op == nil {
				continue
			}
			oid := doc.Paths[path].Get.OperationID
			if !strings.HasPrefix(oid, "API_") {
				t.Errorf("%s %s: operationId %q missing API_ prefix", op.method, path, oid)
			}
		}
	}
}

func TestValidate_RejectsMissingPathParam(t *testing.T) {
	c := testContract()
	// declare path param but don't include it in OpenAPI parameters
	c.Spec.Routes[1].Parameters = nil
	// Render will succeed because contract validator is bypassed here;
	// We bypass contract validation by constructing Render input directly.
	doc, err := Render(c, testApigConfig())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	err = Validate(doc)
	if err == nil || !strings.Contains(err.Error(), "missing path parameter") {
		t.Fatalf("expected missing path parameter, got %v", err)
	}
}

func TestValidate_RejectsWrongOpenAPIVersion(t *testing.T) {
	doc, err := Render(testContract(), testApigConfig())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	doc.OpenAPI = "3.1.0"
	if err := Validate(doc); err == nil {
		t.Fatal("expected error for non-3.0.3 version")
	}
}
