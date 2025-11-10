package apiserver

import (
	"bytes"
	"io"
	"net/http"

	"netguard-pg-backend/internal/k8s/middleware"

	"k8s.io/klog/v2"
)

// WithPatchBodyExtractor wraps an HTTP handler to extract PATCH request bodies
func WithPatchBodyExtractor(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodPatch {
			// Read the entire body
			bodyBytes, err := io.ReadAll(req.Body)
			if err != nil {
				klog.ErrorS(err, "Failed to read PATCH request body")
				http.Error(w, "Failed to read request body", http.StatusBadRequest)
				return
			}

			req.Body.Close()

			req.Body = io.NopCloser(bytes.NewReader(bodyBytes))

			// Store in context
			patchData := &middleware.PatchBodyData{
				Body:        bodyBytes,
				ContentType: req.Header.Get("Content-Type"),
			}
			ctx := middleware.WithPatchBody(req.Context(), patchData)
			req = req.WithContext(ctx)
		}

		handler.ServeHTTP(w, req)
	})
}
