package base

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestListMetadataBuilder_SetResourceVersionFromItems(t *testing.T) {
	list := &unstructured.UnstructuredList{}
	list.Items = []unstructured.Unstructured{
		newUnstructuredWithRV("5"),
		newUnstructuredWithRV("12"),
		newUnstructuredWithRV("9"),
	}

	builder, err := NewListMetadataBuilder(list)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	builder.SetResourceVersionFromItems()

	if got := list.GetResourceVersion(); got != "12" {
		t.Fatalf("expected resourceVersion 12, got %s", got)
	}
}

func TestListMetadataBuilder_EmptyList(t *testing.T) {
	list := &unstructured.UnstructuredList{}

	builder, err := NewListMetadataBuilder(list)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	builder.SetResourceVersionFromItems()

	if got := list.GetResourceVersion(); got != "0" {
		t.Fatalf("expected resourceVersion 0, got %s", got)
	}
}

func TestListMetadataBuilder_NonNumericResourceVersion(t *testing.T) {
	list := &unstructured.UnstructuredList{}
	list.Items = []unstructured.Unstructured{
		newUnstructuredWithRV("abc"),
		newUnstructuredWithRV("abd"),
		newUnstructuredWithRV("zzz"),
	}

	builder, err := NewListMetadataBuilder(list)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	builder.SetResourceVersionFromItems()

	if got := list.GetResourceVersion(); got != "zzz" {
		t.Fatalf("expected resourceVersion zzz, got %s", got)
	}
}

func newUnstructuredWithRV(rv string) unstructured.Unstructured {
	obj := unstructured.Unstructured{}
	obj.SetResourceVersion(rv)
	return obj
}
