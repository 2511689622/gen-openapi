package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gen-openapi/internal/discover"
	gindiscover "gen-openapi/internal/discover/gin"
	"gen-openapi/internal/importer/openapi"
	"gen-openapi/internal/output"
	"gen-openapi/pkg/contract"

	"github.com/spf13/cobra"
)

type discoverOptions struct {
	mode            string
	lang            string
	framework       string
	source          string
	out             string
	serviceName     string
	openAPIURL      string
	baseURL         string
	basePath        string
	defaultAuth     string
	includeInternal bool // when true, internal paths are kept in the candidate
}

func newDiscoverCommand() *cobra.Command {
	opts := &discoverOptions{}

	cmd := &cobra.Command{
		Use:   "discover",
		Short: "Discover APIs and emit an api-contract.detected.yaml candidate",
		Long: `Discover candidate routes from OpenAPI or source code.

Modes:
  --mode static  Use the lightweight Go/Gin AST scanner (default)
  --mode auto    Try runtime OpenAPI, local OpenAPI docs, then Go/Gin fallback

Currently supported static scanner:
  --lang go --framework gin

Auto mode priority:
  1. --openapi-url
  2. --base-url runtime probes (/openapi.json, /openapi.yaml, /v3/api-docs)
  3. --source docs/openapi.yaml|yml|json
  4. Go/Gin static scanner fallback

The output is a CANDIDATE only. Service owners must review it before promoting
it to the canonical api-contract.yaml.

By default internal paths (/debug, /metrics, /internal/*, /healthz, ...) are
omitted from the candidate so they cannot be accidentally promoted to the
public contract.

Use --include-internal to keep them in the candidate (rare; usually only when
you actually want to publish a health endpoint via APIG).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.serviceName == "" {
				return fmt.Errorf("--service is required")
			}
			if opts.source == "" {
				opts.source = "."
			}

			switch strings.ToLower(opts.mode) {
			case "", "static":
				return discoverStatic(opts)
			case "auto":
				return discoverAuto(cmd.Context(), opts)
			default:
				return fmt.Errorf("unsupported --mode %q", opts.mode)
			}
		},
	}

	cmd.Flags().StringVar(&opts.mode, "mode", "static", "discovery mode (static|auto)")
	cmd.Flags().StringVar(&opts.lang, "lang", "", "source language hint; static fallback currently supports go only")
	cmd.Flags().StringVar(&opts.framework, "framework", "", "framework hint; static fallback currently supports gin only")
	cmd.Flags().StringVar(&opts.source, "source", ".", "source directory to scan")
	cmd.Flags().StringVar(&opts.out, "out", "api-contract.detected.yaml", "candidate contract output path")
	cmd.Flags().StringVar(&opts.serviceName, "service", "", "service name to embed in the contract metadata")
	cmd.Flags().StringVar(&opts.openAPIURL, "openapi-url", "", "explicit OpenAPI 3.x endpoint URL for auto mode")
	cmd.Flags().StringVar(&opts.baseURL, "base-url", "", "service base URL for auto mode runtime OpenAPI probes")
	cmd.Flags().StringVar(&opts.basePath, "base-path", "/api", "backend base path used by the renderer for OpenAPI imports")
	cmd.Flags().StringVar(&opts.defaultAuth, "default-auth", "none", "default auth value to apply to imported OpenAPI routes (none|app)")
	cmd.Flags().BoolVar(&opts.includeInternal, "include-internal", false, "keep internal paths (/debug, /metrics, /internal/*) in the candidate; off by default")
	return cmd
}

func discoverStatic(opts *discoverOptions) error {
	if strings.ToLower(opts.lang) != "go" {
		return fmt.Errorf("static discovery currently supports only --lang go --framework gin; got --lang %q", opts.lang)
	}
	if opts.framework != "" && strings.ToLower(opts.framework) != "gin" {
		return fmt.Errorf("static discovery currently supports only --framework gin; got %q", opts.framework)
	}
	return discoverGo(opts)
}

func discoverAuto(ctx context.Context, opts *discoverOptions) error {
	if opts.openAPIURL != "" {
		c, err := importOpenAPIFromURL(ctx, opts, opts.openAPIURL)
		if err != nil {
			return err
		}
		return writeOpenAPICandidate(opts, c, opts.openAPIURL)
	}

	if opts.baseURL != "" {
		for _, u := range openAPIProbeURLs(opts) {
			c, err := importOpenAPIFromURL(ctx, opts, u)
			if err == nil {
				return writeOpenAPICandidate(opts, c, u)
			}
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
		}
	}

	for _, path := range localOpenAPIFiles(opts.source) {
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		c, err := importOpenAPIFromFile(opts, path)
		if err != nil {
			return fmt.Errorf("import local OpenAPI %s: %w", path, err)
		}
		return writeOpenAPICandidate(opts, c, path)
	}

	return discoverStatic(opts)
}

func importOpenAPIFromURL(ctx context.Context, opts *discoverOptions, u string) (*contract.ApiContract, error) {
	c, err := openapi.FromURLContext(ctx, u, openapiImportOptions(opts))
	if err != nil {
		return nil, err
	}
	filterInternalContractRoutes(c, opts.includeInternal)
	return c, nil
}

func importOpenAPIFromFile(opts *discoverOptions, path string) (*contract.ApiContract, error) {
	c, err := openapi.FromFile(path, openapiImportOptions(opts))
	if err != nil {
		return nil, err
	}
	filterInternalContractRoutes(c, opts.includeInternal)
	return c, nil
}

func openapiImportOptions(opts *discoverOptions) openapi.ImportOptions {
	return openapi.ImportOptions{
		ServiceName: opts.serviceName,
		BasePath:    opts.basePath,
		DefaultAuth: opts.defaultAuth,
	}
}

func openAPIProbeURLs(opts *discoverOptions) []string {
	base := strings.TrimRight(opts.baseURL, "/")
	paths := openAPIProbePaths(opts)
	urls := make([]string, 0, len(paths))
	for _, p := range paths {
		urls = append(urls, base+"/"+strings.TrimLeft(p, "/"))
	}
	return urls
}

func openAPIProbePaths(opts *discoverOptions) []string {
	lang := strings.ToLower(opts.lang)
	framework := strings.ToLower(opts.framework)

	switch {
	case lang == "java" || framework == "spring":
		return []string{"/v3/api-docs"}
	case lang == "python" || framework == "fastapi":
		return []string{"/openapi.json"}
	case lang == "go" || framework == "gin":
		return []string{"/openapi.json", "/openapi.yaml"}
	default:
		return []string{"/openapi.json", "/openapi.yaml", "/v3/api-docs"}
	}
}

func localOpenAPIFiles(source string) []string {
	return []string{
		filepath.Join(source, "docs", "openapi.yaml"),
		filepath.Join(source, "docs", "openapi.yml"),
		filepath.Join(source, "docs", "openapi.json"),
	}
}

func filterInternalContractRoutes(c *contract.ApiContract, includeInternal bool) {
	if includeInternal {
		return
	}
	kept := c.Spec.Routes[:0]
	for _, r := range c.Spec.Routes {
		if !discover.IsInternalPath(r.Path) {
			kept = append(kept, r)
		}
	}
	c.Spec.Routes = kept
}

func writeOpenAPICandidate(opts *discoverOptions, c *contract.ApiContract, source string) error {
	if err := output.WriteYAML(opts.out, c); err != nil {
		return err
	}
	fmt.Printf("Auto-discovered OpenAPI from: %s\n", source)
	fmt.Printf("Wrote candidate contract: %s (kept: %d)\n", opts.out, len(c.Spec.Routes))
	fmt.Println("Review it, then promote to api-contract.yaml.")
	return nil
}

func discoverGo(opts *discoverOptions) error {
	type routeKey struct {
		method string
		path   string
	}
	seen := map[routeKey]bool{}
	files := map[string][]byte{}

	err := filepath.WalkDir(opts.source, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Skip vendor/, .git/, and test files — Gin route registrations in
		// _test.go are nearly always for unit tests against the router.
		if d.IsDir() {
			name := d.Name()
			if name == "vendor" || name == ".git" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[path] = b
		return nil
	})
	if err != nil {
		return err
	}

	ordered, err := gindiscover.AnalyzeFiles(files)
	if err != nil || len(ordered) == 0 {
		ordered = nil
		for _, b := range files {
			ordered = append(ordered, gindiscover.ExtractRoutes(b)...)
		}
	}

	deduped := ordered[:0]
	for _, r := range ordered {
		k := routeKey{r.Method, r.Path}
		if seen[k] {
			continue
		}
		seen[k] = true
		deduped = append(deduped, r)
	}
	ordered = deduped

	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Path != ordered[j].Path {
			return ordered[i].Path < ordered[j].Path
		}
		return ordered[i].Method < ordered[j].Method
	})

	c := newCandidate(opts)
	c.Spec.BasePath = "/api"
	for _, r := range ordered {
		if !opts.includeInternal && discover.IsInternalPath(r.Path) {
			continue
		}
		c.Spec.Routes = append(c.Spec.Routes, contract.Route{
			OperationID: generateOpID(r.Method, r.Path),
			Method:      r.Method,
			Path:        r.Path,
			Auth:        "none",
			Parameters:  r.Parameters,
		})
	}

	return writeCandidate(opts, c, ordered)
}

// newCandidate returns a fresh ApiContract skeleton populated from the
// discover flags.
func newCandidate(opts *discoverOptions) *contract.ApiContract {
	return &contract.ApiContract{
		APIVersion: "infra.example.com/v1",
		Kind:       "ApiContract",
		Metadata: contract.Metadata{
			Name:    opts.serviceName,
			Title:   opts.serviceName + " API",
			Version: "1.0.0",
		},
		Spec: contract.Spec{
			Schemas: map[string]*contract.Schema{},
		},
	}
}

// writeCandidate writes the contract YAML and prints a one-line summary.
// `seen` is whatever per-language slice we have lying around; we only use
// its length for the summary, so any slice works.
func writeCandidate(opts *discoverOptions, c *contract.ApiContract, seen any) error {
	if err := output.WriteYAML(opts.out, c); err != nil {
		return err
	}
	rawCount := -1
	switch v := seen.(type) {
	case []gindiscover.RouteCandidate:
		rawCount = len(v)
	}
	if rawCount >= 0 {
		fmt.Printf("Wrote candidate contract: %s (raw routes scanned: %d, kept: %d)\n",
			opts.out, rawCount, len(c.Spec.Routes))
	} else {
		fmt.Printf("Wrote candidate contract: %s\n", opts.out)
	}
	fmt.Println("Review it, then promote to api-contract.yaml.")
	return nil
}

// generateOpID builds a stable fallback operation ID from method and path.
func generateOpID(method, path string) string {
	clean := strings.NewReplacer("/", "_", "{", "", "}", "", "-", "_", ":", "").Replace(path)
	clean = strings.Trim(clean, "_")
	return strings.ToLower(method) + "_" + clean
}
