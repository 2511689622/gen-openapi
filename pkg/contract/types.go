package contract

type ApiContract struct {
	APIVersion string   `yaml:"apiVersion"`
	Kind       string   `yaml:"kind"`
	Metadata   Metadata `yaml:"metadata"`
	Spec       Spec     `yaml:"spec"`
}

type Metadata struct {
	Name        string `yaml:"name"`
	Title       string `yaml:"title"`
	Version     string `yaml:"version"`
	Description string `yaml:"description,omitempty"`
}

type Spec struct {
	BasePath string             `yaml:"basePath,omitempty"`
	Routes   []Route            `yaml:"routes"`
	Schemas  map[string]*Schema `yaml:"schemas,omitempty"`
}

type Route struct {
	OperationID string       `yaml:"operationId"`
	Method      string       `yaml:"method"`
	Path        string       `yaml:"path"`
	Summary     string       `yaml:"summary,omitempty"`
	Description string       `yaml:"description,omitempty"`
	Auth        string       `yaml:"auth,omitempty"`
	BackendPath string       `yaml:"backendPath,omitempty"`
	Parameters  []Parameter  `yaml:"parameters,omitempty"`
	RequestBody *RequestBody `yaml:"requestBody,omitempty"`
}

type Parameter struct {
	Name        string `yaml:"name"`
	In          string `yaml:"in"`
	Type        string `yaml:"type"`
	Required    bool   `yaml:"required,omitempty"`
	Description string `yaml:"description,omitempty"`
}

type RequestBody struct {
	Schema   string `yaml:"schema"`
	Required bool   `yaml:"required,omitempty"`
}

type Schema struct {
	Ref                  string             `yaml:"$ref,omitempty"`
	Type                 string             `yaml:"type,omitempty"`
	Format               string             `yaml:"format,omitempty"`
	Description          string             `yaml:"description,omitempty"`
	Required             []string           `yaml:"required,omitempty"`
	Properties           map[string]*Schema `yaml:"properties,omitempty"`
	Items                *Schema            `yaml:"items,omitempty"`
	AdditionalProperties any                `yaml:"additionalProperties,omitempty"`
	Enum                 []string           `yaml:"enum,omitempty"`
	Example              any                `yaml:"example,omitempty"`
}
