package conditions

import (
	"context"
	"fmt"
	"strings"
	"time"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"

	"k8s.io/klog/v2"
)

func (cm *ConditionManager) batchConditionUpdate(resourceType string, resource interface{}) {
	cm.batchMutex.Lock()
	defer cm.batchMutex.Unlock()

	// Generate unique key for the resource
	var resourceKey string
	switch r := resource.(type) {
	case *models.Service:
		resourceKey = fmt.Sprintf("%s/%s", r.Namespace, r.Name)
	case *models.AddressGroup:
		resourceKey = fmt.Sprintf("%s/%s", r.Namespace, r.Name)
	case *models.RuleS2S:
		resourceKey = fmt.Sprintf("%s/%s", r.Namespace, r.Name)
	case *models.IEAgAgRule:
		resourceKey = fmt.Sprintf("%s/%s", r.Namespace, r.Name)
	case *models.SvcSvcRule:
		resourceKey = fmt.Sprintf("%s/%s", r.Namespace, r.Name)
	default:
		// Fallback for other types
		resourceKey = fmt.Sprintf("%p", resource)
	}

	batchKey := fmt.Sprintf("%s:%s", resourceType, resourceKey)
	cm.pendingBatch[batchKey] = resource

	if len(cm.pendingBatch) >= cm.batchSize {
		go cm.flushConditionBatch()
	} else if cm.batchTimer == nil {
		cm.batchTimer = time.AfterFunc(cm.batchTimeout, func() {
			cm.batchMutex.Lock()
			defer cm.batchMutex.Unlock()
			if len(cm.pendingBatch) > 0 {
				go cm.flushConditionBatch()
			}
		})
	}
}

func (cm *ConditionManager) flushConditionBatch() {
	if cm.sequentialMutex != nil {
		cm.sequentialMutex.Lock()
		defer cm.sequentialMutex.Unlock()
	}

	cm.batchMutex.Lock()

	if len(cm.pendingBatch) == 0 {
		cm.batchMutex.Unlock()
		return
	}

	// Copy the batch and clear it
	currentBatch := make(map[string]interface{})
	for k, v := range cm.pendingBatch {
		currentBatch[k] = v
	}
	cm.pendingBatch = make(map[string]interface{})

	// Reset the timer
	if cm.batchTimer != nil {
		cm.batchTimer.Stop()
		cm.batchTimer = nil
	}

	cm.batchMutex.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if registryWithConditions, ok := cm.registry.(interface {
		WriterForConditions(context.Context) (ports.Writer, error)
	}); ok {
		writer, err := registryWithConditions.WriterForConditions(ctx)
		if err != nil {
			klog.Errorf("Failed to get condition writer: %v", err)
			return
		}

		services := make([]*models.Service, 0)
		addressGroups := make([]*models.AddressGroup, 0)
		ruleS2S := make([]*models.RuleS2S, 0)
		ieAgAgRules := make([]*models.IEAgAgRule, 0)
		svcSvcRules := make([]*models.SvcSvcRule, 0)

		for batchKey, resource := range currentBatch {
			resourceType := strings.Split(batchKey, ":")[0]
			switch resourceType {
			case "Service":
				if svc, ok := resource.(*models.Service); ok {
					services = append(services, svc)
				}
			case "AddressGroup":
				if ag, ok := resource.(*models.AddressGroup); ok {
					addressGroups = append(addressGroups, ag)
				}
			case "RuleS2S":
				if rule, ok := resource.(*models.RuleS2S); ok {
					ruleS2S = append(ruleS2S, rule)
				}
			case "IEAgAgRule":
				if rule, ok := resource.(*models.IEAgAgRule); ok {
					ieAgAgRules = append(ieAgAgRules, rule)
				}
			case "SvcSvcRule":
				if rule, ok := resource.(*models.SvcSvcRule); ok {
					svcSvcRules = append(svcSvcRules, rule)
				}
			}
		}

		success := true
		if len(services) > 0 {
			serviceModels := make([]models.Service, len(services))
			for i, svc := range services {
				serviceModels[i] = *svc
			}
			if err := writer.SyncServices(ctx, serviceModels, ports.EmptyScope{}, ports.ConditionOnlyOperation{}); err != nil {
				klog.Errorf("Failed to batch sync %d services: %v", len(services), err)
				success = false
			}
		}

		if len(addressGroups) > 0 && success {
			agModels := make([]models.AddressGroup, len(addressGroups))
			for i, ag := range addressGroups {
				agModels[i] = *ag
			}

			if err := writer.SyncAddressGroups(ctx, agModels, ports.EmptyScope{}, ports.ConditionOnlyOperation{}); err != nil {
				klog.Errorf("Failed to batch sync %d address groups: %v", len(addressGroups), err)
				success = false
			}
		}

		if len(ruleS2S) > 0 && success {
			ruleModels := make([]models.RuleS2S, len(ruleS2S))
			for i, rule := range ruleS2S {
				ruleModels[i] = *rule
			}
			if err := writer.SyncRuleS2S(ctx, ruleModels, ports.EmptyScope{}, ports.ConditionOnlyOperation{}); err != nil {
				klog.Errorf("Failed to batch sync %d RuleS2S: %v", len(ruleS2S), err)
				success = false
			}
		}

		if len(ieAgAgRules) > 0 && success {
			ruleModels := make([]models.IEAgAgRule, len(ieAgAgRules))
			for i, rule := range ieAgAgRules {
				ruleModels[i] = *rule
			}

			if err := writer.SyncIEAgAgRules(ctx, ruleModels, ports.EmptyScope{}, ports.ConditionOnlyOperation{}); err != nil {
				klog.Errorf("Failed to batch sync %d IEAgAgRules: %v", len(ieAgAgRules), err)
				success = false
			}
		}

		if len(svcSvcRules) > 0 && success {
			ruleModels := make([]models.SvcSvcRule, len(svcSvcRules))
			for i, rule := range svcSvcRules {
				ruleModels[i] = *rule
			}

			if err := writer.SyncSvcSvcRules(ctx, ruleModels, ports.EmptyScope{}, ports.ConditionOnlyOperation{}); err != nil {
				klog.Errorf("Failed to batch sync %d SvcSvcRules: %v", len(svcSvcRules), err)
				success = false
			}
		}

		if success {
			if err := writer.Commit(); err != nil {
				klog.Errorf("Failed to commit batch transaction: %v", err)
				writer.Abort()
			}
		} else {
			writer.Abort()
		}
	} else {
		klog.Errorf("WriterForConditions not available, falling back to individual updates")
		for batchKey, resource := range currentBatch {
			resourceType := strings.Split(batchKey, ":")[0]
			switch resourceType {
			case "Service":
				if svc, ok := resource.(*models.Service); ok {
					cm.SaveServiceConditions(ctx, svc)
				}
			case "AddressGroup":
				if ag, ok := resource.(*models.AddressGroup); ok {
					cm.SaveAddressGroupConditions(ctx, ag)
				}
			case "RuleS2S":
				if rule, ok := resource.(*models.RuleS2S); ok {
					// Individual batch save - already optimized through batching system
					cm.SaveRuleS2SConditions(ctx, rule)
				}
			case "IEAgAgRule":
				if rule, ok := resource.(*models.IEAgAgRule); ok {
					// Individual batch save - already optimized through batching system
					cm.SaveIEAgAgRuleConditions(ctx, rule)
				}
			case "SvcSvcRule":
				if rule, ok := resource.(*models.SvcSvcRule); ok {
					cm.SaveSvcSvcRuleConditions(ctx, rule)
				}
			}
		}
	}
}
