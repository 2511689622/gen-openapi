package config

import (
	"bytes"
	"fmt"
	"os"

	"gen-openapi/pkg/contract"
	"gopkg.in/yaml.v3"
)

func LoadContract(path string) (*contract.ApiContract, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var c contract.ApiContract
	if err := decodeStrict(b, &c); err != nil {
		return nil, fmt.Errorf("load contract %s: %w", path, err)
	}
	if err := ValidateContract(&c); err != nil {
		return nil, fmt.Errorf("validate contract %s: %w", path, err)
	}

	return &c, nil
}

// LoadContractLoose parses the YAML but skips ValidateContract. It is meant
// for candidate / detected contracts produced by the discover/import
// adapters, which routinely violate strict invariants (e.g. they may not
// declare path parameters that appear in the path). diff and other
// read-only consumers should use this; render/check must keep using
// LoadContract so the canonical contract stays sound.
func LoadContractLoose(path string) (*contract.ApiContract, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var c contract.ApiContract
	if err := decodeStrict(b, &c); err != nil {
		return nil, fmt.Errorf("load contract %s: %w", path, err)
	}

	return &c, nil
}

func LoadApigConfig(path string) (*HuaweiApigConfig, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var c HuaweiApigConfig
	if err := decodeStrict(b, &c); err != nil {
		return nil, fmt.Errorf("load apig config %s: %w", path, err)
	}
	if err := ValidateApigConfig(&c); err != nil {
		return nil, fmt.Errorf("validate apig config %s: %w", path, err)
	}

	return &c, nil
}

func decodeStrict(b []byte, out any) error {
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	return dec.Decode(out)
}
