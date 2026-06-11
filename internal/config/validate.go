package config

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"gen-openapi/pkg/contract"
)

// skKeyRe matches OpenAI-style or generic sk-... secret keys.
var skKeyRe = regexp.MustCompile(`(?i)\bsk-[A-Za-z0-9_-]{6,}`)

// awsKeyRe matches AWS access key IDs in field values.
var awsKeyRe = regexp.MustCompile(`AKIA[0-9A-Z]{16}`)

// credentialWords are words that, when appearing as a whole word in a
// string field value, suggest a credential leak.
var credentialWords = []string{
	"password",
	"secret",
	"token",
}

var contractPathParamRe = regexp.MustCompile(`\{([A-Za-z0-9._-]+)\}`)

func ValidateContract(c *contract.ApiContract) error {
	if c.Kind != "ApiContract" {
		return fmt.Errorf("contract kind must be ApiContract")
	}
	if c.Metadata.Name == "" {
		return fmt.Errorf("contract metadata.name is required")
	}
	if c.Metadata.Title == "" {
		return fmt.Errorf("contract metadata.title is required")
	}
	if c.Metadata.Version == "" {
		return fmt.Errorf("contract metadata.version is required")
	}
	if len(c.Spec.Routes) == 0 {
		return fmt.Errorf("contract spec.routes is required")
	}

	seen := map[string]bool{}
	for i, r := range c.Spec.Routes {
		if r.OperationID == "" || r.Method == "" || r.Path == "" {
			return fmt.Errorf("route %d requires operationId, method, and path", i)
		}
		method := strings.ToUpper(r.Method)
		switch method {
		case "GET", "POST", "PUT", "DELETE", "PATCH":
		default:
			return fmt.Errorf("route %s uses unsupported method %s", r.OperationID, r.Method)
		}
		key := method + " " + r.Path
		if seen[key] {
			return fmt.Errorf("duplicate route %s", key)
		}
		seen[key] = true
		if err := validateRoutePathParameters(r); err != nil {
			return err
		}
		for _, p := range r.Parameters {
			if p.Name == "" || p.In == "" || p.Type == "" {
				return fmt.Errorf("route %s has invalid parameter", r.OperationID)
			}
		}
	}

	return nil
}

func validateRoutePathParameters(r contract.Route) error {
	declared := map[string]bool{}
	for _, p := range r.Parameters {
		if strings.EqualFold(p.In, "path") {
			declared[p.Name] = true
		}
	}

	for _, match := range contractPathParamRe.FindAllStringSubmatch(r.Path, -1) {
		name := match[1]
		if !declared[name] {
			return fmt.Errorf("route %s path parameter %s is not declared", r.OperationID, name)
		}
	}

	for name := range declared {
		if !strings.Contains(r.Path, "{"+name+"}") {
			return fmt.Errorf("route %s declares unused path parameter %s", r.OperationID, name)
		}
	}

	return nil
}

func ValidateApigConfig(c *HuaweiApigConfig) error {
	if c.Kind != "HuaweiApigConfig" {
		return fmt.Errorf("apig config kind must be HuaweiApigConfig")
	}
	if c.Spec.GatewayURL == "" {
		return fmt.Errorf("apig spec.gatewayUrl is required")
	}
	if c.Spec.Backend.Type == "" {
		c.Spec.Backend.Type = "HTTP"
	}
	c.Spec.Backend.Type = strings.ToUpper(c.Spec.Backend.Type)
	if c.Spec.Backend.Timeout == 0 {
		c.Spec.Backend.Timeout = 5000
	}
	if err := validateBackend(&c.Spec.Backend); err != nil {
		return err
	}
	if c.Spec.Defaults.MatchMode == "" {
		c.Spec.Defaults.MatchMode = "NORMAL"
	}
	if c.Spec.Defaults.RequestType == "" {
		c.Spec.Defaults.RequestType = "public"
	}
	for name, scheme := range c.Spec.SecuritySchemes {
		if strings.EqualFold(scheme.Type, "AUTHORIZER") {
			if err := validateAuthorizerScheme(name, scheme.Authorizer); err != nil {
				return err
			}
		}
	}

	// Credential leak check: walk all string fields recursively to catch
	// passwords, tokens, and API keys that should not be in apig-config.yaml.
	if err := scanCredentials("", reflect.ValueOf(c)); err != nil {
		return err
	}

	return nil
}

func ValidateImportTarget(t *ImportTarget) error {
	if t == nil {
		return fmt.Errorf("apig spec.importTarget is required for apig-import")
	}
	if t.Region == "" {
		return fmt.Errorf("apig importTarget.region is required")
	}
	if t.ProjectID == "" {
		return fmt.Errorf("apig importTarget.projectId is required")
	}
	if t.InstanceID == "" {
		return fmt.Errorf("apig importTarget.instanceId is required")
	}
	if !t.IsCreateGroup && t.GroupID == "" {
		return fmt.Errorf("apig importTarget.groupId is required when isCreateGroup is false")
	}
	if t.APIMode == "" {
		t.APIMode = "merge"
	}
	if t.ExtendMode == "" {
		t.ExtendMode = "merge"
	}
	if err := validateImportMode("apiMode", t.APIMode); err != nil {
		return err
	}
	if err := validateImportMode("extendMode", t.ExtendMode); err != nil {
		return err
	}
	return nil
}

func validateImportMode(name, value string) error {
	switch value {
	case "merge", "override":
		return nil
	default:
		return fmt.Errorf("apig importTarget.%s must be merge or override", name)
	}
}

func validateBackend(b *Backend) error {
	switch b.Type {
	case "HTTP":
		if b.Address == "" || b.Scheme == "" {
			return fmt.Errorf("apig backend.address and backend.scheme are required")
		}
		if b.RetryCount == "" {
			b.RetryCount = "0"
		}
	case "FUNCTION":
		if b.FunctionURN == "" {
			return fmt.Errorf("apig backend.functionUrn is required for FUNCTION backend")
		}
		if b.InvocationType == "" {
			b.InvocationType = "sync"
		}
		if b.NetworkType == "" {
			b.NetworkType = "V1"
		}
		if b.ReqProtocol == "" {
			b.ReqProtocol = "HTTPS"
		}
		if b.Version == "" {
			b.Version = "latest"
		}
	default:
		return fmt.Errorf("unsupported apig backend.type %s", b.Type)
	}
	return nil
}

func validateAuthorizerScheme(name string, a *Authorizer) error {
	if a == nil {
		return fmt.Errorf("security scheme %s type AUTHORIZER requires authorizer", name)
	}
	if a.AuthorizerType == "" {
		return fmt.Errorf("security scheme %s authorizer.authorizer_type is required", name)
	}
	if a.AuthorizerURI == "" {
		return fmt.Errorf("security scheme %s authorizer.authorizer_uri is required", name)
	}
	if a.AuthorizerVersion == "" {
		return fmt.Errorf("security scheme %s authorizer.authorizer_version is required", name)
	}
	if a.NetworkType == "" {
		return fmt.Errorf("security scheme %s authorizer.network_type is required", name)
	}
	if a.Timeout <= 0 {
		return fmt.Errorf("security scheme %s authorizer.timeout must be positive", name)
	}
	if a.Type == "" {
		return fmt.Errorf("security scheme %s authorizer.type is required", name)
	}
	return nil
}

// scanCredentials recursively walks a reflect.Value tree and checks every
// string field against known credential patterns.
func scanCredentials(path string, v reflect.Value) error {
	switch v.Kind() {
	case reflect.Ptr:
		if !v.IsNil() {
			return scanCredentials(path, v.Elem())
		}
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			f := t.Field(i)
			fieldPath := f.Name
			if path != "" {
				fieldPath = path + "." + fieldPath
			}
			if err := scanCredentials(fieldPath, v.Field(i)); err != nil {
				return err
			}
		}
	case reflect.Map:
		for _, key := range v.MapKeys() {
			keyStr := fmt.Sprintf("%v", key.Interface())
			fieldPath := path + "[" + keyStr + "]"
			if err := scanCredentials(fieldPath, v.MapIndex(key)); err != nil {
				return err
			}
		}
	case reflect.String:
		s := v.String()
		switch {
		case awsKeyRe.MatchString(s):
			return fmt.Errorf("apig config field %q contains an AWS access key (AKIA...)", path)
		case skKeyRe.MatchString(s):
			return fmt.Errorf("apig config field %q contains an sk-... secret key", path)
		default:
			lower := strings.ToLower(s)
			for _, word := range credentialWords {
				if strings.Contains(lower, word) {
					return fmt.Errorf("apig config field %q may contain a credential (%q found in value)", path, word)
				}
			}
		}
	}
	return nil
}
