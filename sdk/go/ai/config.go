package ai

import (
	"errors"
	"os"
	"time"
)

// Config holds AI/LLM configuration for making API calls.
type Config struct {
	// API Key for OpenAI or OpenRouter
	APIKey string

	// BaseURL can be either OpenAI or OpenRouter endpoint
	// Default: https://api.openai.com/v1
	// OpenRouter: https://openrouter.ai/api/v1
	BaseURL string

	// Default model to use (e.g., "gpt-4o", "openai/gpt-4o" for OpenRouter)
	Model string

	// Default temperature for responses (0.0 to 2.0)
	Temperature float64

	// Default max tokens for responses
	MaxTokens int

	// HTTP timeout for requests
	Timeout time.Duration

	// Optional: Site URL for OpenRouter rankings
	SiteURL string

	// Optional: Site name for OpenRouter rankings
	SiteName string
}

// Where a client points when nobody says otherwise.
//
// api.hanzo.ai, because this is Hanzo's SDK. The default used to be
// api.openai.com, which meant a caller who set an API key and nothing else
// sent that key to another company — and the example in this package had to
// set BaseURL by hand to avoid teaching it.
const (
	hanzoBaseURL = "https://api.hanzo.ai/v1"

	// The same default the MCP runtime carries (rust/src/tools), so one
	// estate answers with one model rather than each client choosing.
	hanzoModel = "zen3-vl"
)

// DefaultConfig returns a Config with sensible defaults, read from the
// environment.
//
// Ours first: HANZO_API_KEY selects api.hanzo.ai. A third-party key is an
// explicit choice and is still honoured — OPENAI_API_KEY or OPENROUTER_API_KEY
// each select that provider's endpoint and a model it serves — so every setup
// that worked before still works, and the only behaviour that changed is the
// one where nobody had chosen at all.
//
// AI_BASE_URL and AI_MODEL override whichever of those was selected.
func DefaultConfig() *Config {
	apiKey := os.Getenv("HANZO_API_KEY")
	baseURL := hanzoBaseURL
	model := hanzoModel

	// Only where our own key is absent, so setting HANZO_API_KEY is enough to
	// mean it and a stale third-party key in a shell cannot redirect it.
	// OpenRouter ahead of OpenAI where both are set, which is the precedence
	// this package already had and has a test naming it.
	if apiKey == "" {
		if k := os.Getenv("OPENROUTER_API_KEY"); k != "" {
			apiKey, baseURL, model = k, "https://openrouter.ai/api/v1", "openai/gpt-4o"
		} else if k := os.Getenv("OPENAI_API_KEY"); k != "" {
			apiKey, baseURL, model = k, "https://api.openai.com/v1", "gpt-4o"
		}
	}

	if customURL := os.Getenv("AI_BASE_URL"); customURL != "" {
		baseURL = customURL
	}
	if customModel := os.Getenv("AI_MODEL"); customModel != "" {
		model = customModel
	}

	return &Config{
		APIKey:      apiKey,
		BaseURL:     baseURL,
		Model:       model,
		Temperature: 0.7,
		MaxTokens:   4096,
		Timeout:     30 * time.Second,
	}
}

// Validate ensures the configuration is valid.
func (c *Config) Validate() error {
	if c.APIKey == "" {
		return errors.New("API key is required")
	}
	if c.BaseURL == "" {
		return errors.New("base URL is required")
	}
	if c.Model == "" {
		return errors.New("model is required")
	}
	return nil
}

// IsOpenRouter returns true if the base URL is for OpenRouter.
func (c *Config) IsOpenRouter() bool {
	return c.BaseURL == "https://openrouter.ai/api/v1" ||
		c.BaseURL == "https://openrouter.ai/api/v1/"
}
