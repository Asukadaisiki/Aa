// Package config loads the minimal user-facing provider configuration.
package config

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"strings"
	"time"

	"a2aagent/agent/provider"
	"a2aagent/agent/provider/openai"
	"gopkg.in/yaml.v3"
)

const ProviderDeepSeek = "deepseek"

// Config intentionally contains only the four values that users need to
// select. The API key is read from DEEPSEEK_API_KEY rather than stored in YAML.
type Config struct {
	Provider       string `yaml:"provider"`
	Model          string `yaml:"model"`
	URL            string `yaml:"url"`
	ThinkingEffort string `yaml:"thinking-effort"`
	APIKey         string `yaml:"api_key"`
}

const apiKeyEnvironmentVariable = "DEEPSEEK_API_KEY"

// Load reads and validates a YAML configuration file.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %q: %w", path, err)
	}
	config, err := Parse(data)
	if err != nil {
		return Config{}, fmt.Errorf("parse config %q: %w", path, err)
	}
	return config, nil
}

func LoadConfig(path string) (Config, error) { return Load(path) }

// Parse decodes one strict YAML document. Unknown fields are rejected so a
// misspelled minimal setting cannot silently fall back to a default.
func Parse(data []byte) (Config, error) {
	var config Config
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("decode YAML: %w", err)
	}
	var extra any
	err := decoder.Decode(&extra)
	if err == nil {
		return Config{}, fmt.Errorf("config must contain a single YAML document")
	}
	if err != io.EOF {
		return Config{}, fmt.Errorf("decode YAML document: %w", err)
	}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.Provider) != ProviderDeepSeek {
		return fmt.Errorf("provider must be %q, got %q", ProviderDeepSeek, c.Provider)
	}
	if strings.TrimSpace(c.Model) == "" {
		return fmt.Errorf("model is required")
	}
	parsedURL, err := neturl.Parse(strings.TrimSpace(c.URL))
	if err != nil || (parsedURL.Scheme != "https" && parsedURL.Scheme != "http") || parsedURL.Host == "" || parsedURL.Path == "" || parsedURL.Path == "/" {
		return fmt.Errorf("url must be a complete HTTP(S) request URL")
	}
	switch strings.TrimSpace(c.ThinkingEffort) {
	case "disabled", "high", "max":
		return nil
	default:
		return fmt.Errorf("thinking-effort must be disabled, high, or max")
	}
}

// NewProvider constructs the OpenAI-protocol DeepSeek client. Streaming is
// selected by the caller through Provider.Stream; no extra YAML switch is
// needed.
func (c Config) NewProvider() (provider.Provider, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	apiKey := c.APIKey
	if strings.TrimSpace(apiKey) == "" {
		apiKey = os.Getenv(apiKeyEnvironmentVariable)
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("%s environment variable is required", apiKeyEnvironmentVariable)
	}
	return openai.New(openai.Config{
		APIKey:     apiKey,
		URL:        c.URL,
		Model:      c.Model,
		HTTPClient: &http.Client{Timeout: 120 * time.Second},
	})
}

// Request creates a multi-turn OpenAI-protocol request. The caller supplies
// the complete message history on every turn.
func (c Config) Request(messages []provider.Message, tools []provider.ToolDefinition) provider.Request {
	request := provider.Request{
		Model:     c.Model,
		Messages:  append([]provider.Message(nil), messages...),
		Tools:     append([]provider.ToolDefinition(nil), tools...),
		MaxTokens: 4096,
		Thinking:  &provider.Thinking{Type: "enabled"},
	}
	switch c.ThinkingEffort {
	case "disabled":
		request.Thinking = &provider.Thinking{Type: "disabled"}
		request.Temperature = float64Pointer(0.7)
	case "high", "max":
		request.ReasoningEffort = c.ThinkingEffort
	}
	return request
}

func float64Pointer(value float64) *float64 { return &value }
