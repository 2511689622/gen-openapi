package cmd

import (
	"fmt"
	"os"

	"gen-openapi/internal/apig"
	"gen-openapi/internal/apigimport"
	"gen-openapi/internal/catalog"
	"gen-openapi/internal/config"
	"gen-openapi/internal/output"

	"github.com/spf13/cobra"
)

type apigImportOptions struct {
	contractPath string
	apigPath     string
	outPath      string
	catalogPath  string
	serviceName  string
	dryRun       bool
	skipRender   bool
}

func newApigImportCommand() *cobra.Command {
	opts := &apigImportOptions{}

	cmd := &cobra.Command{
		Use:   "apig-import",
		Short: "Render and import openAPI.yaml into Huawei Cloud APIG",
		Long: `Render api-contract.yaml + apig-config.yaml into a Huawei Cloud APIG
OpenAPI file, validate it, then import it into an APIG target. The target can
come from spec.importTarget in apig-config.yaml or from a central catalog via
--catalog and --service.

Credentials are read from HUAWEICLOUD_SDK_AK and HUAWEICLOUD_SDK_SK. Do not put
AK/SK/token values in apig-config.yaml or catalog.yaml.

This command does not run discover or apply; scanner output must be reviewed and
promoted to api-contract.yaml before importing.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			apigCfg, err := config.LoadApigConfig(opts.apigPath)
			if err != nil {
				return err
			}
			target, err := loadImportTarget(opts, apigCfg)
			if err != nil {
				return err
			}
			if opts.skipRender {
				if err := apig.ValidateFile(opts.outPath); err != nil {
					return err
				}
			} else {
				contractDoc, err := config.LoadContract(opts.contractPath)
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
				if err := output.WriteYAML(opts.outPath, doc); err != nil {
					return err
				}
			}

			if opts.dryRun {
				fmt.Printf("Rendered APIG OpenAPI: %s\n", opts.outPath)
				fmt.Printf("Dry-run APIG import target: region=%s projectId=%s instanceId=%s groupId=%s apiMode=%s extendMode=%s\n",
					target.Region, target.ProjectID, target.InstanceID, target.GroupID, target.APIMode, target.ExtendMode)
				fmt.Println("Dry-run only; no Huawei Cloud API call was made.")
				return nil
			}

			res, err := apigimport.Import(cmd.Context(), apigimport.Options{
				FilePath: opts.outPath,
				Target:   *target,
				AK:       os.Getenv("HUAWEICLOUD_SDK_AK"),
				SK:       os.Getenv("HUAWEICLOUD_SDK_SK"),
			})
			if err != nil {
				return err
			}
			fmt.Printf("Imported APIG OpenAPI: groupId=%s success=%d failure=%d ignored=%d\n",
				res.GroupID, len(res.Success), len(res.Failure), len(res.Ignore))
			for _, f := range res.Failure {
				fmt.Printf("Failure: %s %s %s %s\n", f.Method, f.Path, f.ErrorCode, f.ErrorMsg)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&opts.contractPath, "contract", "api/api-contract.yaml", "path to reviewed api-contract.yaml")
	cmd.Flags().StringVar(&opts.apigPath, "apig-config", "api/apig-config.yaml", "path to apig-config.yaml used for rendering")
	cmd.Flags().StringVarP(&opts.outPath, "out", "o", "api/openAPI.yaml", "APIG OpenAPI YAML path to render/import")
	cmd.Flags().StringVar(&opts.catalogPath, "catalog", "", "optional central catalog.yaml containing the APIG import target")
	cmd.Flags().StringVar(&opts.serviceName, "service", "", "service name in --catalog whose importTarget should be used")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "render and validate, then print APIG import target without calling Huawei Cloud")
	cmd.Flags().BoolVar(&opts.skipRender, "skip-render", false, "validate and import the existing --out file without re-rendering")
	return cmd
}

func loadImportTarget(opts *apigImportOptions, apigCfg *config.HuaweiApigConfig) (*config.ImportTarget, error) {
	if opts.catalogPath != "" || opts.serviceName != "" {
		if opts.catalogPath == "" || opts.serviceName == "" {
			return nil, fmt.Errorf("--catalog and --service must be provided together")
		}
		svc, err := catalog.LoadService(opts.catalogPath, opts.serviceName)
		if err != nil {
			return nil, err
		}
		if svc.ImportTarget == nil {
			return nil, fmt.Errorf("catalog service %q importTarget is required for apig-import", opts.serviceName)
		}
		target := *svc.ImportTarget
		if err := config.ValidateImportTarget(&target); err != nil {
			return nil, err
		}
		return &target, nil
	}
	if apigCfg.Spec.ImportTarget == nil {
		return nil, fmt.Errorf("apig spec.importTarget is required for apig-import when --catalog/--service are not provided")
	}
	target := *apigCfg.Spec.ImportTarget
	if err := config.ValidateImportTarget(&target); err != nil {
		return nil, err
	}
	return &target, nil
}
