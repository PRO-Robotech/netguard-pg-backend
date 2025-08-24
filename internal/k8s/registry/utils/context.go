package utils

import (
	"context"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"

	request "k8s.io/apiserver/pkg/endpoints/request"
)

// NamespaceFrom extracts the namespace from a request context.
// It checks, in order:
//  1. Explicit value stored under the key "namespace" (used in tests).
//  2. request.WithNamespace / request.NamespaceValue.
//  3. The RequestInfo object populated by the k8s request info filter.
//
// If no namespace can be determined it returns an empty string.
func NamespaceFrom(ctx context.Context) string {
	if ns, ok := ctx.Value("namespace").(string); ok && ns != "" {
		return ns
	}
	if ns := request.NamespaceValue(ctx); ns != "" {
		return ns
	}
	if ri, ok := request.RequestInfoFrom(ctx); ok && ri != nil {
		return ri.Namespace
	}
	return ""
}

// ScopeFromContext returns a ports.Scope that limits queries to the namespace
// carried in the request context. If the context has no namespace (cluster-wide
// request), it returns nil so that caller can list across all namespaces.
// It is intended to be passed to backendClient.List* helpers.
func ScopeFromContext(ctx context.Context) ports.Scope {
	ns := NamespaceFrom(ctx)
	if ns == "" {
		return nil
	}
	return ports.NewResourceIdentifierScope(
		models.NewResourceIdentifier("", models.WithNamespace(ns)),
	)
}
