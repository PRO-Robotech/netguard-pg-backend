package base

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SSAContext holds Server-Side Apply specific data and options
// that need to be passed between different storage methods
type SSAContext struct {
	// Data contains the raw YAML/JSON data from the SSA request
	Data []byte

	// FieldManager specifies which field manager is making the request
	FieldManager string

	// DryRun contains dry-run options (e.g. ["All"] for client dry-run, ["server"] for server dry-run)
	DryRun []string

	// Force indicates whether to force conflicts resolution
	Force bool

	// Options contains the original PatchOptions from the request
	Options *metav1.PatchOptions
}

// ssaContextKey is the type used for storing SSAContext in context.Context
type ssaContextKey struct{}

// WithSSAContext stores SSAContext in the given context
func WithSSAContext(ctx context.Context, ssaCtx *SSAContext) context.Context {
	return context.WithValue(ctx, ssaContextKey{}, ssaCtx)
}

// GetSSAContext retrieves SSAContext from the given context
// Returns the context and a boolean indicating whether it was found
func GetSSAContext(ctx context.Context) (*SSAContext, bool) {
	ssaCtx, ok := ctx.Value(ssaContextKey{}).(*SSAContext)
	return ssaCtx, ok
}

// NewSSAContext creates a new SSAContext from PatchOptions and data
func NewSSAContext(data []byte, options *metav1.PatchOptions) *SSAContext {
	ssaCtx := &SSAContext{
		Data:    data,
		Options: options,
	}

	if options != nil {
		ssaCtx.FieldManager = options.FieldManager
		ssaCtx.DryRun = options.DryRun
		ssaCtx.Force = options.Force != nil && *options.Force
	}

	return ssaCtx
}

// IsDryRun returns true if this is a dry-run operation
func (s *SSAContext) IsDryRun() bool {
	return len(s.DryRun) > 0
}

// IsServerDryRun returns true if this is specifically a server dry-run operation
func (s *SSAContext) IsServerDryRun() bool {
	for _, dryRun := range s.DryRun {
		if dryRun == metav1.DryRunAll {
			return true
		}
	}
	return false
}
