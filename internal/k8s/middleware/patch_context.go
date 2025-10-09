package middleware

import (
	"context"
)

type patchBodyKeyType struct{}

var patchBodyKey = patchBodyKeyType{}

// PatchBodyData holds the PATCH request body and content type
type PatchBodyData struct {
	Body        []byte
	ContentType string
}

// WithPatchBody stores PATCH body data in context
func WithPatchBody(ctx context.Context, data *PatchBodyData) context.Context {
	return context.WithValue(ctx, patchBodyKey, data)
}

// PatchBodyFrom retrieves PATCH body data from context
func PatchBodyFrom(ctx context.Context) (*PatchBodyData, bool) {
	data, ok := ctx.Value(patchBodyKey).(*PatchBodyData)
	return data, ok
}
