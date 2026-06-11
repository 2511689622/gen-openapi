package cmd

import (
	"fmt"

	"gen-openapi/internal/catalog"

	"github.com/spf13/cobra"
)

type catalogCheckOptions struct {
	catalogPath string
	service     string
	repo        string
	contract    string
	apigConfig  string
	openAPI     string
}

func newCatalogCheckCommand() *cobra.Command {
	opts := &catalogCheckOptions{}

	cmd := &cobra.Command{
		Use:   "catalog-check",
		Short: "Check that a service is registered in the API catalog",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := catalog.Check(catalog.CheckOptions{
				CatalogPath: opts.catalogPath,
				Service:     opts.service,
				Repo:        opts.repo,
				Contract:    opts.contract,
				ApigConfig:  opts.apigConfig,
				OpenAPI:     opts.openAPI,
			}); err != nil {
				return err
			}
			fmt.Printf("Catalog entry for %s is up to date.\n", opts.service)
			return nil
		},
	}

	cmd.Flags().StringVar(&opts.catalogPath, "catalog", "catalog.yaml", "path to infra-api-catalog catalog.yaml")
	cmd.Flags().StringVar(&opts.service, "service", "", "service name to check")
	cmd.Flags().StringVar(&opts.repo, "repo", "", "expected service repository URL")
	cmd.Flags().StringVar(&opts.contract, "contract", "api/api-contract.yaml", "expected contract path")
	cmd.Flags().StringVar(&opts.apigConfig, "apig-config", "api/apig-config.yaml", "expected APIG config path")
	cmd.Flags().StringVar(&opts.openAPI, "openapi", "api/openAPI.yaml", "expected APIG OpenAPI YAML path")
	_ = cmd.MarkFlagRequired("service")
	return cmd
}
