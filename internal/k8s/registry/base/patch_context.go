package base

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// PatchData holds the patch information extracted from the request
type PatchData struct {
	PatchType types.PatchType
	Data      []byte
	Resource  string
	Name      string
	Namespace string
	// ExtractedObject holds the patched object extracted from objInfo (alternative to manual patching)
	ExtractedObject runtime.Object
}

// Context keys for patch data
type contextKey string

const (
	patchDataKey contextKey = "netguard-patch-data"
)

// WithPatchData stores patch data in the request context
func WithPatchData(ctx context.Context, patchData *PatchData) context.Context {
	return context.WithValue(ctx, patchDataKey, patchData)
}

// PatchDataFrom extracts patch data from the request context
func PatchDataFrom(ctx context.Context) (*PatchData, bool) {
	patchData, ok := ctx.Value(patchDataKey).(*PatchData)
	return patchData, ok
}
