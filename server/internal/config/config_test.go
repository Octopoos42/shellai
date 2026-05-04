package config_test

import (
	"os"
	"testing"

	"github.com/Octopoos42/shellai/server/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_ValidFile(t *testing.T) {
	f, err := os.CreateTemp("", "config-*.yaml")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(f.Name()) })

	_, err = f.WriteString(`
server:
  port: 9090
default_model: claude
llm:
  - name: claude
    endpoint: https://api.anthropic.com/v1/messages
    model: claude-sonnet-4-6
    envkey: CLAUDE_API_KEY
    context_k: 128
    limit_k: 96
`)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	cfg, err := config.Load(f.Name())
	require.NoError(t, err)
	assert.Equal(t, 9090, cfg.Server.Port)
	assert.Equal(t, "claude", cfg.DefaultModel)
	require.Len(t, cfg.LLMs, 1)
	assert.Equal(t, "claude", cfg.LLMs[0].Name)
	assert.Equal(t, "https://api.anthropic.com/v1/messages", cfg.LLMs[0].Endpoint)
	assert.Equal(t, "CLAUDE_API_KEY", cfg.LLMs[0].EnvKey)
	assert.Equal(t, 128, cfg.LLMs[0].ContextK)

	llm, ok := cfg.FindLLM("claude")
	require.True(t, ok)
	assert.Equal(t, "claude-sonnet-4-6", llm.Model)

	_, ok = cfg.FindLLM("nonexistent")
	assert.False(t, ok)
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := config.Load("/nonexistent/path/config.yaml")
	assert.Error(t, err)
}

func TestLoad_InvalidYAML(t *testing.T) {
	f, err := os.CreateTemp("", "config-*.yaml")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(f.Name()) })

	_, err = f.WriteString("::invalid yaml::[[[")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	_, err = config.Load(f.Name())
	assert.Error(t, err)
}
