package config

import (
	"testing"

	"a2aagent/agent/provider"
)

func TestParseMinimalConfig(t *testing.T) {
	t.Setenv(apiKeyEnvironmentVariable, "secret")
	config, err := Parse([]byte(`
provider: deepseek
model: custom-model
url: https://example.test/v1/chat/completions
thinking-effort: max
`))
	if err != nil {
		t.Fatal(err)
	}
	if config.Provider != ProviderDeepSeek || config.Model != "custom-model" || config.ThinkingEffort != "max" {
		t.Fatalf("unexpected config: %+v", config)
	}
	client, err := config.NewProvider()
	if err != nil {
		t.Fatal(err)
	}
	if client.Name() != ProviderDeepSeek {
		t.Fatalf("provider name = %q", client.Name())
	}
	request := config.Request([]provider.Message{{Role: provider.RoleUser, Content: "hello"}}, nil)
	if request.Model != "custom-model" || request.ReasoningEffort != "max" || request.Temperature != nil {
		t.Fatalf("unexpected request: %+v", request)
	}
}

func TestParseRejectsUnknownFieldAndBaseURL(t *testing.T) {
	_, err := Parse([]byte(`
provider: deepseek
model: custom-model
url: https://api.deepseek.com/chat/completions
thinking-effort: high
stream: true
`))
	if err == nil {
		t.Fatal("expected unknown field error")
	}

	_, err = Parse([]byte(`
provider: deepseek
model: custom-model
url: https://api.deepseek.com
thinking-effort: high
`))
	if err == nil {
		t.Fatal("expected base URL to be rejected")
	}
}

func TestDisabledThinkingUsesTemperature(t *testing.T) {
	config := Config{Provider: "deepseek", Model: "model", URL: "https://example.test/chat/completions", ThinkingEffort: "disabled"}
	request := config.Request(nil, nil)
	if request.ReasoningEffort != "" || request.Temperature == nil || *request.Temperature != 0.7 {
		t.Fatalf("unexpected disabled-thinking request: %+v", request)
	}
}

func TestConfigAPIKeyTakesPriorityOverEnvironment(t *testing.T) {
	t.Setenv(apiKeyEnvironmentVariable, "")
	config := Config{
		Provider:       ProviderDeepSeek,
		Model:          "model",
		URL:            "https://example.test/chat/completions",
		ThinkingEffort: "disabled",
		APIKey:         "config-key",
	}
	if config.APIKey == "" {
		t.Fatal("config API key should be available")
	}
	if _, err := config.NewProvider(); err != nil {
		t.Fatalf("config API key should be used before environment fallback: %v", err)
	}
}

func TestEffectiveContextWindowUsesConfiguredAndKnownDefaults(t *testing.T) {
	known := Config{Model: "deepseek-v4-flash"}
	if got := known.EffectiveContextWindow(); got != 1_000_000 {
		t.Fatalf("known context window = %d", got)
	}
	configured := Config{Model: "unknown", ContextWindow: 321}
	if got := configured.EffectiveContextWindow(); got != 321 {
		t.Fatalf("configured context window = %d", got)
	}
	unknown := Config{Model: "unknown"}
	if got := unknown.EffectiveContextWindow(); got != 0 {
		t.Fatalf("unknown context window = %d", got)
	}
}
