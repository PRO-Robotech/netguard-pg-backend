package base

import (
	"fmt"
	"strconv"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ListMetadataBuilder is responsible for populating list metadata such as resourceVersion
// and serve as an extension point for future limit/continue support.
type ListMetadataBuilder struct {
	list  metav1.ListInterface
	items []runtime.Object
}

// NewListMetadataBuilder creates a builder for the provided list object.
func NewListMetadataBuilder(listObj runtime.Object) (*ListMetadataBuilder, error) {
	list, ok := listObj.(metav1.ListInterface)
	if !ok {
		return nil, fmt.Errorf("object %T does not implement metav1.ListInterface", listObj)
	}
	items, err := meta.ExtractList(listObj)
	if err != nil {
		return nil, fmt.Errorf("failed to extract list items: %w", err)
	}
	return &ListMetadataBuilder{
		list:  list,
		items: items,
	}, nil
}

// SetResourceVersionFromItems calculates the maximum resourceVersion among all list items
// and sets it on the list metadata. If the list is empty, resourceVersion is set to "0".
func (b *ListMetadataBuilder) SetResourceVersionFromItems() {
	maxRV := ""
	for _, item := range b.items {
		accessor, err := meta.Accessor(item)
		if err != nil {
			continue
		}
		rv := accessor.GetResourceVersion()
		if rv == "" {
			continue
		}
		if compareResourceVersion(rv, maxRV) > 0 {
			maxRV = rv
		}
	}
	if maxRV == "" {
		maxRV = "0"
	}
	b.list.SetResourceVersion(maxRV)
}

// compareResourceVersion compares two resourceVersion strings.
// It prefers numeric comparison when both versions are numbers, otherwise falls back to string comparison.
func compareResourceVersion(a, b string) int {
	if a == b {
		return 0
	}
	if b == "" {
		return 1
	}
	if a == "" {
		return -1
	}
	ai, errA := strconv.ParseInt(a, 10, 64)
	bi, errB := strconv.ParseInt(b, 10, 64)
	if errA == nil && errB == nil {
		switch {
		case ai < bi:
			return -1
		case ai > bi:
			return 1
		default:
			return 0
		}
	}
	if a < b {
		return -1
	}
	return 1
}
