# gen-openapi

Contract-driven Huawei Cloud APIG OpenAPI generator. Each service repo keeps
an explicit `api/api-contract.yaml` + `api/apig-config.yaml`; `gen-openapi`
deterministically compiles them into a Huawei-importable `api/openAPI.yaml`.
Scanners (`discover` / `import-openapi`) produce candidate contracts for
review — they never write to the canonical contract directly.

See [DESIGN.md](DESIGN.md) for the architecture.

## Install

```bash
go install github.com/YOUR_ORG/gen-openapi@latest
# or from a checkout:
go install ./...
```

## Quick start

```bash
# 1. Scaffold contract + APIG config templates
gen-openapi init --service my-service

# 2. Edit api-contract.yaml and apig-config.yaml by hand
#    (or pre-fill them via import-openapi / discover, then review)

# 3. Render the APIG-importable YAML
gen-openapi render \
  --contract    api/api-contract.yaml \
  --apig-config api/apig-config.yaml \
  --out         api/openAPI.yaml

# 4. Validate the rendered YAML against Huawei APIG's expectations
gen-openapi validate api/openAPI.yaml

# 5. Verify openAPI.yaml is still in sync with the contract (run in CI)
gen-openapi check \
  --contract    api/api-contract.yaml \
  --apig-config api/apig-config.yaml \
  --output      api/openAPI.yaml
```

## Commands

| Command | Purpose |
|---|---|
| `init` | Scaffold `api-contract.yaml` + `apig-config.yaml` templates |
| `render` | Compile contract + config → `openAPI.yaml` |
| `validate` | Check `openAPI.yaml` conforms to Huawei APIG OpenAPI 3.0.3 |
| `check` | Re-render in memory and verify the on-disk `openAPI.yaml` matches |
| `import-openapi` | Convert OpenAPI 3.x (file or URL — springdoc / FastAPI) → candidate |
| `apig-import` | Render and import reviewed `openAPI.yaml` into Huawei Cloud APIG via Huawei Go SDK |
| `catalog-check` | Check that a service is registered correctly in the central API catalog |
| `repo-bootstrap` | Clone a GitHub service repo and generate local API contract/APIG files |
| `discover` | OpenAPI-first discovery with lightweight Go/Gin static fallback |
| `diff` | Compare canonical contract vs candidate, produce a Markdown drift report |
| `apply` | Conservatively merge candidate changes into the canonical contract |
| `pr` | Create a reviewed GitHub PR from detected drift; never auto-merges |


### Bootstrap a GitHub service repo

`repo-bootstrap` is a local generation command. It clones a GitHub repo, discovers Go/Gin routes, and always writes `apig/api-contract.detected.yaml`. If `apig/api-contract.yaml` does not exist, it adopts the detected contract as the initial contract and renders `apig/openAPI.yaml`; if a contract already exists, it leaves it unchanged for diff/review. It does not write GitHub Actions, commit, push, open a PR, or import APIG; use `pr` and `apig-import` for those later steps.

```bash
gen-openapi repo-bootstrap \
  --repo https://github.com/org/my-service \
  --out ./demo/my-service
```

`--service` defaults to the repository name, and `--backend-address` defaults to `<service>.example.com`; pass them only when the generated names need to differ. Use `--api-dir` if you want a different subdirectory name (default: `apig`).

### Discover modes

By default, `discover` preserves the original static scanner behavior:

| `--lang` | `--framework` | Static strategy |
|---|---|---|
| `go` | `gin` | Lightweight AST scan for Gin route/group/path parameters with regex fallback |

Use `--mode auto` to enable OpenAPI-first discovery. Auto mode tries:

1. `--openapi-url`
2. `--base-url` runtime probes (`/openapi.json`, `/openapi.yaml`, `/v3/api-docs`)
3. `--source/docs/openapi.yaml`, `openapi.yml`, or `openapi.json`
4. the Go/Gin static scanner fallback

Java/Spring and Python/FastAPI services should expose or generate OpenAPI and feed it through `import-openapi` or `discover --mode auto`; static Java/Python source scanners are intentionally not included in the current core.

```bash
gen-openapi discover --mode auto \
  --openapi-url http://localhost:8080/v3/api-docs \
  --service my-service \
  --out api/api-contract.detected.yaml

gen-openapi discover --mode auto \
  --lang python \
  --framework fastapi \
  --base-url http://localhost:8000 \
  --service my-service \
  --out api/api-contract.detected.yaml
```


### Import into Huawei Cloud APIG

`apig-import` renders the reviewed canonical contract, validates the APIG OpenAPI file, then imports it into the APIG target configured in `apig-config.yaml` under `spec.importTarget`. Credentials are read from `HUAWEICLOUD_SDK_AK` and `HUAWEICLOUD_SDK_SK`; do not put secrets in YAML.

```bash
gen-openapi discover --lang go --framework gin \
  --source . \
  --service my-service \
  --out api/api-contract.detected.yaml

gen-openapi diff \
  --contract api/api-contract.yaml \
  --detected api/api-contract.detected.yaml

# After review, update api-contract.yaml, then render/import:
gen-openapi apig-import \
  --contract api/api-contract.yaml \
  --apig-config api/apig-config.yaml \
  --out api/openAPI.yaml \
  --dry-run

gen-openapi apig-import \
  --contract api/api-contract.yaml \
  --apig-config api/apig-config.yaml \
  --out api/openAPI.yaml
```

Required APIG import target fields:

```yaml
spec:
  importTarget:
    region: cn-north-4
    projectId: replace-with-project-id
    instanceId: replace-with-apig-instance-id
    groupId: replace-with-api-group-id
    apiMode: merge
    extendMode: merge
```

### Create reviewed drift PRs

`pr` consumes an existing detected candidate, applies it conservatively,
regenerates `openAPI.yaml` by default, and opens a GitHub pull request via
`gh pr create`. It refuses dirty working trees and **never auto-merges** —
service owners must review the PR.

```bash
gen-openapi pr \
  --contract    api/api-contract.yaml \
  --detected    api/api-contract.detected.yaml \
  --apig-config api/apig-config.yaml \
  --openapi     api/openAPI.yaml \
  --dry-run
```

Use `--prune` only when you intentionally want to remove canonical routes
missing from the detected candidate, and `--include-internal` only when an
internal route is explicitly approved for exposure.

### Internal path blacklist

Discover omits `/debug/*`, `/metrics`, `/internal/*`, `/healthz`, `/readyz`,
`/livez`, `/-/*`, `/admin/*`, `/.well-known/*` from the candidate by default
so they cannot be silently promoted to the public contract. Pass
`--include-internal` to keep them in the candidate when you genuinely intend to
publish one.

## API catalog

[`catalog/catalog.yaml`](catalog/catalog.yaml) shows the recommended centralized
index shape. It references each service repo's `api/api-contract.yaml`,
`api/apig-config.yaml`, and `api/openAPI.yaml`; it does not copy those contracts.

## CI templates

Drop these into each service repo:

- [`.github/workflows/apig-check.yml`](.github/workflows/apig-check.yml) —
  runs on every PR; fails if `openAPI.yaml` drifted from the contract.
- [`.github/workflows/apig-drift-scan.yml`](.github/workflows/apig-drift-scan.yml) —
  runs nightly; discovers routes from source, diffs against the canonical
  contract, and opens a reviewed PR when they disagree. **Never auto-merges** —
  human review is the whole point.
- [`.github/workflows/apig-deploy.yml`](.github/workflows/apig-deploy.yml) —
  manually renders, validates, and imports the reviewed `openAPI.yaml` into
  Huawei Cloud APIG using `HUAWEICLOUD_SDK_AK` / `HUAWEICLOUD_SDK_SK` secrets.

Optional catalog variables for service workflows:

| Variable / secret | Purpose |
|---|---|
| `vars.API_CATALOG_REPO` | Central catalog repo, for example `your-org/infra-api-catalog`; when unset, catalog checks are skipped. |
| `vars.API_CATALOG_PATH` | Optional catalog file path; defaults to `infra-api-catalog/catalog.yaml`. |
| `secrets.API_CATALOG_TOKEN` | Optional token for checking out a private catalog repo. |

## Development

```bash
gofmt -w main.go cmd internal pkg
go build ./...
go test ./internal/...
```

For local validation, use the test fixtures under
[`internal/testdata/software-package-server/`](internal/testdata/software-package-server/):

```bash
go run . render \
  --contract    internal/testdata/software-package-server/api-contract.yaml \
  --apig-config internal/testdata/software-package-server/apig-config.yaml \
  --out         /tmp/openAPI.yaml
go run . validate /tmp/openAPI.yaml

go run . diff \
  --contract internal/testdata/software-package-server/api-contract.yaml \
  --detected internal/testdata/software-package-server/api-contract.detected.yaml
```

Tests use only the standard library and `gopkg.in/yaml.v3`; committed fixtures
live under `internal/testdata/`.

## License

TBD.