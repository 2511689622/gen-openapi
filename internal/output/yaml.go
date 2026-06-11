package output

import (
	"os"

	"gopkg.in/yaml.v3"
)

func WriteYAML(path string, v any) error {
	b, err := yaml.Marshal(v)
	if err != nil {
		return err
	}

	return os.WriteFile(path, b, 0o644)
}
