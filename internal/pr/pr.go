package pr

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gen-openapi/internal/apig"
	"gen-openapi/internal/apply"
	"gen-openapi/internal/config"
	"gen-openapi/internal/diff"
	"gen-openapi/internal/output"
)

const defaultTitle = "Update API contract from detected drift"

var branchNameRe = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)

type Options struct {
	ContractPath    string
	DetectedPath    string
	ApigConfigPath  string
	OpenAPIPath     string
	SkipRender      bool
	Prune           bool
	IncludeInternal bool
	Base            string
	Branch          string
	Remote          string
	Title           string
	Draft           bool
	Labels          []string
	DryRun          bool
	Now             time.Time
}

type Result struct {
	NoDrift      bool
	DryRun       bool
	Branch       string
	Base         string
	Title        string
	Body         string
	FilesUpdated []string
	PROutput     string
}

type Runner interface {
	Run(ctx context.Context, dir, name string, args []string, stdin string) (string, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, dir, name string, args []string, stdin string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("%s %s failed: %w\n%s", name, strings.Join(args, " "), err, out.String())
	}
	return out.String(), nil
}

func Run(ctx context.Context, opts Options, runner Runner) (*Result, error) {
	if runner == nil {
		runner = ExecRunner{}
	}
	normalizeOptions(&opts)

	repoRoot, err := gitOutput(ctx, runner, "", "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, err
	}
	repoRoot = strings.TrimSpace(repoRoot)

	if err := ensureCleanTree(ctx, runner, repoRoot, opts.DetectedPath); err != nil {
		return nil, err
	}

	canonical, err := config.LoadContract(opts.ContractPath)
	if err != nil {
		return nil, fmt.Errorf("load canonical: %w", err)
	}
	detected, err := config.LoadContractLoose(opts.DetectedPath)
	if err != nil {
		return nil, fmt.Errorf("load detected: %w", err)
	}
	drift := diff.Compare(canonical, detected)
	if drift.IsEmpty() {
		return &Result{NoDrift: true, DryRun: opts.DryRun}, nil
	}

	merged, _, err := apply.Merge(canonical, detected, apply.Options{Prune: opts.Prune, IncludeInternal: opts.IncludeInternal})
	if err != nil {
		return nil, err
	}

	branch := opts.Branch
	if branch == "" {
		branch = GenerateBranchName(opts.Now)
	}
	if !ValidBranchName(branch) {
		return nil, fmt.Errorf("invalid branch name %q", branch)
	}
	base := opts.Base
	if base == "" {
		base, err = currentBranch(ctx, runner, repoRoot)
		if err != nil {
			return nil, err
		}
	}

	files := []string{opts.ContractPath}
	var rendered any
	if !opts.SkipRender {
		apigCfg, err := config.LoadApigConfig(opts.ApigConfigPath)
		if err != nil {
			return nil, fmt.Errorf("load apig config: %w", err)
		}
		rendered, err = apig.Render(merged, apigCfg)
		if err != nil {
			return nil, fmt.Errorf("render openapi: %w", err)
		}
		files = append(files, opts.OpenAPIPath)
	}

	body := Body(drift, opts, files)
	res := &Result{DryRun: opts.DryRun, Branch: branch, Base: base, Title: opts.Title, Body: body, FilesUpdated: files}
	if opts.DryRun {
		return res, nil
	}

	if _, err := runner.Run(ctx, repoRoot, "git", []string{"checkout", "-b", branch}, ""); err != nil {
		return nil, err
	}
	if err := output.WriteYAML(opts.ContractPath, merged); err != nil {
		return nil, fmt.Errorf("write contract: %w", err)
	}
	if !opts.SkipRender {
		if err := output.WriteYAML(opts.OpenAPIPath, rendered); err != nil {
			return nil, fmt.Errorf("write openapi: %w", err)
		}
	}
	if _, err := runner.Run(ctx, repoRoot, "git", append([]string{"add"}, files...), ""); err != nil {
		return nil, err
	}
	if _, err := runner.Run(ctx, repoRoot, "git", []string{"diff", "--cached", "--quiet"}, ""); err == nil {
		return nil, fmt.Errorf("merged contract produced no staged changes; no PR created")
	}
	if _, err := runner.Run(ctx, repoRoot, "git", []string{"commit", "-m", opts.Title}, ""); err != nil {
		return nil, err
	}
	if _, err := runner.Run(ctx, repoRoot, "git", []string{"push", "--set-upstream", opts.Remote, branch}, ""); err != nil {
		return nil, err
	}
	args := []string{"pr", "create", "--base", base, "--head", branch, "--title", opts.Title, "--body-file", "-"}
	if opts.Draft {
		args = append(args, "--draft")
	}
	for _, label := range opts.Labels {
		args = append(args, "--label", label)
	}
	out, err := runner.Run(ctx, repoRoot, "gh", args, body)
	if err != nil {
		return nil, err
	}
	res.PROutput = strings.TrimSpace(out)
	return res, nil
}

func normalizeOptions(o *Options) {
	if o.ContractPath == "" {
		o.ContractPath = "api/api-contract.yaml"
	}
	if o.DetectedPath == "" {
		o.DetectedPath = "api/api-contract.detected.yaml"
	}
	if o.ApigConfigPath == "" {
		o.ApigConfigPath = "api/apig-config.yaml"
	}
	if o.OpenAPIPath == "" {
		o.OpenAPIPath = "api/openAPI.yaml"
	}
	if o.Remote == "" {
		o.Remote = "origin"
	}
	if o.Title == "" {
		o.Title = defaultTitle
	}
	if len(o.Labels) == 0 {
		o.Labels = []string{"api-contract", "drift"}
	}
	if o.Now.IsZero() {
		o.Now = time.Now().UTC()
	}
}

func Body(report diff.Report, opts Options, files []string) string {
	var b strings.Builder
	b.WriteString(diff.Markdown(report))
	b.WriteString("\n---\n\nGenerated by `gen-openapi pr`.\n\nFiles updated:\n")
	for _, f := range files {
		fmt.Fprintf(&b, "- `%s`\n", f)
	}
	b.WriteString("\nReview notes:\n")
	b.WriteString("- Detected routes were merged conservatively.\n")
	b.WriteString("- Owner-authored fields such as auth, summary, description, and backendPath were preserved where possible.\n")
	b.WriteString("- Internal routes are skipped unless `--include-internal` was used.\n")
	b.WriteString("- This PR was created automatically but will not be merged automatically.\n")
	var flags []string
	if opts.Prune {
		flags = append(flags, "--prune")
	}
	if opts.IncludeInternal {
		flags = append(flags, "--include-internal")
	}
	if opts.SkipRender {
		flags = append(flags, "--skip-render")
	}
	if len(flags) > 0 {
		fmt.Fprintf(&b, "\nFlags used: `%s`\n", strings.Join(flags, " "))
	}
	return b.String()
}

func GenerateBranchName(now time.Time) string {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return "gen-openapi/api-contract-drift-" + now.UTC().Format("20060102-150405")
}

func ValidBranchName(name string) bool {
	return name != "" && !strings.Contains(name, "..") && !strings.HasPrefix(name, "/") && !strings.HasSuffix(name, "/") && branchNameRe.MatchString(name)
}

func ensureCleanTree(ctx context.Context, runner Runner, repoRoot, detectedPath string) error {
	out, err := runner.Run(ctx, repoRoot, "git", []string{"status", "--porcelain=v1", "--untracked-files=all"}, "")
	if err != nil {
		return err
	}
	return CheckDirtyStatus(out, detectedPath)
}

func CheckDirtyStatus(status, detectedPath string) error {
	detectedClean := filepath.ToSlash(filepath.Clean(detectedPath))
	for _, line := range strings.Split(strings.TrimSpace(status), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if strings.HasPrefix(line, "?? ") {
			p := filepath.ToSlash(filepath.Clean(strings.TrimSpace(strings.TrimPrefix(line, "?? "))))
			if p == detectedClean {
				continue
			}
		}
		return fmt.Errorf("working tree is dirty; refusing to create PR branch (first dirty entry: %q)", line)
	}
	return nil
}

func currentBranch(ctx context.Context, runner Runner, repoRoot string) (string, error) {
	out, err := runner.Run(ctx, repoRoot, "git", []string{"branch", "--show-current"}, "")
	if err != nil {
		return "", err
	}
	branch := strings.TrimSpace(out)
	if branch == "" {
		return "", fmt.Errorf("unable to determine current branch; pass --base")
	}
	return branch, nil
}

func gitOutput(ctx context.Context, runner Runner, dir, subcmd string, args ...string) (string, error) {
	return runner.Run(ctx, dir, "git", append([]string{subcmd}, args...), "")
}
