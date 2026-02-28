package config

import (
	"fmt"
	"os"

	"github.com/bnaylor/perry/internal/dispatch"
	"github.com/bnaylor/perry/internal/policy"
	"gopkg.in/yaml.v3"
)

func LoadRoutingConfig(path string) (dispatch.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return dispatch.Config{}, fmt.Errorf("read routing config: %w", err)
	}
	var cfg dispatch.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return dispatch.Config{}, fmt.Errorf("parse routing config: %w", err)
	}
	return cfg, nil
}

func LoadPolicyConfig(path string) (policy.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return policy.Config{}, fmt.Errorf("read policy config: %w", err)
	}
	var cfg policy.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return policy.Config{}, fmt.Errorf("parse policy config: %w", err)
	}
	return cfg, nil
}
