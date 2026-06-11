package cmd

import (
	"fmt"

	"gen-openapi/internal/check"

	"github.com/spf13/cobra"
)

type checkOptions struct {
	contractPath string
	apigPath     string
	outputPath   string
}

func newCheckCommand() *cobra.Command {
	opts := &checkOptions{}

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check whether openAPI.yaml is up to date with the contract and apig config",
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := check.Run(opts.contractPath, opts.apigPath, opts.outputPath)
			if err != nil {
				return err
			}
			if res.UpToDate {
				fmt.Printf("openAPI.yaml is up to date: %s\n", opts.outputPath)
				return nil
			}
			return fmt.Errorf("openAPI.yaml is out of date.\n%s\nRun:\n  gen-openapi render --contract %s --apig-config %s -o %s",
				res.Diff, opts.contractPath, opts.apigPath, opts.outputPath)
		},
	}

	cmd.Flags().StringVar(&opts.contractPath, "contract", "api-contract.yaml", "path to api-contract.yaml")
	cmd.Flags().StringVar(&opts.apigPath, "apig-config", "apig-config.yaml", "path to apig-config.yaml")
	cmd.Flags().StringVar(&opts.outputPath, "output", "openAPI.yaml", "path to existing openAPI.yaml to verify")
	return cmd
}
