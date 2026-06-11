package config

type HuaweiApigConfig struct {
	APIVersion string     `yaml:"apiVersion"`
	Kind       string     `yaml:"kind"`
	Metadata   Metadata   `yaml:"metadata"`
	Spec       HuaweiSpec `yaml:"spec"`
}

type Metadata struct {
	Name string `yaml:"name"`
}

type HuaweiSpec struct {
	GatewayURL      string                    `yaml:"gatewayUrl"`
	Backend         Backend                   `yaml:"backend"`
	Defaults        Defaults                  `yaml:"defaults"`
	SecuritySchemes map[string]SecurityScheme `yaml:"securitySchemes"`
	ImportTarget    *ImportTarget             `yaml:"importTarget,omitempty"`
}

// ImportTarget describes the Huawei Cloud APIG destination used by apig-import.
// It intentionally contains IDs and import modes only; credentials must come
// from the Huawei SDK environment variables, not from YAML.
type ImportTarget struct {
	Region        string `yaml:"region"`
	ProjectID     string `yaml:"projectId"`
	InstanceID    string `yaml:"instanceId"`
	GroupID       string `yaml:"groupId"`
	APIMode       string `yaml:"apiMode,omitempty"`
	ExtendMode    string `yaml:"extendMode,omitempty"`
	SimpleMode    bool   `yaml:"simpleMode,omitempty"`
	MockMode      bool   `yaml:"mockMode,omitempty"`
	IsCreateGroup bool   `yaml:"isCreateGroup,omitempty"`
}

type Backend struct {
	Type           string `yaml:"type"`
	Scheme         string `yaml:"scheme,omitempty"`
	Address        string `yaml:"address,omitempty"`
	Timeout        int    `yaml:"timeout"`
	RetryCount     string `yaml:"retryCount,omitempty"`
	AliasURN       string `yaml:"aliasUrn,omitempty"`
	FunctionURN    string `yaml:"functionUrn,omitempty"`
	InvocationType string `yaml:"invocationType,omitempty"`
	NetworkType    string `yaml:"networkType,omitempty"`
	ReqProtocol    string `yaml:"reqProtocol,omitempty"`
	Version        string `yaml:"version,omitempty"`
}

type Defaults struct {
	Cors             bool   `yaml:"cors"`
	SendFgBodyBase64 bool   `yaml:"sendFgBodyBase64"`
	MatchMode        string `yaml:"matchMode"`
	RequestType      string `yaml:"requestType"`
	SecurityScheme   string `yaml:"securityScheme"`
}

type SecurityScheme struct {
	Type            string      `yaml:"type"`
	In              string      `yaml:"in"`
	Name            string      `yaml:"name"`
	AppcodeAuthType string      `yaml:"appcodeAuthType,omitempty"`
	Authorizer      *Authorizer `yaml:"authorizer,omitempty"`
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
