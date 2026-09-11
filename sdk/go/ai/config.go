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
const (
	hanzoBaseURL = "https://api.hanzo.ai/v1"

	// The model the MCP runtime also defaults to.
	hanzoModel = "zen3-vl"
)

// DefaultConfig reads the environment. HANZO_API_KEY selects api.hanzo.ai;
// otherwise OPENROUTER_API_KEY, then OPENAI_API_KEY, selects that provider.
// AI_BASE_URL and AI_MODEL override the result.
func DefaultConfig() *Config {
	apiKey := os.Getenv("HANZO_API_KEY")
	baseURL := hanzoBaseURL
	model := hanzoModel

	// OpenRouter ahead of OpenAI where both are set.
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
