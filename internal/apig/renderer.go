package apig

import (
	"fmt"
	"path"
	"strings"

	"gen-openapi/internal/config"
	"gen-openapi/pkg/contract"
	huawei "gen-openapi/pkg/huaweicloudapig"
)

func Render(c *contract.ApiContract, cfg *config.HuaweiApigConfig) (*huawei.OpenAPI, error) {
	doc := &huawei.OpenAPI{
		OpenAPI: "3.0.3",
		Info: huawei.Info{
			Description: c.Metadata.Description,
			Title:       c.Metadata.Title,
			Version:     c.Metadata.Version,
		},
		Servers: []huawei.Server{{URL: cfg.Spec.GatewayURL}},
		Paths:   huawei.Paths{},
		Components: huawei.Components{
			Responses: map[string]huawei.Response{
				"default": defaultResponse(),
			},
			SecuritySchemes: renderSecuritySchemes(cfg.Spec.SecuritySchemes),
			Schemas:         c.Spec.Schemas,
		},
	}

	for _, r := range c.Spec.Routes {
		op, err := renderOperation(c, cfg, r)
		if err != nil {
			return nil, err
		}
		item := doc.Paths[r.Path]
		if item == nil {
			item = &huawei.PathItem{}
			doc.Paths[r.Path] = item
		}
		setOperation(item, strings.ToUpper(r.Method), op)
	}

	return doc, nil
}

func renderOperation(c *contract.ApiContract, cfg *config.HuaweiApigConfig, r contract.Route) (*huawei.Operation, error) {
	backendPath := r.BackendPath
	if backendPath == "" {
		backendPath = path.Join(c.Spec.BasePath, r.Path)
		if !strings.HasPrefix(backendPath, "/") {
			backendPath = "/" + backendPath
		}
	}

	op := &huawei.Operation{
		OperationID: operationID(r.OperationID),
		Summary:     r.Summary,
		Description: r.Description,
		Parameters:  renderParameters(r.Parameters),
		Responses: map[string]huawei.Response{
			"default": defaultResponse(),
		},
		Servers:                     []huawei.Server{{URL: cfg.Spec.GatewayURL}},
		XApigatewayBackend:          renderBackend(cfg, r, backendPath),
		XApigatewayCors:             cfg.Spec.Defaults.Cors,
		XApigatewaySendFgBodyBase64: cfg.Spec.Defaults.SendFgBodyBase64,
		XApigatewayMatchMode:        cfg.Spec.Defaults.MatchMode,
		XApigatewayRequestType:      cfg.Spec.Defaults.RequestType,
	}

	if r.Auth != "" && r.Auth != "none" {
		scheme := cfg.Spec.Defaults.SecurityScheme
		if scheme == "" {
			return nil, fmt.Errorf("route %s requires auth but defaults.securityScheme is empty", r.OperationID)
		}
		op.Security = []map[string][]string{{scheme: {}}}
	}

	if r.RequestBody != nil && r.RequestBody.Schema != "" {
		op.RequestBody = &huawei.RequestBody{
			Required: r.RequestBody.Required,
			Content: map[string]huawei.MediaType{
				"application/json": {
					Schema: contract.Schema{Ref: "#/components/schemas/" + r.RequestBody.Schema},
				},
			},
		}
	}

	return op, nil
}

func renderBackend(cfg *config.HuaweiApigConfig, r contract.Route, backendPath string) *huawei.ApigatewayBackend {
	backend := &huawei.ApigatewayBackend{
		Parameters: renderBackendParameters(r.Parameters),
		Type:       cfg.Spec.Backend.Type,
	}
	if strings.EqualFold(cfg.Spec.Backend.Type, "FUNCTION") {
		backend.FunctionEndpoints = &huawei.FunctionEndpoint{
			AliasURN:       cfg.Spec.Backend.AliasURN,
			Description:    "",
			FunctionURN:    cfg.Spec.Backend.FunctionURN,
			InvocationType: cfg.Spec.Backend.InvocationType,
			NetworkType:    cfg.Spec.Backend.NetworkType,
			ReqProtocol:    cfg.Spec.Backend.ReqProtocol,
			Timeout:        cfg.Spec.Backend.Timeout,
			Version:        cfg.Spec.Backend.Version,
		}
		return backend
	}
	backend.HttpEndpoints = &huawei.HttpEndpoint{
		Address:         cfg.Spec.Backend.Address,
		Description:     "",
		EnableClientSSL: false,
		EnableSmChannel: false,
		Method:          strings.ToUpper(r.Method),
		Path:            backendPath,
		RetryCount:      cfg.Spec.Backend.RetryCount,
		Scheme:          cfg.Spec.Backend.Scheme,
		Timeout:         cfg.Spec.Backend.Timeout,
	}
	return backend
}

func operationID(id string) string {
	if strings.HasPrefix(id, "API_") {
		return id
	}
	return "API_" + id
}

func renderSecuritySchemes(in map[string]config.SecurityScheme) map[string]huawei.SecurityScheme {
	out := map[string]huawei.SecurityScheme{}
	for name, s := range in {
		v := huawei.SecurityScheme{
			In:                  s.In,
			Name:                s.Name,
			Type:                "apiKey",
			XApigatewayAuthType: s.Type,
		}
		if s.AppcodeAuthType != "" {
			v.XApigatewayAuthOpt = map[string]string{"appcode-auth-type": s.AppcodeAuthType}
		}
		if s.Authorizer != nil {
			v.XApigatewayAuthorizer = renderAuthorizer(s.Authorizer)
		}
		out[name] = v
	}
	return out
}

func renderAuthorizer(in *config.Authorizer) *huawei.Authorizer {
	out := &huawei.Authorizer{
		AuthDowngradeEnabled: in.AuthDowngradeEnabled,
		AuthorizerAliasURI:   in.AuthorizerAliasURI,
		AuthorizerType:       in.AuthorizerType,
		AuthorizerURI:        in.AuthorizerURI,
		AuthorizerVersion:    in.AuthorizerVersion,
		NeedBody:             in.NeedBody,
		NetworkType:          in.NetworkType,
		RetryAttempts:        in.RetryAttempts,
		Timeout:              in.Timeout,
		TTL:                  in.TTL,
		Type:                 in.Type,
	}
	if len(in.Identities) > 0 {
		out.Identities = make([]huawei.AuthorizerIdentity, 0, len(in.Identities))
		for _, identity := range in.Identities {
			out.Identities = append(out.Identities, huawei.AuthorizerIdentity{
				Location:   identity.Location,
				Name:       identity.Name,
				Validation: identity.Validation,
			})
		}
	}
	return out
}

func renderParameters(in []contract.Parameter) []huawei.Parameter {
	out := make([]huawei.Parameter, 0, len(in))
	for _, p := range in {
		out = append(out, huawei.Parameter{
			In:                        p.In,
			Name:                      p.Name,
			Required:                  p.Required || strings.EqualFold(p.In, "path"),
			Schema:                    huawei.ParameterSchema{Type: apigParameterType(p.Type)},
			XApigatewayOrchestrations: []any{},
			XApigatewayPassThrough:    "always",
		})
	}
	return out
}

func apigParameterType(string) string {
	// Huawei APIG validates backend mappings against request parameters by name.
	// Non-string query parameters can be rejected as missing req_params during
	// import, while HTTP forwards all query/path/header values as strings.
	return "string"
}

func renderBackendParameters(in []contract.Parameter) []huawei.BackendParameter {
	out := []huawei.BackendParameter{}
	for _, p := range in {
		location := strings.ToUpper(p.In)
		switch location {
		case "PATH", "QUERY", "HEADER":
			out = append(out, huawei.BackendParameter{
				In:     location,
				Name:   p.Name,
				Origin: "REQUEST",
				Value:  p.Name,
			})
		}
	}
	return out
}

func setOperation(item *huawei.PathItem, method string, op *huawei.Operation) {
	switch method {
	case "GET":
		item.Get = op
	case "POST":
		item.Post = op
	case "PUT":
		item.Put = op
	case "DELETE":
		item.Delete = op
	case "PATCH":
		item.Patch = op
	}
}

func defaultResponse() huawei.Response {
	return huawei.Response{
		Description:                    "response example",
		XApigatewayResultFailureSample: "",
		XApigatewayResultNormalSample:  "",
	}
}
