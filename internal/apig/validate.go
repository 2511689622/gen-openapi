package apig

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	huawei "gen-openapi/pkg/huaweicloudapig"
	"gopkg.in/yaml.v3"
)

var pathParamRe = regexp.MustCompile(`\{([A-Za-z0-9._-]+)\}`)

func ValidateFile(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var doc huawei.OpenAPI
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&doc); err != nil {
		return fmt.Errorf("load %s: %w", path, err)
	}

	return Validate(&doc)
}

func Validate(doc *huawei.OpenAPI) error {
	if doc.OpenAPI != "3.0.3" {
		return fmt.Errorf("openapi must be 3.0.3")
	}
	if len(doc.Paths) == 0 {
		return fmt.Errorf("paths is required")
	}

	paths := make([]string, 0, len(doc.Paths))
	for p := range doc.Paths {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	for _, p := range paths {
		for _, entry := range operations(doc.Paths[p]) {
			method, op := entry.method, entry.operation
			if op.OperationID == "" {
				return fmt.Errorf("%s %s missing operationId", method, p)
			}
			if op.XApigatewayBackend == nil {
				return fmt.Errorf("%s %s missing x-apigateway-backend", method, p)
			}
			if _, ok := op.Responses["default"]; !ok {
				return fmt.Errorf("%s %s missing responses.default", method, p)
			}
			if err := validatePathParams(p, method, op); err != nil {
				return err
			}
			for _, sec := range op.Security {
				for name := range sec {
					if _, ok := doc.Components.SecuritySchemes[name]; !ok {
						return fmt.Errorf("%s %s references unknown security scheme %s", method, p, name)
					}
				}
			}
		}
	}
	return nil
}

type operationEntry struct {
	method    string
	operation *huawei.Operation
}

func operations(item *huawei.PathItem) []operationEntry {
	ordered := []operationEntry{}
	if item.Get != nil {
		ordered = append(ordered, operationEntry{"GET", item.Get})
	}
	if item.Post != nil {
		ordered = append(ordered, operationEntry{"POST", item.Post})
	}
	if item.Put != nil {
		ordered = append(ordered, operationEntry{"PUT", item.Put})
	}
	if item.Delete != nil {
		ordered = append(ordered, operationEntry{"DELETE", item.Delete})
	}
	if item.Patch != nil {
		ordered = append(ordered, operationEntry{"PATCH", item.Patch})
	}
	return ordered
}

func validatePathParams(p, method string, op *huawei.Operation) error {
	matches := pathParamRe.FindAllStringSubmatch(p, -1)
	for _, match := range matches {
		name := match[1]
		if !hasOpenAPIPathParam(op.Parameters, name) {
			return fmt.Errorf("%s %s missing path parameter %s", method, p, name)
		}
		if !hasBackendPathParam(op.XApigatewayBackend.Parameters, name) {
			return fmt.Errorf("%s %s missing backend path parameter mapping %s", method, p, name)
		}
	}
	return nil
}

func hasOpenAPIPathParam(params []huawei.Parameter, name string) bool {
	for _, p := range params {
		if p.Name == name && strings.EqualFold(p.In, "path") && p.Required {
			return true
		}
	}
	return false
}

func hasBackendPathParam(params []huawei.BackendParameter, name string) bool {
	for _, p := range params {
		if p.Name == name && p.Value == name && strings.EqualFold(p.In, "PATH") {
			return true
		}
	}
	return false
}
