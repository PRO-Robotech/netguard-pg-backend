package utils

import (
	"sort"

	"netguard-pg-backend/internal/domain/models"
)

// SortByNamespaceName sorts a slice in-place by Namespace then Name.
// idFn extracts ResourceIdentifier from the slice element.
// It works for any slice element type via a generic parameter.
func SortByNamespaceName[T any](items []T, idFn func(T) models.ResourceIdentifier) {
	sort.Slice(items, func(i, j int) bool {
		idI := idFn(items[i])
		idJ := idFn(items[j])
		if idI.Namespace != idJ.Namespace {
			return idI.Namespace < idJ.Namespace
		}
		return idI.Name < idJ.Name
	})
}
