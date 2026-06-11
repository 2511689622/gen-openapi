package catalog

import (
	"fmt"
	"os"

	"gen-openapi/internal/config"

	"gopkg.in/yaml.v3"
)

type CheckOptions struct {
	CatalogPath string
	Service     string
	Repo        string
	Contract    string
	ApigConfig  string
	OpenAPI     string
}

type Catalog struct {
	APIVersion string      `yaml:"apiVersion"`
	Kind       string      `yaml:"kind"`
	Spec       CatalogSpec `yaml:"spec"`
}

type CatalogSpec struct {
	Services []Service `yaml:"services"`
}

type Service struct {
	Name         string               `yaml:"name"`
	Repo         string               `yaml:"repo"`
	Contract     string               `yaml:"contract"`
	ApigConfig   string               `yaml:"apigConfig"`
	ApigYAML     string               `yaml:"apigYaml"`
	ImportTarget *config.ImportTarget `yaml:"importTarget,omitempty"`
}

func Check(opts CheckOptions) error {
	if opts.CatalogPath == "" {
		return fmt.Errorf("--catalog is required")
	}
	if opts.Service == "" {
		return fmt.Errorf("--service is required")
	}
	svc, err := LoadService(opts.CatalogPath, opts.Service)
	if err != nil {
		return err
	}
	return checkService(opts, *svc)
}

func LoadService(catalogPath, service string) (*Service, error) {
	if catalogPath == "" {
		return nil, fmt.Errorf("--catalog is required")
	}
	if service == "" {
		return nil, fmt.Errorf("--service is required")
	}
	b, err := os.ReadFile(catalogPath)
	if err != nil {
		return nil, err
	}
	var c Catalog
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("load catalog %s: %w", catalogPath, err)
	}
	if c.Kind != "ApiCatalog" {
		return nil, fmt.Errorf("catalog kind must be ApiCatalog")
	}
	for _, svc := range c.Spec.Services {
		if svc.Name == service {
			return &svc, nil
		}
	}
	return nil, fmt.Errorf("catalog missing service %q", service)
}

func checkService(opts CheckOptions, svc Service) error {
	checks := []struct {
		name string
		want string
		got  string
	}{
		{name: "repo", want: opts.Repo, got: svc.Repo},
		{name: "contract", want: opts.Contract, got: svc.Contract},
		{name: "apigConfig", want: opts.ApigConfig, got: svc.ApigConfig},
		{name: "apigYaml", want: opts.OpenAPI, got: svc.ApigYAML},
	}
	for _, check := range checks {
		if check.want == "" {
			continue
		}
		if check.got != check.want {
			return fmt.Errorf("catalog service %s %s mismatch: got %q, want %q", opts.Service, check.name, check.got, check.want)
		}
	}
	return nil
}
