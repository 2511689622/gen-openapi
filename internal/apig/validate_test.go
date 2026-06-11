package apig

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestValidateFile_CanonicalExampleOpenAI(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path")
	}
	path := filepath.Join(filepath.Dir(filename), "..", "..", "example", "openAPI.yaml")
	if err := ValidateFile(path); err != nil {
		t.Fatalf("canonical example should validate: %v", err)
	}
}
