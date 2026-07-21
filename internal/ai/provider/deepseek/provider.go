// Package deepseek configures the shared OpenAI-compatible Chat Completions
// implementation for DeepSeek. It intentionally does not own a provider
// registry, model catalog, or a second Provider abstraction.
package deepseek

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/yuanbohan/pia/internal/ai"
	"github.com/yuanbohan/pia/internal/ai/provider/openaicompatible"
)

// DefaultEndpoint is DeepSeek's streamed Chat Completions endpoint. Callers
// may override it explicitly for compatible gateways and local development.
const DefaultEndpoint = "https://api.deepseek.com/chat/completions"

// Config contains the DeepSeek-specific construction inputs.
type Config struct {
	HTTPClient *http.Client
	Endpoint   string
	APIKey     string
	Model      string

	ReasoningEffort        string
	AllowInsecureLocalhost bool
}

// New validates DeepSeek configuration and returns the shared wire-protocol
// implementation behind the model-neutral ai.Provider port.
func New(config Config) (ai.Provider, error) {
	if strings.TrimSpace(config.APIKey) == "" {
		return nil, fmt.Errorf("deepseek: API key is required")
	}
	if strings.TrimSpace(config.Model) == "" {
		return nil, fmt.Errorf("deepseek: model is required")
	}

	endpoint := config.Endpoint
	if strings.TrimSpace(endpoint) == "" {
		endpoint = DefaultEndpoint
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("deepseek: parse endpoint: %w", err)
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme == "http" {
		if !config.AllowInsecureLocalhost || !isLoopbackHost(parsed.Hostname()) {
			return nil, fmt.Errorf(
				"deepseek: plaintext HTTP is allowed only for loopback endpoints with AllowInsecureLocalhost",
			)
		}
	}

	return openaicompatible.New(openaicompatible.Config{
		HTTPClient: config.HTTPClient,
		Endpoint:   parsed.String(),
		APIKey:     config.APIKey,
		Model:      config.Model,
		Profile: openaicompatible.Profile{
			ReplayReasoning: true,
			StreamUsage:     true,
			Thinking:        true,
		},
		ReasoningEffort: config.ReasoningEffort,
	})
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}
