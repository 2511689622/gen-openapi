package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

type initOptions struct {
	service string
	outDir  string
}

func newInitCommand() *cobra.Command {
	opts := &initOptions{}

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create api-contract.yaml and apig-config.yaml templates",
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.service == "" {
				return fmt.Errorf("--service is required")
			}
			if err := os.MkdirAll(opts.outDir, 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(opts.outDir, "api-contract.yaml"), []byte(contractTemplate(opts.service)), 0o644); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(opts.outDir, "apig-config.yaml"), []byte(apigConfigTemplate(opts.service)), 0o644)
		},
	}

	cmd.Flags().StringVar(&opts.service, "service", "", "service name")
	cmd.Flags().StringVar(&opts.outDir, "out", ".", "output directory")
	return cmd
}

func contractTemplate(service string) string {
	return fmt.Sprintf(`apiVersion: infra.example.com/v1
kind: ApiContract
metadata:
  name: %s
  title: %s API
  version: 1.0.0
  description: "TODO: describe this service"
spec:
  basePath: /api
  routes:
    - operationId: listResources
      method: GET
      path: /v1/resources
      summary: List resources
      auth: none
      backendPath: /api/v1/resources
      parameters:
        - name: page
          in: query
          type: integer
  schemas: {}
`, service, service)
}

func apigConfigTemplate(service string) string {
	return fmt.Sprintf(`apiVersion: infra.example.com/v1
kind: HuaweiApigConfig
metadata:
  name: %s-apig
spec:
  gatewayUrl: https://example.apic.ap-southeast-1.huaweicloudapis.com
  backend:
    type: HTTP
    scheme: https
    address: example.internal
    timeout: 5000
    retryCount: "0"
  defaults:
    cors: false
    sendFgBodyBase64: true
    matchMode: NORMAL
    requestType: public
    securityScheme: apig-auth-app-header
  securitySchemes:
    apig-auth-app-header:
      type: AppSigv1
      in: header
      name: Authorization
      appcodeAuthType: header
`, service)
}
