package cmd

import (
	"fmt"
	"os"

	"gen-openapi/internal/config"
	"gen-openapi/internal/diff"

	"github.com/spf13/cobra"
)

type diffOptions struct {
	contractPath string
	detectedPath string
	outPath      string
	failOnDrift  bool
}

func newDiffCommand() *cobra.Command {
	opts := &diffOptions{}

	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Diff a canonical api-contract.yaml against a detected candidate",
		Long: `Diff classifies every route into added / removed / changed / not-exposed
and prints a Markdown report. The "not exposed" section surfaces internal
paths (/debug, /metrics, /internal/*) that were discovered in the source but
deliberately kept out of the candidate — a human reviewer decides whether
they should be exposed via APIG.

Exit code is 0 on no drift, 1 if --fail-on-drift is set and any change is
found. The drift scan CI workflow relies on this to gate "open issue" steps.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Canonical contract must satisfy strict validation — it's the source
			// of truth. We deliberately want a parse/validate error here to fail
			// the whole drift run.
			canonical, err := config.LoadContract(opts.contractPath)
			if err != nil {
				return fmt.Errorf("load canonical contract: %w", err)
			}

			// Candidate is the output of the discover/import adapters; it routinely
			// has under-specified routes (e.g. path params not declared). Skip
			// strict validation for it.
			candidate, err := config.LoadContractLoose(opts.detectedPath)
			if err != nil {
				return fmt.Errorf("load detected contract: %w", err)
			}

			report := diff.Compare(canonical, candidate)
			md := diff.Markdown(report)

			if opts.outPath != "" {
				if report.IsEmpty() {
					// Leave the file empty on no drift so the CI shell `[ -s file ]`
					// check can decide whether to upload an artifact.
					if err := os.WriteFile(opts.outPath, nil, 0o644); err != nil {
						return err
					}
				} else if err := os.WriteFile(opts.outPath, []byte(md), 0o644); err != nil {
					return err
				}
			} else {
				fmt.Print(md)
			}

			if opts.failOnDrift && !report.IsEmpty() {
				return fmt.Errorf("drift detected")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&opts.contractPath, "contract", "api-contract.yaml", "path to the canonical api-contract.yaml")
	cmd.Flags().StringVar(&opts.detectedPath, "detected", "api-contract.detected.yaml", "path to the candidate api-contract.detected.yaml")
	cmd.Flags().StringVar(&opts.outPath, "out", "", "write the Markdown report to this file instead of stdout; empty file on no drift")
	cmd.Flags().BoolVar(&opts.failOnDrift, "fail-on-drift", false, "exit with non-zero status when any drift is found")
	return cmd
}
