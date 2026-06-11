package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

// Version is overridden at build time:
//
//	go build -ldflags "-X gen-openapi/cmd.Version=v0.3.0" .
//
// Defaults to "dev" for `go run .` and bare `go build`.
var Version = "dev"

// NewRootCommand returns a fresh command tree with independent flag state.
func NewRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "gen-openapi",
		Short: "Generate Huawei Cloud APIG OpenAPI YAML from explicit API contracts",
		Long: `gen-openapi compiles an explicit api-contract.yaml + apig-config.yaml
into a Huawei Cloud APIG-importable OpenAPI 3.0.3 document.

The contract is the source of truth; scanners (discover / import-openapi)
produce candidates for review, never the canonical contract.
Scanner output is always *.detected.yaml; promotion to api-contract.yaml
goes through a PR reviewed by the service owner.

See DESIGN.md for the architecture and docs/SESSION-CONTEXT.md for the
current state.`,
		Version: Version,
		// Don't print the full usage block on a RunE error — a "file not found"
		// shouldn't dump 80 lines of help to stderr.
		SilenceUsage: true,
		// We print errors ourselves in Execute() so they go to stderr without
		// the "Error: " prefix cobra adds.
		SilenceErrors: true,
	}

	rootCmd.AddCommand(
		newInitCommand(),
		newRenderCommand(),
		newValidateCommand(),
		newCheckCommand(),
		newImportOpenAPICommand(),
		newApigImportCommand(),
		newDiscoverCommand(),
		newDiffCommand(),
		newApplyCommand(),
		newPRCommand(),
		newCatalogCheckCommand(),
		newRepoBootstrapCommand(),
		newActionsInitCommand(),
	)

	return rootCmd
}

// Execute runs the root command with a context that is cancelled on
// SIGINT / SIGTERM, so long-running operations (HTTP fetches in
// import-openapi, big WalkDir trees in discover) can shut down promptly.
func Execute() {
	ctx, cancel := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := NewRootCommand().ExecuteContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "gen-openapi: %s\n", err)
		os.Exit(1)
	}
}
