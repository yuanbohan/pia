package ai

import "context"

// Provider starts one model response stream bound to ctx.
//
// The contract stays in the model-neutral ai package because Agent consumes
// it. Concrete implementations are grouped below internal/ai/provider and
// depend inward on this package.
type Provider interface {
	Stream(ctx context.Context, request Request) Stream
}
