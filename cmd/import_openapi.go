package cmd

import (
	"fmt"
	"strings"

	"gen-openapi/internal/importer/openapi"
	"gen-openapi/internal/output"
	"gen-openapi/pkg/contract"

	"github.com/spf13/cobra"
)

type importOpenAPIOptions struct {
	input       string
	out         string
	serviceName string
	basePath    string
	defaultAuth string
}

func newImportOpenAPICommand() *cobra.Command {
	opts := &importOpenAPIOptions{}

	cmd := &cobra.Command{
		Use:   "import-openapi",
		Short: "Import an OpenAPI 3.x doc (file or URL) and produce an ApiContract draft",
		Long: `Import an OpenAPI 3.x document and convert it to an ApiContract draft.

Sources supported:
  --input ./openapi.yaml
  --input http://localhost:8080/v3/api-docs   (Spring Boot via springdoc-openapi)
  --input http://localhost:8000/openapi.json  (FastAPI native /openapi.json)

This produces a candidate contract. It must be reviewed before being promoted
to the canonical api-contract.yaml.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.input == "" {
				return fmt.Errorf("--input is required")
			}
			if opts.serviceName == "" {
				return fmt.Errorf("--service is required")
			}

			importOpts := openapi.ImportOptions{
				ServiceName: opts.serviceName,
				BasePath:    opts.basePath,
				DefaultAuth: opts.defaultAuth,
			}

			var err error
			c := &contract.ApiContract{}
			if isURL(opts.input) {
				c, err = openapi.FromURLContext(cmd.Context(), opts.input, importOpts)
			} else {
				c, err = openapi.FromFile(opts.input, importOpts)
			}
			if err != nil {
				return err
			}

			if err := output.WriteYAML(opts.out, c); err != nil {
				return err
			}
			fmt.Printf("Wrote candidate contract: %s\n", opts.out)
			fmt.Println("Review it, then promote to api-contract.yaml.")
			return nil
		},
	}

	cmd.Flags().StringVar(&opts.input, "input", "", "path or URL to OpenAPI 3.x file (yaml/json) or endpoint")
	cmd.Flags().StringVar(&opts.out, "out", "api-contract.detected.yaml", "candidate contract output path")
	cmd.Flags().StringVar(&opts.serviceName, "service", "", "service name to embed in the contract metadata")
	cmd.Flags().StringVar(&opts.basePath, "base-path", "/api", "backend base path used by the renderer")
	cmd.Flags().StringVar(&opts.defaultAuth, "default-auth", "none", "default auth value to apply to each route (none|app)")
	return cmd
}

func isURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}
