package cmd

import (
	"fmt"
	"strings"

	prrun "gen-openapi/internal/pr"

	"github.com/spf13/cobra"
)

type prOptions struct {
	contractPath    string
	detectedPath    string
	apigConfigPath  string
	openAPIPath     string
	skipRender      bool
	prune           bool
	includeInternal bool
	base            string
	branch          string
	remote          string
	title           string
	draft           bool
	labels          []string
	dryRun          bool
}

func newPRCommand() *cobra.Command {
	opts := &prOptions{}

	cmd := &cobra.Command{
		Use:   "pr",
		Short: "Create a reviewed GitHub PR from detected API contract drift",
		Long: `Create a GitHub pull request from a detected candidate contract.

The command consumes api/api-contract.detected.yaml, conservatively merges it
into api/api-contract.yaml, renders api/openAPI.yaml by default, then creates a
branch, commits the generated changes, pushes it, and calls gh pr create.

It never merges the PR. Human owner review is required before merge.

Use --dry-run to preview the branch, files, title, and PR body without writing
files or invoking git/gh mutation commands.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := prrun.Run(cmd.Context(), prrun.Options{
				ContractPath:    opts.contractPath,
				DetectedPath:    opts.detectedPath,
				ApigConfigPath:  opts.apigConfigPath,
				OpenAPIPath:     opts.openAPIPath,
				SkipRender:      opts.skipRender,
				Prune:           opts.prune,
				IncludeInternal: opts.includeInternal,
				Base:            opts.base,
				Branch:          opts.branch,
				Remote:          opts.remote,
				Title:           opts.title,
				Draft:           opts.draft,
				Labels:          opts.labels,
				DryRun:          opts.dryRun,
			}, prrun.ExecRunner{})
			if err != nil {
				return err
			}
			printPRResult(res)
			return nil
		},
	}

	cmd.Flags().StringVar(&opts.contractPath, "contract", "api/api-contract.yaml", "path to canonical api-contract.yaml")
	cmd.Flags().StringVar(&opts.detectedPath, "detected", "api/api-contract.detected.yaml", "path to detected candidate contract")
	cmd.Flags().StringVar(&opts.apigConfigPath, "apig-config", "api/apig-config.yaml", "path to apig-config.yaml used for rendering")
	cmd.Flags().StringVar(&opts.openAPIPath, "openapi", "api/openAPI.yaml", "path to rendered APIG OpenAPI output")
	cmd.Flags().BoolVar(&opts.skipRender, "skip-render", false, "do not update openAPI.yaml")
	cmd.Flags().BoolVar(&opts.prune, "prune", false, "remove routes from canonical that no longer exist in detected")
	cmd.Flags().BoolVar(&opts.includeInternal, "include-internal", false, "include internal routes (/debug, /metrics, /internal/*)")
	cmd.Flags().StringVar(&opts.base, "base", "", "base branch for the pull request (default: current branch)")
	cmd.Flags().StringVar(&opts.branch, "branch", "", "branch name to create (default: generated timestamped branch)")
	cmd.Flags().StringVar(&opts.remote, "remote", "origin", "git remote to push to")
	cmd.Flags().StringVar(&opts.title, "title", "Update API contract from detected drift", "pull request title and commit subject")
	cmd.Flags().BoolVar(&opts.draft, "draft", false, "create a draft pull request")
	cmd.Flags().StringSliceVar(&opts.labels, "label", []string{"api-contract", "drift"}, "labels to apply to the pull request")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "preview without writing files or invoking mutation commands")
	return cmd
}

func printPRResult(res *prrun.Result) {
	if res.NoDrift {
		fmt.Println("No drift detected; no PR created.")
		return
	}
	if res.DryRun {
		fmt.Printf("[DRY RUN] Would create branch %s from %s\n", res.Branch, res.Base)
		fmt.Printf("[DRY RUN] Would update %s\n", strings.Join(res.FilesUpdated, ", "))
		fmt.Printf("[DRY RUN] Would open GitHub PR: %s\n\n", res.Title)
		fmt.Println(res.Body)
		return
	}
	fmt.Printf("Created API contract drift PR from branch %s\n", res.Branch)
	if res.PROutput != "" {
		fmt.Println(res.PROutput)
	}
}
