package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gen-openapi/internal/apig"
	"gen-openapi/internal/config"
	"gen-openapi/internal/discover"
	gindiscover "gen-openapi/internal/discover/gin"
	"gen-openapi/internal/output"
	"gen-openapi/pkg/contract"

	"github.com/spf13/cobra"
)

type repoBootstrapOptions struct {
	repo           string
	service        string
	outDir         string
	apiDir         string
	lang           string
	framework      string
	gatewayURL     string
	backendAddress string
	backendScheme  string
}

func newRepoBootstrapCommand() *cobra.Command {
	opts := &repoBootstrapOptions{}
	cmd := &cobra.Command{
		Use:   "repo-bootstrap",
		Short: "Clone a service repo and generate API contract/APIG files",
		Long: `Clone a GitHub service repository, discover Go/Gin routes, then generate
apig/api-contract.yaml, apig/apig-config.yaml, and apig/openAPI.yaml locally.
Use --api-dir to pick a different directory name.

This command only generates files. It does not write GitHub Actions, commit,
push, open a PR, or import anything into Huawei Cloud APIG. Use the dedicated
pr and apig-import commands for those later steps.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRepoBootstrap(cmd.Context(), *opts)
		},
	}
	cmd.Flags().StringVar(&opts.repo, "repo", "", "GitHub repository URL or owner/name")
	cmd.Flags().StringVar(&opts.service, "service", "", "service name for api-contract.yaml (default: repo name)")
	cmd.Flags().StringVar(&opts.outDir, "out", "", "directory to clone into and write generated files (default: ./gen-openapi-bootstrap-<service>)")
	cmd.Flags().StringVar(&opts.apiDir, "api-dir", "apig", "directory inside the cloned repo for generated files")
	cmd.Flags().StringVar(&opts.lang, "lang", "go", "service language; first version supports go only")
	cmd.Flags().StringVar(&opts.framework, "framework", "gin", "service framework; first version supports gin only")
	cmd.Flags().StringVar(&opts.gatewayURL, "gateway-url", "https://replace-with-gateway.apic.huaweicloudapis.com", "Huawei APIG gateway URL placeholder written to apig-config.yaml")
	cmd.Flags().StringVar(&opts.backendAddress, "backend-address", "", "backend service host/address (default: <service>.example.com)")
	cmd.Flags().StringVar(&opts.backendScheme, "backend-scheme", "https", "backend scheme")
	_ = cmd.MarkFlagRequired("repo")
	return cmd
}

func runRepoBootstrap(ctx context.Context, opts repoBootstrapOptions) error {
	if strings.ToLower(opts.lang) != "go" || strings.ToLower(opts.framework) != "gin" {
		return fmt.Errorf("repo-bootstrap currently supports only --lang go --framework gin")
	}
	if opts.service == "" {
		service, err := serviceNameFromRepo(opts.repo)
		if err != nil {
			return err
		}
		opts.service = service
	}
	if opts.backendAddress == "" {
		opts.backendAddress = opts.service + ".example.com"
	}
	root := bootstrapOutputDir(opts)

	if err := runCmdIn(ctx, "", "git", "clone", opts.repo, root); err != nil {
		return err
	}

	contractDoc, err := bootstrapContract(root, opts.service)
	if err != nil {
		return err
	}
	apiDir, adopted, err := writeBootstrapFiles(root, opts, contractDoc, bootstrapApigConfig(opts))
	if err != nil {
		return err
	}

	fmt.Printf("Generated detected contract in %s\n", filepath.Join(apiDir, "api-contract.detected.yaml"))
	if !adopted {
		fmt.Printf("Existing contract found at %s; left it unchanged. Review the detected contract with diff/apply.\n", filepath.Join(apiDir, "api-contract.yaml"))
		return nil
	}
	fmt.Printf("Generated API files in %s\n", apiDir)
	fmt.Println("Next: review api-contract.yaml and apig-config.yaml, then run validate/check as needed.")
	return nil
}

func bootstrapOutputDir(opts repoBootstrapOptions) string {
	if opts.outDir != "" {
		return opts.outDir
	}
	return filepath.Join(".", "gen-openapi-bootstrap-"+opts.service)
}

func writeBootstrapFiles(root string, opts repoBootstrapOptions, detected *contract.ApiContract, apigCfg *config.HuaweiApigConfig) (string, bool, error) {
	apiDir := filepath.Join(root, opts.apiDir)
	if err := os.MkdirAll(apiDir, 0o755); err != nil {
		return "", false, err
	}

	detectedPath := filepath.Join(apiDir, "api-contract.detected.yaml")
	if err := output.WriteYAML(detectedPath, detected); err != nil {
		return "", false, err
	}

	contractPath := filepath.Join(apiDir, "api-contract.yaml")
	if _, err := os.Stat(contractPath); err == nil {
		return apiDir, false, nil
	} else if !os.IsNotExist(err) {
		return "", false, err
	}

	if err := output.WriteYAML(contractPath, detected); err != nil {
		return "", false, err
	}
	if err := output.WriteYAML(filepath.Join(apiDir, "apig-config.yaml"), apigCfg); err != nil {
		return "", false, err
	}
	rendered, err := apig.Render(detected, apigCfg)
	if err != nil {
		return "", false, err
	}
	if err := apig.Validate(rendered); err != nil {
		return "", false, err
	}
	if err := output.WriteYAML(filepath.Join(apiDir, "openAPI.yaml"), rendered); err != nil {
		return "", false, err
	}
	return apiDir, true, nil
}

func serviceNameFromRepo(repo string) (string, error) {
	trimmed := strings.TrimSpace(strings.TrimRight(repo, "/"))
	if trimmed == "" {
		return "", fmt.Errorf("--repo is required")
	}
	if i := strings.LastIndex(trimmed, ":"); i >= 0 && !strings.Contains(trimmed[i+1:], "/") {
		trimmed = trimmed[i+1:]
	}
	parts := strings.Split(trimmed, "/")
	name := parts[len(parts)-1]
	name = strings.TrimSuffix(name, ".git")
	if name == "" || name == ".git" {
		return "", fmt.Errorf("cannot infer --service from --repo %q", repo)
	}
	return name, nil
}

func bootstrapContract(root, service string) (*contract.ApiContract, error) {
	files, err := collectGoFiles(root)
	if err != nil {
		return nil, err
	}
	routes, err := gindiscover.AnalyzeFiles(files)
	if err != nil || len(routes) == 0 {
		for _, b := range files {
			routes = append(routes, gindiscover.ExtractRoutes(b)...)
		}
	}
	if len(routes) == 0 {
		return nil, fmt.Errorf("no Go/Gin routes discovered in %s", root)
	}
	sort.SliceStable(routes, func(i, j int) bool {
		if routes[i].Path != routes[j].Path {
			return routes[i].Path < routes[j].Path
		}
		return routes[i].Method < routes[j].Method
	})
	doc := &contract.ApiContract{
		APIVersion: "infra.example.com/v1",
		Kind:       "ApiContract",
		Metadata: contract.Metadata{
			Name:    service,
			Title:   service + " API",
			Version: time.Now().UTC().Format("2006-01-02"),
		},
		Spec: contract.Spec{BasePath: "/api", Schemas: map[string]*contract.Schema{}},
	}
	seen := map[string]bool{}
	for _, r := range routes {
		if discover.IsInternalPath(r.Path) {
			continue
		}
		key := r.Method + " " + r.Path
		if seen[key] {
			continue
		}
		seen[key] = true
		doc.Spec.Routes = append(doc.Spec.Routes, contract.Route{
			OperationID: generateOpID(r.Method, r.Path),
			Method:      r.Method,
			Path:        r.Path,
			Auth:        "none",
			Parameters:  r.Parameters,
		})
	}
	if len(doc.Spec.Routes) == 0 {
		return nil, fmt.Errorf("all discovered routes were filtered as internal")
	}
	return doc, nil
}

func collectGoFiles(root string) (map[string][]byte, error) {
	files := map[string][]byte{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "vendor", "node_modules":
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
	return files, err
}

func bootstrapApigConfig(opts repoBootstrapOptions) *config.HuaweiApigConfig {
	return &config.HuaweiApigConfig{
		APIVersion: "infra.example.com/v1",
		Kind:       "HuaweiApigConfig",
		Metadata:   config.Metadata{Name: opts.service + "-apig"},
		Spec: config.HuaweiSpec{
			GatewayURL: opts.gatewayURL,
			Backend:    config.Backend{Type: "HTTP", Scheme: opts.backendScheme, Address: opts.backendAddress, Timeout: 5000, RetryCount: "0"},
			Defaults:   config.Defaults{Cors: false, SendFgBodyBase64: true, MatchMode: "NORMAL", RequestType: "public", SecurityScheme: "apig-auth-app-header"},
			SecuritySchemes: map[string]config.SecurityScheme{
				"apig-auth-app-header": {Type: "AppSigv1", In: "header", Name: "Authorization", AppcodeAuthType: "header"},
			},
		},
	}
}

func runCmdIn(ctx context.Context, dir, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s failed: %w", name, strings.Join(args, " "), err)
	}
	return nil
}
