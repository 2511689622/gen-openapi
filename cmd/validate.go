package cmd

import (
	"fmt"

	"gen-openapi/internal/apig"

	"github.com/spf13/cobra"
)

func newValidateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "validate <openAPI.yaml>",
		Short: "Validate a Huawei Cloud APIG OpenAPI YAML file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := apig.ValidateFile(args[0]); err != nil {
				return fmt.Errorf("validate %s: %w", args[0], err)
			}
			return nil
		},
	}
}
