package agenttargets

import (
	"os"
)

// Variant represents an alternative API endpoint for Claude-compatible providers.
// Variants allow switching between models/providers by redirecting API URLs.
type Variant struct {
	Name        string            // Human-readable name
	Description string            // What this variant provides
	Env         map[string]string // Environment variables to set
	SecretEnv   map[string]string // Maps target env var to source secret name
}

// BuiltinVariants provides pre-configured variants for popular Claude-compatible APIs.
var BuiltinVariants = map[string]Variant{
	"kimi-thinking": {
		Name:        "Kimi K1.5 (Moonshot)",
		Description: "Moonshot's Kimi K1.5 with extended thinking capabilities",
		Env: map[string]string{
			"ANTHROPIC_BASE_URL": "https://api.moonshot.cn/anthropic/v1",
		},
		SecretEnv: map[string]string{
			"ANTHROPIC_API_KEY": "MOONSHOT_API_KEY",
		},
	},
	"glm-4.7": {
		Name:        "GLM-4.7 (Zhipu)",
		Description: "Zhipu's GLM-4.7 via Anthropic-compatible API",
		Env: map[string]string{
			"ANTHROPIC_BASE_URL": "https://open.bigmodel.cn/api/anthropic/v1",
		},
		SecretEnv: map[string]string{
			"ANTHROPIC_API_KEY": "ZHIPU_API_KEY",
		},
	},
	"deepseek-r1": {
		Name:        "DeepSeek R1",
		Description: "DeepSeek's reasoning model via Anthropic-compatible API",
		Env: map[string]string{
			"ANTHROPIC_BASE_URL": "https://api.deepseek.com/anthropic/v1",
		},
		SecretEnv: map[string]string{
			"ANTHROPIC_API_KEY": "DEEPSEEK_API_KEY",
		},
	},
	"openrouter": {
		Name:        "OpenRouter",
		Description: "OpenRouter's unified API for multiple models",
		Env: map[string]string{
			"ANTHROPIC_BASE_URL": "https://openrouter.ai/api/v1",
		},
		SecretEnv: map[string]string{
			"ANTHROPIC_API_KEY": "OPENROUTER_API_KEY",
		},
	},
}

// GetVariant returns a variant by name.
func GetVariant(name string) (Variant, bool) {
	v, ok := BuiltinVariants[name]
	return v, ok
}

// ListVariants returns all available variant names.
func ListVariants() []string {
	names := make([]string, 0, len(BuiltinVariants))
	for name := range BuiltinVariants {
		names = append(names, name)
	}
	return names
}

// ApplyVariant sets environment variables for a variant.
// Returns a cleanup function to restore the original environment.
func ApplyVariant(name string) (cleanup func(), err error) {
	v, ok := GetVariant(name)
	if !ok {
		return nil, nil // No variant, nothing to do
	}

	// Store original values for cleanup
	originals := make(map[string]string)

	// Apply direct env vars
	for key, value := range v.Env {
		originals[key] = os.Getenv(key)
		os.Setenv(key, value)
	}

	// Apply secret mappings (read from source, set to target)
	for targetKey, sourceKey := range v.SecretEnv {
		originals[targetKey] = os.Getenv(targetKey)
		if secret := os.Getenv(sourceKey); secret != "" {
			os.Setenv(targetKey, secret)
		}
	}

	// Return cleanup function
	return func() {
		for key, original := range originals {
			if original == "" {
				os.Unsetenv(key)
			} else {
				os.Setenv(key, original)
			}
		}
	}, nil
}

// VariantEnv returns the environment variables that would be set for a variant.
// This is useful for spawning subprocesses with the variant applied.
func VariantEnv(name string) map[string]string {
	v, ok := GetVariant(name)
	if !ok {
		return nil
	}

	env := make(map[string]string)

	// Copy direct env vars
	for key, value := range v.Env {
		env[key] = value
	}

	// Resolve secret mappings
	for targetKey, sourceKey := range v.SecretEnv {
		if secret := os.Getenv(sourceKey); secret != "" {
			env[targetKey] = secret
		}
	}

	return env
}
