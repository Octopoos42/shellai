// Package config handles loading and validating the server's YAML configuration file.
// Sensitive values (database URL, admin credentials, LLM API keys) are intentionally
// excluded from the config file and must be provided via environment variables.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Port int `yaml:"port"`
}

// LLMConfig holds the configuration for a single LLM provider.
// The actual API key is read at runtime from the environment variable named by EnvKey.
type LLMConfig struct {
	Name     string `yaml:"name"`
	Endpoint string `yaml:"endpoint"` // full API endpoint URL
	Model    string `yaml:"model"`
	EnvKey   string `yaml:"envkey"`
	ContextK int    `yaml:"context_k"` // context window size in k-tokens
	LimitK   int    `yaml:"limit_k"`   // safe working limit in k-tokens
}

// AgentConfig controls the agentic loop behaviour.
type AgentConfig struct {
	// MaxIterations caps the number of planner→tool cycles per request (default: 10).
	MaxIterations int `yaml:"max_iterations"`
	// ToolConfirmTimeoutSecs is how long the agent waits for the user to approve
	// or reject a tool call before giving up (default: 300).
	ToolConfirmTimeoutSecs int `yaml:"tool_confirm_timeout_secs"`
}

// WithDefaults returns AgentConfig with sensible fallbacks applied.
func (a AgentConfig) WithDefaults() AgentConfig {
	if a.MaxIterations <= 0 {
		a.MaxIterations = 10
	}
	if a.ToolConfirmTimeoutSecs <= 0 {
		a.ToolConfirmTimeoutSecs = 300
	}
	return a
}

// Config is the root structure parsed from the YAML config file.
type Config struct {
	Server       ServerConfig `yaml:"server"`
	DefaultModel string       `yaml:"default_model"` // name of the LLM to use when unspecified
	LLMs         []LLMConfig  `yaml:"llm"`
	Agent        AgentConfig  `yaml:"agent"`
}

// FindLLM returns the LLMConfig with the given name, or false if not found.
func (c *Config) FindLLM(name string) (LLMConfig, bool) {
	for _, llm := range c.LLMs {
		if llm.Name == name {
			return llm, true
		}
	}
	return LLMConfig{}, false
}

// Load reads and parses the YAML config file at the given path.
func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open config file: %w", err)
	}
	defer func() { _ = f.Close() }()

	var cfg Config
	if err := yaml.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}
	return &cfg, nil
}
