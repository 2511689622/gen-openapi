package cmd

import (
	"gen-openapi/internal/apig"
	"gen-openapi/internal/config"
	"gen-openapi/internal/output"

	"github.com/spf13/cobra"
)

type renderOptions struct {
	contractPath string
	apigPath     string
	outPath      string
}

func newRenderCommand() *cobra.Command {
	opts := &renderOptions{}

	cmd := &cobra.Command{
		Use:   "render",
		Short: "Render an API contract as Huawei Cloud APIG OpenAPI YAML",
		RunE: func(cmd *cobra.Command, args []string) error {
			contractDoc, err := config.LoadContract(opts.contractPath)
			if err != nil {
				return err
			}

			apigCfg, err := config.LoadApigConfig(opts.apigPath)
			if err != nil {
				return err
			}

			doc, err := apig.Render(contractDoc, apigCfg)
			if err != nil {
				return err
			}
			if err := apig.Validate(doc); err != nil {
				return err
			}

			return output.WriteYAML(opts.outPath, doc)
		},
	}

	cmd.Flags().StringVar(&opts.contractPath, "contract", "api-contract.yaml", "path to api-contract.yaml")
	cmd.Flags().StringVar(&opts.apigPath, "apig-config", "apig-config.yaml", "path to apig-config.yaml")
	cmd.Flags().StringVarP(&opts.outPath, "out", "o", "openAPI.yaml", "output YAML path")
	return cmd
}
