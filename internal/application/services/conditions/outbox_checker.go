package conditions

import (
	"context"
	"k8s.io/klog/v2"
)

func (cm *ConditionManager) hasPendingOutboxEntry(ctx context.Context, resourceType, namespace, name string) bool {
	if cm.outboxRepo == nil {
		return false
	}
	// FIX: Use per-resource check instead of global count to avoid blocking ALL resources
	count, err := cm.outboxRepo.CountPendingForResource(ctx, resourceType, namespace, name)
	if err != nil {
		klog.ErrorS(err, "hasPendingOutboxEntry: failed to count pending entries for resource",
			"resource_type", resourceType, "namespace", namespace, "name", name)
		return false
	}
	if count > 0 {
		klog.V(2).InfoS("hasPendingOutboxEntry: found pending outbox entry for THIS resource, skipping batch update",
			"resource_type", resourceType,
			"namespace", namespace,
			"name", name,
			"pending_count", count)
		return true
	}
	return false
}
