package openapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"gen-openapi/pkg/contract"
	"gopkg.in/yaml.v3"
)

// openAPIDoc represents the minimal OpenAPI 3.x doc we need to read.
type openAPIDoc struct {
	OpenAPI    string                 `yaml:"openapi" json:"openapi"`
	Info       openAPIInfo            `yaml:"info" json:"info"`
	Paths      map[string]openAPIPath `yaml:"paths" json:"paths"`
	Components openAPIComponents      `yaml:"components,omitempty" json:"components,omitempty"`
}

type openAPIInfo struct {
	Title       string `yaml:"title" json:"title"`
	Description string `yaml:"description" json:"description"`
	Version     string `yaml:"version" json:"version"`
}

type openAPIPath struct {
	Get    *openAPIOperation `yaml:"get,omitempty" json:"get,omitempty"`
	Post   *openAPIOperation `yaml:"post,omitempty" json:"post,omitempty"`
	Put    *openAPIOperation `yaml:"put,omitempty" json:"put,omitempty"`
	Delete *openAPIOperation `yaml:"delete,omitempty" json:"delete,omitempty"`
	Patch  *openAPIOperation `yaml:"patch,omitempty" json:"patch,omitempty"`
}

type openAPIOperation struct {
	Summary     string              `yaml:"summary" json:"summary"`
	Description string              `yaml:"description" json:"description"`
	OperationID string              `yaml:"operationId" json:"operationId"`
	Tags        []string            `yaml:"tags" json:"tags"`
	Parameters  []openAPIParameter  `yaml:"parameters" json:"parameters"`
	RequestBody *openAPIRequestBody `yaml:"requestBody,omitempty" json:"requestBody,omitempty"`
}

type openAPIParameter struct {
	Name        string         `yaml:"name" json:"name"`
	In          string         `yaml:"in" json:"in"`
	Description string         `yaml:"description" json:"description"`
	Required    bool           `yaml:"required" json:"required"`
	Schema      *openAPISchema `yaml:"schema" json:"schema"`
}

type openAPIRequestBody struct {
	Required bool                        `yaml:"required" json:"required"`
	Content  map[string]openAPIMediaType `yaml:"content" json:"content"`
}

type openAPIMediaType struct {
	Schema *openAPISchema `yaml:"schema" json:"schema"`
}

type openAPIComponents struct {
	Schemas map[string]openAPISchema `yaml:"schemas" json:"schemas"`
}

type openAPISchema struct {
	Ref                  string                    `yaml:"$ref,omitempty" json:"$ref,omitempty"`
	Type                 string                    `yaml:"type,omitempty" json:"type,omitempty"`
	Format               string                    `yaml:"format,omitempty" json:"format,omitempty"`
	Description          string                    `yaml:"description,omitempty" json:"description,omitempty"`
	Required             []string                  `yaml:"required,omitempty" json:"required,omitempty"`
	Properties           map[string]*openAPISchema `yaml:"properties,omitempty" json:"properties,omitempty"`
	Items                *openAPISchema            `yaml:"items,omitempty" json:"items,omitempty"`
	AdditionalProperties any                       `yaml:"additionalProperties,omitempty" json:"additionalProperties,omitempty"`
	Enum                 []string                  `yaml:"enum,omitempty" json:"enum,omitempty"`
}

// ImportOptions controls how the OpenAPI document is translated.
type ImportOptions struct {
	ServiceName string
	BasePath    string
	DefaultAuth string
}

// FromFile reads an OpenAPI 3.x YAML/JSON file and produces an ApiContract draft.
func FromFile(path string, opts ImportOptions) (*contract.ApiContract, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return fromBytes(b, opts)
}

// FromURL fetches an OpenAPI JSON doc from a URL (springdoc /v3/api-docs or
// FastAPI /openapi.json) and produces an ApiContract draft.
func FromURL(url string, opts ImportOptions) (*contract.ApiContract, error) {
	return FromURLContext(context.Background(), url, opts)
}

// FromURLContext is like FromURL but binds the HTTP request to ctx so CLI
// cancellation can stop runtime OpenAPI discovery promptly.
func FromURLContext(ctx context.Context, url string, opts ImportOptions) (*contract.ApiContract, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request %s: %w", url, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", url, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s returned HTTP %d", url, resp.StatusCode)
	}
	return fromBytes(body, opts)
}

func fromBytes(b []byte, opts ImportOptions) (*contract.ApiContract, error) {
	var doc openAPIDoc

	// Try JSON first since springdoc and FastAPI both return JSON; fall back to YAML.
	if err := json.Unmarshal(b, &doc); err != nil {
		dec := yaml.NewDecoder(bytes.NewReader(b))
		if err := dec.Decode(&doc); err != nil {
			return nil, fmt.Errorf("parse openapi document: %w", err)
		}
	}
	if doc.OpenAPI == "" {
		return nil, fmt.Errorf("document missing openapi version field")
	}

	c := &contract.ApiContract{
		APIVersion: "infra.example.com/v1",
		Kind:       "ApiContract",
		Metadata: contract.Metadata{
			Name:        opts.ServiceName,
			Title:       firstNonEmpty(doc.Info.Title, opts.ServiceName),
			Version:     firstNonEmpty(doc.Info.Version, "1.0.0"),
			Description: doc.Info.Description,
		},
		Spec: contract.Spec{
			BasePath: opts.BasePath,
			Schemas:  map[string]*contract.Schema{},
		},
	}

	for name, s := range doc.Components.Schemas {
		c.Spec.Schemas[name] = convertSchema(&s)
	}

	pathNames := make([]string, 0, len(doc.Paths))
	for p := range doc.Paths {
		pathNames = append(pathNames, p)
	}
	sort.Strings(pathNames)

	for _, p := range pathNames {
		item := doc.Paths[p]
		for _, m := range methodsInOrder(item) {
			c.Spec.Routes = append(c.Spec.Routes, convertOperation(p, m.method, m.operation, opts))
		}
	}

	return c, nil
}

type methodEntry struct {
	method    string
	operation *openAPIOperation
}

func methodsInOrder(item openAPIPath) []methodEntry {
	out := []methodEntry{}
	if item.Get != nil {
		out = append(out, methodEntry{"GET", item.Get})
	}
	if item.Post != nil {
		out = append(out, methodEntry{"POST", item.Post})
	}
	if item.Put != nil {
		out = append(out, methodEntry{"PUT", item.Put})
	}
	if item.Delete != nil {
		out = append(out, methodEntry{"DELETE", item.Delete})
	}
	if item.Patch != nil {
		out = append(out, methodEntry{"PATCH", item.Patch})
	}
	return out
}

func convertOperation(path, method string, op *openAPIOperation, opts ImportOptions) contract.Route {
	r := contract.Route{
		OperationID: firstNonEmpty(op.OperationID, fallbackOperationID(method, path)),
		Method:      method,
		Path:        path,
		Summary:     op.Summary,
		Description: op.Description,
		Auth:        opts.DefaultAuth,
	}

	for _, p := range op.Parameters {
		typ := "string"
		if p.Schema != nil && p.Schema.Type != "" {
			typ = p.Schema.Type
		}
		switch strings.ToLower(p.In) {
		case "path":
			r.Parameters = append(r.Parameters, contract.Parameter{
				Name:        p.Name,
				In:          "path",
				Type:        typ,
				Required:    true,
				Description: p.Description,
			})
		case "query", "header":
			r.Parameters = append(r.Parameters, contract.Parameter{
				Name:        p.Name,
				In:          strings.ToLower(p.In),
				Type:        typ,
				Required:    p.Required,
				Description: p.Description,
			})
		}
	}

	if op.RequestBody != nil {
		if media, ok := op.RequestBody.Content["application/json"]; ok && media.Schema != nil && media.Schema.Ref != "" {
			r.RequestBody = &contract.RequestBody{
				Schema:   refName(media.Schema.Ref),
				Required: op.RequestBody.Required,
			}
		}
	}

	return r
}

func convertSchema(s *openAPISchema) *contract.Schema {
	if s == nil {
		return nil
	}
	out := &contract.Schema{
		Type:                 s.Type,
		Format:               s.Format,
		Description:          s.Description,
		Required:             append([]string(nil), s.Required...),
		Enum:                 append([]string(nil), s.Enum...),
		AdditionalProperties: s.AdditionalProperties,
	}
	if s.Ref != "" {
		out.Ref = "#/components/schemas/" + refName(s.Ref)
	}
	if s.Items != nil {
		out.Items = convertSchema(s.Items)
	}
	if len(s.Properties) > 0 {
		out.Properties = map[string]*contract.Schema{}
		for name, prop := range s.Properties {
			out.Properties[name] = convertSchema(prop)
		}
	}
	return out
}

func refName(ref string) string {
	if i := strings.LastIndex(ref, "/"); i >= 0 {
		return ref[i+1:]
	}
	return ref
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func fallbackOperationID(method, path string) string {
	clean := strings.NewReplacer("/", "_", "{", "", "}", "", "-", "_").Replace(path)
	clean = strings.Trim(clean, "_")
	return strings.ToLower(method) + "_" + clean
}
