package config

import (
	"strings"
	"testing"

	"gen-openapi/pkg/contract"
)

func validContract() *contract.ApiContract {
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
				},
			},
		},
	}
}

func TestValidateContract_Valid(t *testing.T) {
	if err := ValidateContract(validContract()); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidateContract_KindWrong(t *testing.T) {
	c := validContract()
	c.Kind = "WrongKind"
	if err := ValidateContract(c); err == nil {
		t.Fatal("expected error for wrong kind")
	}
}

func TestValidateContract_MissingMetadata(t *testing.T) {
	cases := map[string]func(*contract.ApiContract){
		"name":    func(c *contract.ApiContract) { c.Metadata.Name = "" },
		"title":   func(c *contract.ApiContract) { c.Metadata.Title = "" },
		"version": func(c *contract.ApiContract) { c.Metadata.Version = "" },
	}
	for name, mut := range cases {
		c := validContract()
		mut(c)
		if err := ValidateContract(c); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestValidateContract_NoRoutes(t *testing.T) {
	c := validContract()
	c.Spec.Routes = nil
	if err := ValidateContract(c); err == nil {
		t.Fatal("expected error for empty routes")
	}
}

func TestValidateContract_UnsupportedMethod(t *testing.T) {
	c := validContract()
	c.Spec.Routes[0].Method = "OPTIONS"
	err := ValidateContract(c)
	if err == nil || !strings.Contains(err.Error(), "unsupported method") {
		t.Fatalf("expected unsupported method error, got %v", err)
	}
}

func TestValidateContract_DuplicateRoute(t *testing.T) {
	c := validContract()
	c.Spec.Routes = append(c.Spec.Routes, c.Spec.Routes[0])
	err := ValidateContract(c)
	if err == nil || !strings.Contains(err.Error(), "duplicate route") {
		t.Fatalf("expected duplicate route error, got %v", err)
	}
}

func TestValidateContract_PathParamDeclaredMatchesPlaceholder(t *testing.T) {
	c := validContract()
	c.Spec.Routes[0].Path = "/v1/pkg/{id}"
	c.Spec.Routes[0].Parameters = []contract.Parameter{
		{Name: "id", In: "path", Type: "string", Required: true},
	}
	if err := ValidateContract(c); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidateContract_PathParamMissingDeclaration(t *testing.T) {
	c := validContract()
	c.Spec.Routes[0].Path = "/v1/pkg/{id}"
	err := ValidateContract(c)
	if err == nil || !strings.Contains(err.Error(), "not declared") {
		t.Fatalf("expected 'not declared' error, got %v", err)
	}
}

func TestValidateContract_PathParamUnusedDeclaration(t *testing.T) {
	c := validContract()
	c.Spec.Routes[0].Parameters = []contract.Parameter{
		{Name: "missing", In: "path", Type: "string"},
	}
	err := ValidateContract(c)
	if err == nil || !strings.Contains(err.Error(), "unused") {
		t.Fatalf("expected 'unused' error, got %v", err)
	}
}

func TestValidateApigConfig_DefaultsApplied(t *testing.T) {
	c := &HuaweiApigConfig{
		Kind: "HuaweiApigConfig",
		Spec: HuaweiSpec{
			GatewayURL: "https://example.huaweicloudapis.com",
			Backend: Backend{
				Scheme:  "https",
				Address: "backend.example.com",
			},
		},
	}
	if err := ValidateApigConfig(c); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if c.Spec.Backend.Type != "HTTP" {
		t.Errorf("backend.type default not applied: %q", c.Spec.Backend.Type)
	}
	if c.Spec.Backend.Timeout != 5000 {
		t.Errorf("backend.timeout default not applied: %d", c.Spec.Backend.Timeout)
	}
	if c.Spec.Backend.RetryCount != "0" {
		t.Errorf("backend.retryCount default not applied: %q", c.Spec.Backend.RetryCount)
	}
	if c.Spec.Defaults.MatchMode != "NORMAL" {
		t.Errorf("defaults.matchMode default not applied: %q", c.Spec.Defaults.MatchMode)
	}
	if c.Spec.Defaults.RequestType != "public" {
		t.Errorf("defaults.requestType default not applied: %q", c.Spec.Defaults.RequestType)
	}
}

func TestValidateApigConfig_RequiresGatewayURL(t *testing.T) {
	c := &HuaweiApigConfig{
		Kind: "HuaweiApigConfig",
		Spec: HuaweiSpec{
			Backend: Backend{Scheme: "https", Address: "x"},
		},
	}
	if err := ValidateApigConfig(c); err == nil {
		t.Fatal("expected error for missing gatewayUrl")
	}
}

func TestValidateApigConfig_RejectsCredentials(t *testing.T) {
	c := &HuaweiApigConfig{
		Kind: "HuaweiApigConfig",
		Spec: HuaweiSpec{
			GatewayURL: "https://example.com",
			Backend:    Backend{Scheme: "https", Address: "backend.example.com"},
			SecuritySchemes: map[string]SecurityScheme{
				"my-scheme": {
					Type:            "AppSigv1",
					In:              "header",
					Name:            "Authorization",
					AppcodeAuthType: "header",
				},
			},
		},
	}

	t.Run("no credentials should pass", func(t *testing.T) {
		if err := ValidateApigConfig(c); err != nil {
			t.Fatalf("expected nil for clean config, got %v", err)
		}
	})

	t.Run("gatewayUrl with password should fail", func(t *testing.T) {
		bad := *c
		bad.Spec.GatewayURL = "https://user:password123@example.com"
		if err := ValidateApigConfig(&bad); err == nil {
			t.Fatal("expected error for password in URL")
		}
	})

	t.Run("secret in field should fail", func(t *testing.T) {
		bad := *c
		bad.Spec.Backend.Address = "my-secret-token.example.com"
		if err := ValidateApigConfig(&bad); err == nil {
			t.Fatal("expected error for secret in address")
		}
	})

	t.Run("AKIA key should fail", func(t *testing.T) {
		bad := *c
		bad.Spec.GatewayURL = "https://AKIAIOSFODNN7EXAMPLE.s3.amazonaws.com"
		if err := ValidateApigConfig(&bad); err == nil {
			t.Fatal("expected error for AKIA key")
		}
	})

	t.Run("sk- prefix should fail", func(t *testing.T) {
		bad := *c
		bad.Spec.GatewayURL = "https://user:sk-abc123def456@example.com"
		if err := ValidateApigConfig(&bad); err == nil {
			t.Fatal("expected error for sk- key")
		}
	})
}

func TestValidateApigConfig_RequiresBackendAddressAndScheme(t *testing.T) {
	c := &HuaweiApigConfig{
		Kind: "HuaweiApigConfig",
		Spec: HuaweiSpec{
			GatewayURL: "https://example.com",
			Backend:    Backend{},
		},
	}
	if err := ValidateApigConfig(c); err == nil {
		t.Fatal("expected error for missing backend.address/scheme")
	}
}

func TestValidateApigConfig_RejectsAuthorizerWithoutConfig(t *testing.T) {
	c := &HuaweiApigConfig{
		Kind: "HuaweiApigConfig",
		Spec: HuaweiSpec{
			GatewayURL: "https://example.com",
			Backend: Backend{
				Scheme:  "https",
				Address: "backend.example.com",
			},
			SecuritySchemes: map[string]SecurityScheme{
				"apig-auth-custom": {
					Type: "AUTHORIZER",
					In:   "header",
					Name: "unused",
				},
			},
		},
	}
	if err := ValidateApigConfig(c); err == nil || !strings.Contains(err.Error(), "requires authorizer") {
		t.Fatalf("expected missing authorizer error, got %v", err)
	}
}

func TestValidateApigConfig_AllowsAuthorizerConfig(t *testing.T) {
	c := &HuaweiApigConfig{
		Kind: "HuaweiApigConfig",
		Spec: HuaweiSpec{
			GatewayURL: "https://example.com",
			Backend: Backend{
				Scheme:  "https",
				Address: "backend.example.com",
			},
			SecuritySchemes: map[string]SecurityScheme{
				"apig-auth-custom": {
					Type: "AUTHORIZER",
					In:   "header",
					Name: "unused",
					Authorizer: &Authorizer{
						AuthorizerType:    "FUNC",
						AuthorizerURI:     "urn:fss:ap-southeast-1:00000000000000000000000000000000:function:default:custom-auth",
						AuthorizerVersion: "latest",
						NetworkType:       "V1",
						Timeout:           30000,
						Type:              "FRONTEND",
					},
				},
			},
		},
	}
	if err := ValidateApigConfig(c); err != nil {
		t.Fatalf("expected valid authorizer config, got %v", err)
	}
}

func TestValidateImportTarget_Defaults(t *testing.T) {
	target := &ImportTarget{
		Region:     "cn-north-4",
		ProjectID:  "project-1",
		InstanceID: "instance-1",
		GroupID:    "group-1",
	}
	if err := ValidateImportTarget(target); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if target.APIMode != "merge" {
		t.Fatalf("apiMode default = %q", target.APIMode)
	}
	if target.ExtendMode != "merge" {
		t.Fatalf("extendMode default = %q", target.ExtendMode)
	}
}

func TestValidateImportTarget_RequiresTarget(t *testing.T) {
	if err := ValidateImportTarget(nil); err == nil {
		t.Fatal("expected nil target error")
	}
}

func TestValidateImportTarget_RejectsBadMode(t *testing.T) {
	target := &ImportTarget{
		Region:     "cn-north-4",
		ProjectID:  "project-1",
		InstanceID: "instance-1",
		GroupID:    "group-1",
		APIMode:    "replace",
		ExtendMode: "merge",
	}
	if err := ValidateImportTarget(target); err == nil {
		t.Fatal("expected bad apiMode error")
	}
}
