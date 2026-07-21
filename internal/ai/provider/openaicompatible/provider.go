package openaicompatible

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"github.com/yuanbohan/pia/internal/ai"
)

const maxHTTPErrorBodyBytes = 64 << 10

// Config contains the stable wire-level configuration shared by
// OpenAI-compatible providers.
type Config struct {
	HTTPClient      *http.Client
	Endpoint        string
	APIKey          string
	Model           string
	Profile         Profile
	ReasoningEffort string
}

// Provider implements ai.Provider over an OpenAI-compatible streamed Chat
// Completions endpoint.
type Provider struct {
	httpClient      *http.Client
	endpoint        string
	apiKey          string
	model           string
	profile         Profile
	reasoningEffort string
}

var _ ai.Provider = (*Provider)(nil)

// New validates stable configuration without performing network I/O.
func New(config Config) (*Provider, error) {
	if strings.TrimSpace(config.Endpoint) == "" {
		return nil, fmt.Errorf("openai-compatible: endpoint is required")
	}
	endpoint, err := url.Parse(config.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("openai-compatible: parse endpoint: %w", err)
	}
	if !endpoint.IsAbs() || endpoint.Host == "" {
		return nil, fmt.Errorf("openai-compatible: endpoint must be an absolute URL")
	}
	if endpoint.Scheme != "http" && endpoint.Scheme != "https" {
		return nil, fmt.Errorf("openai-compatible: endpoint scheme must be http or https")
	}
	if endpoint.User != nil {
		return nil, fmt.Errorf("openai-compatible: endpoint must not contain user information")
	}
	if endpoint.Fragment != "" {
		return nil, fmt.Errorf("openai-compatible: endpoint must not contain a fragment")
	}
	if strings.TrimSpace(config.Model) == "" {
		return nil, fmt.Errorf("openai-compatible: model is required")
	}
	if strings.ContainsAny(config.APIKey, "\r\n") {
		return nil, fmt.Errorf("openai-compatible: API key contains an invalid header character")
	}

	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &Provider{
		httpClient:      httpClient,
		endpoint:        endpoint.String(),
		apiKey:          config.APIKey,
		model:           config.Model,
		profile:         config.Profile,
		reasoningEffort: config.ReasoningEffort,
	}, nil
}

// Stream freezes request ownership immediately and defers all network I/O
// until the first Receive call.
func (provider *Provider) Stream(ctx context.Context, request ai.Request) ai.Stream {
	stream := &responseStream{
		ctx:      ctx,
		provider: provider,
		request:  ai.CloneRequest(request),
	}
	if ctx == nil {
		stream.setupErr = fmt.Errorf("openai-compatible: context is nil")
	}
	return stream
}

type responseStream struct {
	ctx      context.Context
	provider *Provider
	request  ai.Request

	setupErr error
	started  bool
	terminal bool
	response *http.Response

	sse          *sseReader
	queue        []ai.Event
	content      []*contentState
	tools        map[int]*toolCallState
	usage        ai.Usage
	finishReason ai.StopReason
}

func (stream *responseStream) start() (ai.Event, error) {
	if stream.setupErr != nil {
		return stream.fail(stream.setupErr)
	}
	if cause := context.Cause(stream.ctx); cause != nil {
		return stream.fail(cause)
	}

	payload, err := buildRequestPayload(
		stream.provider.model,
		stream.request,
		stream.provider.profile,
		stream.provider.reasoningEffort,
	)
	if err != nil {
		return stream.fail(fmt.Errorf("openai-compatible: build request: %w", err))
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return stream.fail(fmt.Errorf("openai-compatible: encode request: %w", err))
	}

	request, err := http.NewRequestWithContext(
		stream.ctx,
		http.MethodPost,
		stream.provider.endpoint,
		bytes.NewReader(body),
	)
	if err != nil {
		return stream.fail(fmt.Errorf("openai-compatible: create HTTP request: %w", err))
	}
	request.Header.Set("Accept", "text/event-stream")
	request.Header.Set("Content-Type", "application/json")
	if stream.provider.apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+stream.provider.apiKey)
	}

	response, err := stream.provider.httpClient.Do(request)
	if err != nil {
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		return stream.fail(fmt.Errorf("openai-compatible: send request: %w", err))
	}
	stream.response = response
	if response.Body == nil {
		return stream.fail(fmt.Errorf("openai-compatible: response body is nil"))
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return stream.fail(stream.httpStatusError(response))
	}

	contentType := response.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || !strings.EqualFold(mediaType, "text/event-stream") {
		return stream.fail(fmt.Errorf(
			"openai-compatible: response Content-Type %q is not text/event-stream",
			contentType,
		))
	}

	return ai.StartEvent{}, nil
}

func (stream *responseStream) httpStatusError(response *http.Response) error {
	limited, err := io.ReadAll(io.LimitReader(response.Body, maxHTTPErrorBodyBytes+1))
	if err != nil {
		return fmt.Errorf(
			"openai-compatible: HTTP status %d; read error body: %w",
			response.StatusCode,
			err,
		)
	}

	truncated := len(limited) > maxHTTPErrorBodyBytes
	if truncated {
		limited = limited[:maxHTTPErrorBodyBytes]
	}
	detail := strings.TrimSpace(string(limited))
	if detail == "" {
		detail = http.StatusText(response.StatusCode)
	}
	if truncated {
		detail += " [truncated]"
	}
	return fmt.Errorf("openai-compatible: HTTP status %d: %s", response.StatusCode, detail)
}

func (stream *responseStream) fail(err error) (ai.Event, error) {
	stream.closeResponse()
	stream.queue = nil
	stream.terminal = true

	stopReason := ai.StopReasonError
	errorMessage := err.Error()
	if stream.ctx != nil && context.Cause(stream.ctx) != nil {
		stopReason = ai.StopReasonAborted
		errorMessage = context.Cause(stream.ctx).Error()
	}
	message := stream.terminalMessage(stopReason, stream.hasFinishReason())
	message.ErrorMessage = redactCredential(errorMessage, stream.provider.apiKey)
	return ai.ErrorEvent{
		Message: message,
	}, nil
}

func (stream *responseStream) closeResponse() {
	if stream.response == nil || stream.response.Body == nil {
		return
	}
	_ = stream.response.Body.Close()
	stream.response.Body = nil
}

func redactCredential(message, credential string) string {
	if credential == "" {
		return message
	}
	return strings.ReplaceAll(message, credential, "[REDACTED]")
}
