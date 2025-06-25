/*
Copyright 2024 The Netguard Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package watch

import (
	"sync"

	"netguard-pg-backend/internal/k8s/client"
)

// PollerManager управляет shared poller'ами для всех типов ресурсов
type PollerManager struct {
	mu      sync.RWMutex
	pollers map[string]*SharedPoller // resourceType -> SharedPoller
	backend client.BackendClient
}

var globalPollerManager *PollerManager
var pollerManagerMutex sync.Mutex

// GetPollerManager возвращает глобальный экземпляр PollerManager
func GetPollerManager(backend client.BackendClient) *PollerManager {
	pollerManagerMutex.Lock()
	defer pollerManagerMutex.Unlock()

	if globalPollerManager == nil {
		globalPollerManager = &PollerManager{
			pollers: make(map[string]*SharedPoller),
			backend: backend,
		}
	}
	return globalPollerManager
}

// GetPoller возвращает SharedPoller для указанного типа ресурса
func (pm *PollerManager) GetPoller(resourceType string) *SharedPoller {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if poller, exists := pm.pollers[resourceType]; exists {
		return poller
	}

	// Создать новый поллер
	converter := GetConverterForResourceType(resourceType)
	poller := NewSharedPoller(pm.backend, resourceType, converter)
	pm.pollers[resourceType] = poller

	return poller
}

// Shutdown останавливает все поллеры
func (pm *PollerManager) Shutdown() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for _, poller := range pm.pollers {
		poller.Shutdown()
	}
	pm.pollers = make(map[string]*SharedPoller)
}

// GetConverterForResourceType возвращает конвертер для указанного типа ресурса
func GetConverterForResourceType(resourceType string) Converter {
	switch resourceType {
	case "services":
		return &ServiceConverter{}
	case "addressgroups":
		return &AddressGroupConverter{}
	case "addressgroupbindings":
		return &AddressGroupBindingConverter{}
	case "addressgroupportmappings":
		return &AddressGroupPortMappingConverter{}
	case "rules2s":
		return &RuleS2SConverter{}
	case "servicealiases":
		return &ServiceAliasConverter{}
	case "addressgroupbindingpolicies":
		return &AddressGroupBindingPolicyConverter{}
	case "ieagagrules":
		return &IEAgAgRuleConverter{}
	default:
		return nil
	}
}
