package check

import (
	"bytes"
	"fmt"
	"os"

	"gen-openapi/internal/apig"
	"gen-openapi/internal/config"
	"gopkg.in/yaml.v3"
)

// Result describes the outcome of a render-drift check.
type Result struct {
	UpToDate bool
	Diff     string
}

// Run renders the contract + apig config in memory and compares it to the
// existing openAPI.yaml on disk. It does not write any files.
func Run(contractPath, apigPath, outputPath string) (*Result, error) {
	contractDoc, err := config.LoadContract(contractPath)
	if err != nil {
		return nil, err
	}

	apigCfg, err := config.LoadApigConfig(apigPath)
	if err != nil {
		return nil, err
	}

	doc, err := apig.Render(contractDoc, apigCfg)
	if err != nil {
		return nil, err
	}
	if err := apig.Validate(doc); err != nil {
		return nil, fmt.Errorf("rendered openAPI.yaml failed validation: %w", err)
	}

	rendered, err := yaml.Marshal(doc)
	if err != nil {
		return nil, err
	}

	existing, err := os.ReadFile(outputPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &Result{UpToDate: false, Diff: fmt.Sprintf("%s does not exist", outputPath)}, nil
		}
		return nil, err
	}

	if bytes.Equal(normalize(rendered), normalize(existing)) {
		return &Result{UpToDate: true}, nil
	}

	return &Result{UpToDate: false, Diff: shortDiff(existing, rendered)}, nil
}

func normalize(b []byte) []byte {
	return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
}

func shortDiff(existing, rendered []byte) string {
	const maxLines = 20
	exLines := bytes.Split(normalize(existing), []byte("\n"))
	reLines := bytes.Split(normalize(rendered), []byte("\n"))

	var buf bytes.Buffer
	limit := len(exLines)
	if len(reLines) > limit {
		limit = len(reLines)
	}

	shown := 0
	for i := 0; i < limit && shown < maxLines; i++ {
		var ex, re []byte
		if i < len(exLines) {
			ex = exLines[i]
		}
		if i < len(reLines) {
			re = reLines[i]
		}
		if !bytes.Equal(ex, re) {
			fmt.Fprintf(&buf, "  line %d:\n    have: %s\n    want: %s\n", i+1, ex, re)
			shown++
		}
	}
	if shown == 0 {
		buf.WriteString("  files differ but no per-line diff was produced; lengths differ\n")
	}
	return buf.String()
}
