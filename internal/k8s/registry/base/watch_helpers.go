package base

import (
	"strconv"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
)

// extractResourceVersion извлекает resourceVersion из K8s объекта
func (s *BaseStorage[K, D]) extractResourceVersion(obj runtime.Object) int64 {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		klog.V(6).InfoS("Failed to get accessor from object",
			"resource", s.resourceName,
			"error", err.Error())
		return 0
	}

	rvStr := accessor.GetResourceVersion()
	if rvStr == "" {
		klog.V(6).InfoS("Object has empty resourceVersion",
			"resource", s.resourceName,
			"name", accessor.GetName(),
			"namespace", accessor.GetNamespace())
		return 0
	}

	rv, err := strconv.ParseInt(rvStr, 10, 64)
	if err != nil {
		klog.V(4).InfoS("Failed to parse resourceVersion",
			"resource", s.resourceName,
			"name", accessor.GetName(),
			"namespace", accessor.GetNamespace(),
			"resourceVersion", rvStr,
			"error", err.Error())
		return 0
	}

	return rv
}

// GetWatchCache возвращает watch cache для этого storage (для тестирования)
func (s *BaseStorage[K, D]) GetWatchCache() interface{} {
	return s.watchCache
}
