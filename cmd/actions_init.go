package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

type actionsInitOptions struct {
	outDir string
	force  bool
}

func newActionsInitCommand() *cobra.Command {
	opts := &actionsInitOptions{}
	cmd := &cobra.Command{
		Use:   "actions-init",
		Short: "Write APIG GitHub Actions workflows into a service repo",
		Long: `Write the standard APIG GitHub Actions workflows into a service repo.

The command writes .github/workflows/apig-check.yml, apig-drift-scan.yml, and
apig-deploy.yml. It does not commit, push, open a PR, or import APIG.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			written, err := runActionsInit(*opts)
			if err != nil {
				return err
			}
			for _, path := range written {
				fmt.Printf("Wrote %s\n", path)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&opts.outDir, "out", ".", "service repo root to write workflows into")
	cmd.Flags().BoolVar(&opts.force, "force", false, "overwrite existing workflow files")
	return cmd
}

func runActionsInit(opts actionsInitOptions) ([]string, error) {
	workflowDir := filepath.Join(opts.outDir, ".github", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		return nil, err
	}
	files := map[string]string{
		"apig-check.yml":      actionsCheckWorkflow,
		"apig-drift-scan.yml": actionsDriftWorkflow,
		"apig-deploy.yml":     actionsDeployWorkflow,
	}
	var written []string
	for name, content := range files {
		path := filepath.Join(workflowDir, name)
		if !opts.force {
			if _, err := os.Stat(path); err == nil {
				return nil, fmt.Errorf("workflow already exists: %s (use --force to overwrite)", path)
			} else if !os.IsNotExist(err) {
				return nil, err
			}
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return nil, err
		}
		written = append(written, path)
	}
	return written, nil
}

const actionsCheckWorkflow = `# PR check: ensure apig/openAPI.yaml stays in sync with apig/api-contract.yaml
# and apig/apig-config.yaml in the same service repo.
name: apig-check

on:
  pull_request:
    branches: [main, master]
    paths:
      - 'apig/**'
      - '**/*.go'
      - '**/*.java'
      - '**/*.py'
  workflow_dispatch:

jobs:
  check:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout service repo
        uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'

      - name: Checkout gen-openapi tool and catalog
        uses: actions/checkout@v4
        with:
          repository: ${{ vars.GEN_OPENAPI_REPO }}
          ref: ${{ vars.GEN_OPENAPI_REF || 'main' }}
          path: gen-openapi
          token: ${{ secrets.GEN_OPENAPI_TOKEN || github.token }}

      - name: Install gen-openapi
        working-directory: gen-openapi
        run: go install .

      - name: Check API catalog registration
        run: |
          gen-openapi catalog-check \
            --catalog "${API_CATALOG_PATH:-gen-openapi/catalog/catalog.yaml}" \
            --service "${{ github.event.repository.name }}" \
            --repo "${{ github.server_url }}/${{ github.repository }}.git" \
            --contract apig/api-contract.yaml \
            --apig-config apig/apig-config.yaml \
            --openapi apig/openAPI.yaml
        env:
          API_CATALOG_PATH: ${{ vars.API_CATALOG_PATH }}

      - name: Validate api-contract.yaml + apig-config.yaml
        run: |
          gen-openapi render \
            --contract    apig/api-contract.yaml \
            --apig-config apig/apig-config.yaml \
            --out         /tmp/openAPI.rendered.yaml
          gen-openapi validate /tmp/openAPI.rendered.yaml

      - name: Check openAPI.yaml is in sync
        run: |
          gen-openapi check \
            --contract    apig/api-contract.yaml \
            --apig-config apig/apig-config.yaml \
            --output      apig/openAPI.yaml
`

const actionsDriftWorkflow = `# Nightly drift scan: rerun discovery against the live source tree, diff against
# apig/api-contract.yaml, and create a reviewed PR when they disagree.
name: apig-drift-scan

on:
  schedule:
    - cron: '0 2 * * *'
  workflow_dispatch:

permissions:
  contents: write
  pull-requests: write

jobs:
  drift:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout service repo
        uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'

      - name: Checkout gen-openapi tool and catalog
        uses: actions/checkout@v4
        with:
          repository: ${{ vars.GEN_OPENAPI_REPO }}
          ref: ${{ vars.GEN_OPENAPI_REF || 'main' }}
          path: gen-openapi
          token: ${{ secrets.GEN_OPENAPI_TOKEN || github.token }}

      - name: Install gen-openapi
        working-directory: gen-openapi
        run: go install .

      - name: Check API catalog registration
        run: |
          gen-openapi catalog-check \
            --catalog "${API_CATALOG_PATH:-gen-openapi/catalog/catalog.yaml}" \
            --service "${{ github.event.repository.name }}" \
            --repo "${{ github.server_url }}/${{ github.repository }}.git" \
            --contract apig/api-contract.yaml \
            --apig-config apig/apig-config.yaml \
            --openapi apig/openAPI.yaml
        env:
          API_CATALOG_PATH: ${{ vars.API_CATALOG_PATH }}

      - name: Discover routes from source
        env:
          DETECT_MODE: ${{ vars.DETECT_MODE }}
          DETECT_LANG: ${{ vars.DETECT_LANG }}
          DETECT_FRAMEWORK: ${{ vars.DETECT_FRAMEWORK }}
          DETECT_OPENAPI_URL: ${{ vars.DETECT_OPENAPI_URL }}
          DETECT_BASE_URL: ${{ vars.DETECT_BASE_URL }}
        run: |
          args=(
            --mode      "${DETECT_MODE:-static}"
            --lang      "${DETECT_LANG:-go}"
            --framework "${DETECT_FRAMEWORK:-gin}"
            --source    .
            --service   "${{ github.event.repository.name }}"
            --out       /tmp/api-contract.detected.yaml
          )
          if [ -n "${DETECT_OPENAPI_URL:-}" ]; then
            args+=(--openapi-url "$DETECT_OPENAPI_URL")
          fi
          if [ -n "${DETECT_BASE_URL:-}" ]; then
            args+=(--base-url "$DETECT_BASE_URL")
          fi
          gen-openapi discover "${args[@]}"

      - name: Diff candidate against canonical contract
        id: diff
        run: |
          gen-openapi diff \
            --contract apig/api-contract.yaml \
            --detected /tmp/api-contract.detected.yaml \
            --out      /tmp/diff.md
          if [ -s /tmp/diff.md ]; then
            echo "drift=true" >> "$GITHUB_OUTPUT"
          else
            echo "drift=false" >> "$GITHUB_OUTPUT"
          fi

      - name: Create reviewed drift PR
        if: steps.diff.outputs.drift == 'true'
        env:
          GH_TOKEN: ${{ github.token }}
        run: |
          gen-openapi pr \
            --contract    apig/api-contract.yaml \
            --detected    /tmp/api-contract.detected.yaml \
            --apig-config apig/apig-config.yaml \
            --openapi     apig/openAPI.yaml \
            --base        "${{ github.event.repository.default_branch }}" \
            --branch      "gen-openapi/api-contract-drift-${{ github.run_id }}-${{ github.run_attempt }}" \
            --title       "Update API contract from detected drift"
`

const actionsDeployWorkflow = `# Manual APIG deployment: render the reviewed contract and import openAPI.yaml
# into Huawei Cloud APIG.
name: apig-deploy

on:
  workflow_dispatch:

permissions:
  contents: read

jobs:
  deploy:
    runs-on: ubuntu-latest
    environment:
      name: apig-production
    steps:
      - name: Checkout service repo
        uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'

      - name: Checkout gen-openapi tool and catalog
        uses: actions/checkout@v4
        with:
          repository: ${{ vars.GEN_OPENAPI_REPO }}
          ref: ${{ vars.GEN_OPENAPI_REF || 'main' }}
          path: gen-openapi
          token: ${{ secrets.GEN_OPENAPI_TOKEN || github.token }}

      - name: Install gen-openapi
        working-directory: gen-openapi
        run: go install .

      - name: Dry-run APIG import
        run: |
          gen-openapi apig-import \
            --contract    apig/api-contract.yaml \
            --apig-config apig/apig-config.yaml \
            --out         /tmp/openAPI.yaml \
            --catalog     "${API_CATALOG_PATH:-gen-openapi/catalog/catalog.yaml}" \
            --service     "${{ github.event.repository.name }}" \
            --dry-run
        env:
          API_CATALOG_PATH: ${{ vars.API_CATALOG_PATH }}

      - name: Import to Huawei Cloud APIG
        env:
          HUAWEICLOUD_SDK_AK: ${{ secrets.HUAWEICLOUD_SDK_AK }}
          HUAWEICLOUD_SDK_SK: ${{ secrets.HUAWEICLOUD_SDK_SK }}
          API_CATALOG_PATH: ${{ vars.API_CATALOG_PATH }}
        run: |
          gen-openapi apig-import \
            --contract    apig/api-contract.yaml \
            --apig-config apig/apig-config.yaml \
            --out         /tmp/openAPI.yaml \
            --catalog     "${API_CATALOG_PATH:-gen-openapi/catalog/catalog.yaml}" \
            --service     "${{ github.event.repository.name }}"
`
