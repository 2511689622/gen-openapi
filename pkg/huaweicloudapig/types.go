package huaweicloudapig

import "gen-openapi/pkg/contract"

type OpenAPI struct {
	OpenAPI    string     `yaml:"openapi"`
	Info       Info       `yaml:"info"`
	Servers    []Server   `yaml:"servers,omitempty"`
	Paths      Paths      `yaml:"paths"`
	Components Components `yaml:"components,omitempty"`
}

type Info struct {
	Description string `yaml:"description,omitempty"`
	Title       string `yaml:"title"`
	Version     string `yaml:"version"`
}

type Server struct {
	URL         string `yaml:"url"`
	Description string `yaml:"description,omitempty"`
}

type Paths map[string]*PathItem

type PathItem struct {
	Get    *Operation `yaml:"get,omitempty"`
	Post   *Operation `yaml:"post,omitempty"`
	Put    *Operation `yaml:"put,omitempty"`
	Delete *Operation `yaml:"delete,omitempty"`
	Patch  *Operation `yaml:"patch,omitempty"`
}

type Operation struct {
	OperationID                 string                `yaml:"operationId"`
	Summary                     string                `yaml:"summary,omitempty"`
	Description                 string                `yaml:"description,omitempty"`
	Parameters                  []Parameter           `yaml:"parameters,omitempty"`
	RequestBody                 *RequestBody          `yaml:"requestBody,omitempty"`
	Responses                   map[string]Response   `yaml:"responses"`
	Security                    []map[string][]string `yaml:"security,omitempty"`
	Servers                     []Server              `yaml:"servers,omitempty"`
	XApigatewayBackend          *ApigatewayBackend    `yaml:"x-apigateway-backend,omitempty"`
	XApigatewayCors             bool                  `yaml:"x-apigateway-cors"`
	XApigatewaySendFgBodyBase64 bool                  `yaml:"x-apigateway-is-send-fg-body-base64"`
	XApigatewayMatchMode        string                `yaml:"x-apigateway-match-mode"`
	XApigatewayRequestType      string                `yaml:"x-apigateway-request-type"`
}

type Parameter struct {
	In                        string          `yaml:"in"`
	Name                      string          `yaml:"name"`
	Required                  bool            `yaml:"required,omitempty"`
	Schema                    ParameterSchema `yaml:"schema"`
	XApigatewayOrchestrations []any           `yaml:"x-apigateway-orchestrations"`
	XApigatewayPassThrough    string          `yaml:"x-apigateway-pass-through"`
}

type ParameterSchema struct {
	Type string `yaml:"type"`
}

type RequestBody struct {
	Content  map[string]MediaType `yaml:"content"`
	Required bool                 `yaml:"required,omitempty"`
}

type MediaType struct {
	Schema contract.Schema `yaml:"schema"`
}

type Response struct {
	Description                    string `yaml:"description"`
	XApigatewayResultFailureSample string `yaml:"x-apigateway-result-failure-sample"`
	XApigatewayResultNormalSample  string `yaml:"x-apigateway-result-normal-sample"`
}

type Components struct {
	Responses       map[string]Response         `yaml:"responses,omitempty"`
	SecuritySchemes map[string]SecurityScheme   `yaml:"securitySchemes,omitempty"`
	Schemas         map[string]*contract.Schema `yaml:"schemas,omitempty"`
}

type SecurityScheme struct {
	In                    string            `yaml:"in"`
	Name                  string            `yaml:"name"`
	Type                  string            `yaml:"type"`
	XApigatewayAuthOpt    map[string]string `yaml:"x-apigateway-auth-opt,omitempty"`
	XApigatewayAuthType   string            `yaml:"x-apigateway-auth-type"`
	XApigatewayAuthorizer *Authorizer       `yaml:"x-apigateway-authorizer,omitempty"`
}

type Authorizer struct {
	AuthDowngradeEnabled bool                 `yaml:"auth_downgrade_enabled"`
	AuthorizerAliasURI   string               `yaml:"authorizer_alias_uri"`
	AuthorizerType       string               `yaml:"authorizer_type"`
	AuthorizerURI        string               `yaml:"authorizer_uri"`
	AuthorizerVersion    string               `yaml:"authorizer_version"`
	Identities           []AuthorizerIdentity `yaml:"identities,omitempty"`
	NeedBody             bool                 `yaml:"need_body"`
	NetworkType          string               `yaml:"network_type"`
	RetryAttempts        int                  `yaml:"retry_attempts"`
	Timeout              int                  `yaml:"timeout"`
	TTL                  int                  `yaml:"ttl"`
	Type                 string               `yaml:"type"`
}

type AuthorizerIdentity struct {
	Location   string `yaml:"location"`
	Name       string `yaml:"name"`
	Validation string `yaml:"validation"`
}

type ApigatewayBackend struct {
	FunctionEndpoints *FunctionEndpoint  `yaml:"functionEndpoints,omitempty"`
	HttpEndpoints     *HttpEndpoint      `yaml:"httpEndpoints,omitempty"`
	Parameters        []BackendParameter `yaml:"parameters,omitempty"`
	Type              string             `yaml:"type"`
}

type FunctionEndpoint struct {
	AliasURN       string `yaml:"alias-urn"`
	Description    string `yaml:"description"`
	FunctionURN    string `yaml:"function-urn"`
	InvocationType string `yaml:"invocation-type"`
	NetworkType    string `yaml:"network-type"`
	ReqProtocol    string `yaml:"req_protocol"`
	Timeout        int    `yaml:"timeout"`
	Version        string `yaml:"version"`
}

type HttpEndpoint struct {
	Address         string `yaml:"address"`
	Description     string `yaml:"description"`
	EnableClientSSL bool   `yaml:"enableClientSsl"`
	EnableSmChannel bool   `yaml:"enableSmChannel"`
	Method          string `yaml:"method"`
	Path            string `yaml:"path"`
	RetryCount      string `yaml:"retryCount"`
	Scheme          string `yaml:"scheme"`
	Timeout         int    `yaml:"timeout"`
}

type BackendParameter struct {
	In     string `yaml:"in"`
	Name   string `yaml:"name"`
	Origin string `yaml:"origin"`
	Value  string `yaml:"value"`
}
