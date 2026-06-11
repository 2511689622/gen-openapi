package cmd

import (
	"fmt"
	"strings"

	"gen-openapi/internal/apply"
	"gen-openapi/internal/config"
	"gen-openapi/internal/output"

	"github.com/spf13/cobra"
)

type applyOptions struct {
	contractPath    string
	detectedPath    string
	outPath         string
	prune           bool
	includeInternal bool
	dryRun          bool
}

func newApplyCommand() *cobra.Command {
	opts := &applyOptions{}

	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Merge detected routes into the canonical api-contract.yaml",
		Long: `Apply merges routes from the detected candidate (api-contract.detected.yaml)
into the canonical api-contract.yaml using a conservative strategy:

  - Added routes:       copied from detected into canonical
  - Changed routes:     preserve canonical auth/summary/description/backendPath,
                        only add new parameters from detected
  - Removed routes:     kept by default (use --prune to remove)
  - Internal routes:    skipped by default (use --include-internal to promote)

The merged contract is validated before writing. Use --dry-run to preview.

Exit code is 0 on success, 1 on error.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			canonical, err := config.LoadContract(opts.contractPath)
			if err != nil {
				return fmt.Errorf("load canonical: %w", err)
			}

			detected, err := config.LoadContractLoose(opts.detectedPath)
			if err != nil {
				return fmt.Errorf("load detected: %w", err)
			}

			merged, rep, err := apply.Merge(canonical, detected, apply.Options{
				Prune:           opts.prune,
				IncludeInternal: opts.includeInternal,
			})
			if err != nil {
				return err
			}

			if !opts.dryRun {
				if err := output.WriteYAML(opts.outPath, merged); err != nil {
					return fmt.Errorf("write merged contract: %w", err)
				}
			}

			printApplySummary(opts.outPath, rep, opts.dryRun)
			return nil
		},
	}

	cmd.Flags().StringVar(&opts.contractPath, "contract", "api-contract.yaml", "path to canonical api-contract.yaml")
	cmd.Flags().StringVar(&opts.detectedPath, "detected", "api-contract.detected.yaml", "path to detected candidate")
	cmd.Flags().StringVarP(&opts.outPath, "out", "o", "api-contract.yaml", "output path for the merged contract")
	cmd.Flags().BoolVar(&opts.prune, "prune", false, "remove routes from canonical that no longer exist in detected")
	cmd.Flags().BoolVar(&opts.includeInternal, "include-internal", false, "include internal routes (/debug, /metrics, /internal/*)")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "preview changes without writing to disk")
	return cmd
}

func printApplySummary(path string, rep *apply.Report, dryRun bool) {
	prefix := ""
	if dryRun {
		prefix = "[DRY RUN] "
	}

	if rep.Added == nil && rep.Merged == nil && rep.Pruned == nil && rep.Skipped == nil {
		fmt.Printf("%sNo changes to apply to %s\n", prefix, path)
		return
	}

	fmt.Printf("%sApplied changes to %s\n\n", prefix, path)

	for _, c := range rep.Added {
		fmt.Printf("  + %s %s\n", c.Key.Method, c.Key.Path)
	}
	for _, c := range rep.Merged {
		fmt.Printf("  ~ %s %s  (%s)\n", c.Key.Method, c.Key.Path, strings.Join(c.Reasons, ", "))
	}
	for _, c := range rep.Pruned {
		fmt.Printf("  - %s %s\n", c.Key.Method, c.Key.Path)
	}
	for _, c := range rep.Skipped {
		fmt.Printf("  . %s %s  (skipped)\n", c.Key.Method, c.Key.Path)
	}
	for _, name := range rep.AddedSchema {
		fmt.Printf("  ~ schema: %s  (copied from detected)\n", name)
	}

	fmt.Printf("\nSummary: %d added, %d merged, %d pruned, %d skipped, %d schema(s) copied\n",
		len(rep.Added), len(rep.Merged), len(rep.Pruned), len(rep.Skipped), len(rep.AddedSchema))
}
