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

package admission

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"netguard-pg-backend/internal/application/validation"
	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/client"
)

// ValidationWebhook реализует валидацию ресурсов через backend валидаторы
type ValidationWebhook struct {
	backendClient client.BackendClient
}

func NewValidationWebhook(backendClient client.BackendClient) *ValidationWebhook {
	return &ValidationWebhook{
		backendClient: backendClient,
	}
}

func (w *ValidationWebhook) ValidateAdmissionReview(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	log.Printf("Validating %s %s/%s operation %s", req.Kind.Kind, req.Namespace, req.Name, req.Operation)

	switch req.Kind.Kind {
	case "Service":
		return w.validateService(ctx, req)
	case "AddressGroup":
		return w.validateAddressGroup(ctx, req)
	case "AddressGroupBinding":
		return w.validateAddressGroupBinding(ctx, req)
	case "AddressGroupPortMapping":
		return w.validateAddressGroupPortMapping(ctx, req)
	case "RuleS2S":
		return w.validateRuleS2S(ctx, req)
	case "ServiceAlias":
		return w.validateServiceAlias(ctx, req)
	case "AddressGroupBindingPolicy":
		return w.validateAddressGroupBindingPolicy(ctx, req)
	case "IEAgAgRule":
		return w.validateIEAgAgRule(ctx, req)
	default:
		return w.errorResponse(req.UID, fmt.Sprintf("Unknown resource kind: %s", req.Kind.Kind))
	}
}

func (w *ValidationWebhook) validateService(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	var service netguardv1beta1.Service
	if err := json.Unmarshal(req.Object.Raw, &service); err != nil {
		return w.errorResponse(req.UID, fmt.Sprintf("Failed to unmarshal Service: %v", err))
	}

	// Получаем Reader для валидации
	reader, err := w.backendClient.GetReader(ctx)
	if err != nil {
		return w.errorResponse(req.UID, fmt.Sprintf("Failed to get reader: %v", err))
	}
	defer reader.Close()

	// Получаем валидатор
	validator := w.backendClient.GetDependencyValidator()
	serviceValidator := validator.GetServiceValidator()

	// Конвертируем в domain модель
	domainService := convertServiceToDomain(service)

	switch req.Operation {
	case admissionv1.Create:
		// Валидация для создания
		if err := serviceValidator.ValidateForCreation(ctx, domainService); err != nil {
			return w.errorResponse(req.UID, fmt.Sprintf("Service validation failed: %v", err))
		}

	case admissionv1.Update:
		// Получаем старую версию для валидации обновления
		var oldService netguardv1beta1.Service
		if err := json.Unmarshal(req.OldObject.Raw, &oldService); err != nil {
			return w.errorResponse(req.UID, fmt.Sprintf("Failed to unmarshal old Service: %v", err))
		}

		oldDomainService := convertServiceToDomain(oldService)

		// Валидация для обновления
		if err := serviceValidator.ValidateForUpdate(ctx, oldDomainService, domainService); err != nil {
			return w.errorResponse(req.UID, fmt.Sprintf("Service update validation failed: %v", err))
		}

	case admissionv1.Delete:
		// Для Delete операций не используем валидацию - она будет в API Server при вызове backend
		log.Printf("Delete operation for Service %s/%s - validation will be done in API Server", req.Namespace, req.Name)
	}

	return w.allowResponse(req.UID, "Service validation passed")
}

func (w *ValidationWebhook) validateAddressGroup(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	var addressGroup netguardv1beta1.AddressGroup
	if err := json.Unmarshal(req.Object.Raw, &addressGroup); err != nil {
		return w.errorResponse(req.UID, fmt.Sprintf("Failed to unmarshal AddressGroup: %v", err))
	}

	// Получаем Reader для валидации
	reader, err := w.backendClient.GetReader(ctx)
	if err != nil {
		return w.errorResponse(req.UID, fmt.Sprintf("Failed to get reader: %v", err))
	}
	defer reader.Close()

	// Получаем валидатор
	validator := w.backendClient.GetDependencyValidator()
	addressGroupValidator := validator.GetAddressGroupValidator()

	// Конвертируем в domain модель
	domainAddressGroup := convertAddressGroupToDomain(addressGroup)

	switch req.Operation {
	case admissionv1.Create:
		// Валидация для создания
		if err := addressGroupValidator.ValidateForCreation(ctx, domainAddressGroup); err != nil {
			return w.errorResponse(req.UID, fmt.Sprintf("AddressGroup validation failed: %v", err))
		}

	case admissionv1.Update:
		// Получаем старую версию для валидации обновления
		var oldAddressGroup netguardv1beta1.AddressGroup
		if err := json.Unmarshal(req.OldObject.Raw, &oldAddressGroup); err != nil {
			return w.errorResponse(req.UID, fmt.Sprintf("Failed to unmarshal old AddressGroup: %v", err))
		}

		oldDomainAddressGroup := convertAddressGroupToDomain(oldAddressGroup)

		// Валидация для обновления
		if err := addressGroupValidator.ValidateForUpdate(ctx, oldDomainAddressGroup, domainAddressGroup); err != nil {
			return w.errorResponse(req.UID, fmt.Sprintf("AddressGroup update validation failed: %v", err))
		}

	case admissionv1.Delete:
		// Для Delete операций не используем валидацию - она будет в API Server при вызове backend
		log.Printf("Delete operation for AddressGroup %s/%s - validation will be done in API Server", req.Namespace, req.Name)
	}

	return w.allowResponse(req.UID, "AddressGroup validation passed")
}

func (w *ValidationWebhook) validateAddressGroupBinding(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	var binding netguardv1beta1.AddressGroupBinding
	if err := json.Unmarshal(req.Object.Raw, &binding); err != nil {
		return w.errorResponse(req.UID, fmt.Sprintf("Failed to unmarshal AddressGroupBinding: %v", err))
	}

	// Получаем Reader для валидации
	reader, err := w.backendClient.GetReader(ctx)
	if err != nil {
		return w.errorResponse(req.UID, fmt.Sprintf("Failed to get reader: %v", err))
	}
	defer reader.Close()

	// Получаем валидатор
	validator := w.backendClient.GetDependencyValidator()
	bindingValidator := validator.GetAddressGroupBindingValidator()

	// Конвертируем в domain модель
	domainBinding := convertAddressGroupBindingToDomain(binding)

	switch req.Operation {
	case admissionv1.Create:
		// Валидация для создания
		if err := bindingValidator.ValidateForCreation(ctx, &domainBinding); err != nil {
			return w.errorResponse(req.UID, fmt.Sprintf("AddressGroupBinding validation failed: %v", err))
		}

	case admissionv1.Update:
		// Получаем старую версию для валидации обновления
		var oldBinding netguardv1beta1.AddressGroupBinding
		if err := json.Unmarshal(req.OldObject.Raw, &oldBinding); err != nil {
			return w.errorResponse(req.UID, fmt.Sprintf("Failed to unmarshal old AddressGroupBinding: %v", err))
		}

		oldDomainBinding := convertAddressGroupBindingToDomain(oldBinding)

		// Валидация для обновления
		if err := bindingValidator.ValidateForUpdate(ctx, oldDomainBinding, &domainBinding); err != nil {
			return w.errorResponse(req.UID, fmt.Sprintf("AddressGroupBinding update validation failed: %v", err))
		}

	case admissionv1.Delete:
		// Для Delete операций не используем валидацию - она будет в API Server при вызове backend
		log.Printf("Delete operation for AddressGroupBinding %s/%s - validation will be done in API Server", req.Namespace, req.Name)
	}

	return w.allowResponse(req.UID, "AddressGroupBinding validation passed")
}

func (w *ValidationWebhook) validateAddressGroupPortMapping(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	var mapping netguardv1beta1.AddressGroupPortMapping
	if err := json.Unmarshal(req.Object.Raw, &mapping); err != nil {
		return w.errorResponse(req.UID, fmt.Sprintf("Failed to unmarshal AddressGroupPortMapping: %v", err))
	}

	// Получаем Reader для валидации
	reader, err := w.backendClient.GetReader(ctx)
	if err != nil {
		return w.errorResponse(req.UID, fmt.Sprintf("Failed to get reader: %v", err))
	}
	defer reader.Close()

	// Получаем валидатор
	validator := w.backendClient.GetDependencyValidator()
	mappingValidator := validator.GetAddressGroupPortMappingValidator()

	// Конвертируем в domain модель
	domainMapping := convertAddressGroupPortMappingToDomain(mapping)

	switch req.Operation {
	case admissionv1.Create:
		// Валидация для создания
		if err := mappingValidator.ValidateForCreation(ctx, domainMapping); err != nil {
			return w.errorResponse(req.UID, fmt.Sprintf("AddressGroupPortMapping validation failed: %v", err))
		}

	case admissionv1.Update:
		// Получаем старую версию для валидации обновления
		var oldMapping netguardv1beta1.AddressGroupPortMapping
		if err := json.Unmarshal(req.OldObject.Raw, &oldMapping); err != nil {
			return w.errorResponse(req.UID, fmt.Sprintf("Failed to unmarshal old AddressGroupPortMapping: %v", err))
		}

		oldDomainMapping := convertAddressGroupPortMappingToDomain(oldMapping)

		// Валидация для обновления
		if err := mappingValidator.ValidateForUpdate(ctx, oldDomainMapping, domainMapping); err != nil {
			return w.errorResponse(req.UID, fmt.Sprintf("AddressGroupPortMapping update validation failed: %v", err))
		}

	case admissionv1.Delete:
		// Для Delete операций не используем валидацию - она будет в API Server при вызове backend
		log.Printf("Delete operation for AddressGroupPortMapping %s/%s - validation will be done in API Server", req.Namespace, req.Name)
	}

	return w.allowResponse(req.UID, "AddressGroupPortMapping validation passed")
}

func (w *ValidationWebhook) validateRuleS2S(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	var rule netguardv1beta1.RuleS2S
	if err := json.Unmarshal(req.Object.Raw, &rule); err != nil {
		return w.errorResponse(req.UID, fmt.Sprintf("Failed to unmarshal RuleS2S: %v", err))
	}

	// Получаем Reader для валидации
	reader, err := w.backendClient.GetReader(ctx)
	if err != nil {
		return w.errorResponse(req.UID, fmt.Sprintf("Failed to get reader: %v", err))
	}
	defer reader.Close()

	// Получаем валидатор
	validator := w.backendClient.GetDependencyValidator()
	ruleValidator := validator.GetRuleS2SValidator()

	// Конвертируем в domain модель
	domainRule := convertRuleS2SToDomain(rule)

	switch req.Operation {
	case admissionv1.Create:
		// Валидация для создания
		if err := ruleValidator.ValidateForCreation(ctx, domainRule); err != nil {
			return w.errorResponse(req.UID, fmt.Sprintf("RuleS2S validation failed: %v", err))
		}

	case admissionv1.Update:
		// Получаем старую версию для валидации обновления
		var oldRule netguardv1beta1.RuleS2S
		if err := json.Unmarshal(req.OldObject.Raw, &oldRule); err != nil {
			return w.errorResponse(req.UID, fmt.Sprintf("Failed to unmarshal old RuleS2S: %v", err))
		}

		oldDomainRule := convertRuleS2SToDomain(oldRule)

		// Валидация для обновления
		if err := ruleValidator.ValidateForUpdate(ctx, oldDomainRule, domainRule); err != nil {
			return w.errorResponse(req.UID, fmt.Sprintf("RuleS2S update validation failed: %v", err))
		}

	case admissionv1.Delete:
		// Для Delete операций не используем валидацию - она будет в API Server при вызове backend
		log.Printf("Delete operation for RuleS2S %s/%s - validation will be done in API Server", req.Namespace, req.Name)
	}

	return w.allowResponse(req.UID, "RuleS2S validation passed")
}

func (w *ValidationWebhook) validateServiceAlias(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	var alias netguardv1beta1.ServiceAlias
	if err := json.Unmarshal(req.Object.Raw, &alias); err != nil {
		return w.errorResponse(req.UID, fmt.Sprintf("Failed to unmarshal ServiceAlias: %v", err))
	}

	// Получаем Reader для валидации
	reader, err := w.backendClient.GetReader(ctx)
	if err != nil {
		return w.errorResponse(req.UID, fmt.Sprintf("Failed to get reader: %v", err))
	}
	defer reader.Close()

	// Получаем валидатор
	validator := w.backendClient.GetDependencyValidator()
	aliasValidator := validator.GetServiceAliasValidator()

	// Конвертируем в domain модель
	domainAlias := convertServiceAliasToDomain(alias)

	switch req.Operation {
	case admissionv1.Create:
		// Валидация для создания
		if err := aliasValidator.ValidateForCreation(ctx, &domainAlias); err != nil {
			return w.errorResponse(req.UID, fmt.Sprintf("ServiceAlias validation failed: %v", err))
		}

	case admissionv1.Update:
		// Получаем старую версию для валидации обновления
		var oldAlias netguardv1beta1.ServiceAlias
		if err := json.Unmarshal(req.OldObject.Raw, &oldAlias); err != nil {
			return w.errorResponse(req.UID, fmt.Sprintf("Failed to unmarshal old ServiceAlias: %v", err))
		}

		oldDomainAlias := convertServiceAliasToDomain(oldAlias)

		// Валидация для обновления
		if err := aliasValidator.ValidateForUpdate(ctx, oldDomainAlias, domainAlias); err != nil {
			return w.errorResponse(req.UID, fmt.Sprintf("ServiceAlias update validation failed: %v", err))
		}

	case admissionv1.Delete:
		// Для Delete операций не используем валидацию - она будет в API Server при вызове backend
		log.Printf("Delete operation for ServiceAlias %s/%s - validation will be done in API Server", req.Namespace, req.Name)
	}

	return w.allowResponse(req.UID, "ServiceAlias validation passed")
}

func (w *ValidationWebhook) validateAddressGroupBindingPolicy(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	var policy netguardv1beta1.AddressGroupBindingPolicy
	if err := json.Unmarshal(req.Object.Raw, &policy); err != nil {
		return w.errorResponse(req.UID, fmt.Sprintf("Failed to unmarshal AddressGroupBindingPolicy: %v", err))
	}

	// Получаем Reader для валидации
	reader, err := w.backendClient.GetReader(ctx)
	if err != nil {
		return w.errorResponse(req.UID, fmt.Sprintf("Failed to get reader: %v", err))
	}
	defer reader.Close()

	// Получаем валидатор
	validator := w.backendClient.GetDependencyValidator()
	policyValidator := validator.GetAddressGroupBindingPolicyValidator()

	// Конвертируем в domain модель
	domainPolicy := convertAddressGroupBindingPolicyToDomain(policy)

	switch req.Operation {
	case admissionv1.Create:
		// Валидация для создания
		if err := policyValidator.ValidateForCreation(ctx, &domainPolicy); err != nil {
			return w.errorResponse(req.UID, fmt.Sprintf("AddressGroupBindingPolicy validation failed: %v", err))
		}

	case admissionv1.Update:
		// Получаем старую версию для валидации обновления
		var oldPolicy netguardv1beta1.AddressGroupBindingPolicy
		if err := json.Unmarshal(req.OldObject.Raw, &oldPolicy); err != nil {
			return w.errorResponse(req.UID, fmt.Sprintf("Failed to unmarshal old AddressGroupBindingPolicy: %v", err))
		}

		oldDomainPolicy := convertAddressGroupBindingPolicyToDomain(oldPolicy)

		// Валидация для обновления
		if err := policyValidator.ValidateForUpdate(ctx, oldDomainPolicy, &domainPolicy); err != nil {
			return w.errorResponse(req.UID, fmt.Sprintf("AddressGroupBindingPolicy update validation failed: %v", err))
		}

	case admissionv1.Delete:
		// Для Delete операций не используем валидацию - она будет в API Server при вызове backend
		log.Printf("Delete operation for AddressGroupBindingPolicy %s/%s - validation will be done in API Server", req.Namespace, req.Name)
	}

	return w.allowResponse(req.UID, "AddressGroupBindingPolicy validation passed")
}

func (w *ValidationWebhook) validateIEAgAgRule(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	var rule netguardv1beta1.IEAgAgRule
	if err := json.Unmarshal(req.Object.Raw, &rule); err != nil {
		return w.errorResponse(req.UID, fmt.Sprintf("Failed to unmarshal IEAgAgRule: %v", err))
	}

	// Получаем Reader для валидации
	reader, err := w.backendClient.GetReader(ctx)
	if err != nil {
		return w.errorResponse(req.UID, fmt.Sprintf("Failed to get reader: %v", err))
	}
	defer reader.Close()

	// Получаем валидатор
	validator := w.backendClient.GetDependencyValidator()
	ruleValidator := validator.GetIEAgAgRuleValidator()

	// Конвертируем в domain модель
	domainRule := convertIEAgAgRuleToDomain(rule)

	switch req.Operation {
	case admissionv1.Create:
		// Валидация для создания
		if err := ruleValidator.ValidateForCreation(ctx, domainRule); err != nil {
			return w.errorResponse(req.UID, fmt.Sprintf("IEAgAgRule validation failed: %v", err))
		}

	case admissionv1.Update:
		// Получаем старую версию для валидации обновления
		var oldRule netguardv1beta1.IEAgAgRule
		if err := json.Unmarshal(req.OldObject.Raw, &oldRule); err != nil {
			return w.errorResponse(req.UID, fmt.Sprintf("Failed to unmarshal old IEAgAgRule: %v", err))
		}

		oldDomainRule := convertIEAgAgRuleToDomain(oldRule)

		// Валидация для обновления
		if err := ruleValidator.ValidateForUpdate(ctx, oldDomainRule, domainRule); err != nil {
			return w.errorResponse(req.UID, fmt.Sprintf("IEAgAgRule update validation failed: %v", err))
		}

	case admissionv1.Delete:
		// Для Delete операций не используем валидацию - она будет в API Server при вызове backend
		log.Printf("Delete operation for IEAgAgRule %s/%s - validation will be done in API Server", req.Namespace, req.Name)
	}

	return w.allowResponse(req.UID, "IEAgAgRule validation passed")
}

// Helper functions для конвертации K8s API типов в domain модели
// Создаем новые конверторы K8s → domain (не через protobuf)

func convertServiceToDomain(k8sService netguardv1beta1.Service) models.Service {
	// Прямая конвертация K8s → domain модель
	service := models.Service{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sService.Name,
				Namespace: k8sService.Namespace,
			},
		},
		Description: k8sService.Spec.Description,
	}

	// Конвертация портов - используем ПРАВИЛЬНЫЙ парсинг
	for _, port := range k8sService.Spec.IngressPorts {
		// Используем validation.ParsePortRanges для валидации портов
		_, err := validation.ParsePortRanges(port.Port)
		if err != nil {
			// Если ошибка парсинга, пропускаем этот порт
			log.Printf("Failed to parse Service port %s: %v", port.Port, err)
			continue
		}

		service.IngressPorts = append(service.IngressPorts, models.IngressPort{
			Protocol:    models.TransportProtocol(port.Protocol),
			Port:        port.Port,
			Description: port.Description,
		})
	}

	return service
}

func convertAddressGroupToDomain(k8sGroup netguardv1beta1.AddressGroup) models.AddressGroup {
	// Конвертация адресов из []Address в []string
	var addresses []string
	for _, addr := range k8sGroup.Spec.Addresses {
		addresses = append(addresses, addr.Address)
	}

	return models.AddressGroup{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sGroup.Name,
				Namespace: k8sGroup.Namespace,
			},
		},
		Description: k8sGroup.Spec.Description,
		Addresses:   addresses,
	}
}

func convertAddressGroupBindingToDomain(k8sBinding netguardv1beta1.AddressGroupBinding) models.AddressGroupBinding {
	return models.AddressGroupBinding{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sBinding.Name,
				Namespace: k8sBinding.Namespace,
			},
		},
		ServiceRef: models.ServiceRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sBinding.Spec.ServiceRef.Name,
				Namespace: k8sBinding.Namespace, // ObjectReference не имеет Namespace, используем namespace самого объекта
			},
		},
		AddressGroupRef: models.AddressGroupRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sBinding.Spec.AddressGroupRef.Name,
				Namespace: k8sBinding.Spec.AddressGroupRef.Namespace,
			},
		},
	}
}

func convertAddressGroupPortMappingToDomain(k8sMapping netguardv1beta1.AddressGroupPortMapping) models.AddressGroupPortMapping {
	mapping := models.AddressGroupPortMapping{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sMapping.Name,
				Namespace: k8sMapping.Namespace,
			},
		},
		AccessPorts: make(map[models.ServiceRef]models.ServicePorts),
	}

	// Конвертация access ports из AccessPorts поля
	for _, accessPort := range k8sMapping.AccessPorts.Items {
		serviceRef := models.ServiceRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      accessPort.Name,
				Namespace: accessPort.Namespace,
			},
		}

		servicePorts := models.ServicePorts{
			Ports: make(models.ProtocolPorts),
		}

		// Конвертация TCP портов - используем ПРАВИЛЬНЫЙ парсинг
		if len(accessPort.Ports.TCP) > 0 {
			var tcpRanges []models.PortRange
			for _, tcpPort := range accessPort.Ports.TCP {
				// Используем validation.ParsePortRanges для ПРАВИЛЬНОГО парсинга
				parsedRanges, err := validation.ParsePortRanges(tcpPort.Port)
				if err != nil {
					// Если ошибка парсинга, пропускаем этот порт
					log.Printf("Failed to parse TCP port %s: %v", tcpPort.Port, err)
					continue
				}
				tcpRanges = append(tcpRanges, parsedRanges...)
			}
			if len(tcpRanges) > 0 {
				servicePorts.Ports[models.TCP] = tcpRanges
			}
		}

		// Конвертация UDP портов - используем ПРАВИЛЬНЫЙ парсинг
		if len(accessPort.Ports.UDP) > 0 {
			var udpRanges []models.PortRange
			for _, udpPort := range accessPort.Ports.UDP {
				// Используем validation.ParsePortRanges для ПРАВИЛЬНОГО парсинга
				parsedRanges, err := validation.ParsePortRanges(udpPort.Port)
				if err != nil {
					// Если ошибка парсинга, пропускаем этот порт
					log.Printf("Failed to parse UDP port %s: %v", udpPort.Port, err)
					continue
				}
				udpRanges = append(udpRanges, parsedRanges...)
			}
			if len(udpRanges) > 0 {
				servicePorts.Ports[models.UDP] = udpRanges
			}
		}

		mapping.AccessPorts[serviceRef] = servicePorts
	}

	return mapping
}

func convertRuleS2SToDomain(k8sRule netguardv1beta1.RuleS2S) models.RuleS2S {
	return models.RuleS2S{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sRule.Name,
				Namespace: k8sRule.Namespace,
			},
		},
		Traffic: models.Traffic(k8sRule.Spec.Traffic),
		ServiceLocalRef: models.ServiceAliasRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sRule.Spec.ServiceLocalRef.Name,
				Namespace: k8sRule.Spec.ServiceLocalRef.Namespace,
			},
		},
		ServiceRef: models.ServiceAliasRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sRule.Spec.ServiceRef.Name,
				Namespace: k8sRule.Spec.ServiceRef.Namespace,
			},
		},
	}
}

func convertServiceAliasToDomain(k8sAlias netguardv1beta1.ServiceAlias) models.ServiceAlias {
	return models.ServiceAlias{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sAlias.Name,
				Namespace: k8sAlias.Namespace,
			},
		},
		ServiceRef: models.ServiceRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sAlias.Spec.ServiceRef.Name,
				Namespace: k8sAlias.Namespace, // ObjectReference не имеет Namespace, используем namespace самого объекта
			},
		},
	}
}

func convertAddressGroupBindingPolicyToDomain(k8sPolicy netguardv1beta1.AddressGroupBindingPolicy) models.AddressGroupBindingPolicy {
	return models.AddressGroupBindingPolicy{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sPolicy.Name,
				Namespace: k8sPolicy.Namespace,
			},
		},
		ServiceRef: models.ServiceRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sPolicy.Spec.ServiceRef.Name,
				Namespace: k8sPolicy.Spec.ServiceRef.Namespace,
			},
		},
		AddressGroupRef: models.AddressGroupRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sPolicy.Spec.AddressGroupRef.Name,
				Namespace: k8sPolicy.Spec.AddressGroupRef.Namespace,
			},
		},
		// Ports поля нет в domain модели AddressGroupBindingPolicy
	}
}

func convertIEAgAgRuleToDomain(k8sRule netguardv1beta1.IEAgAgRule) models.IEAgAgRule {
	rule := models.IEAgAgRule{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sRule.Name,
				Namespace: k8sRule.Namespace,
			},
		},
		Transport: models.TransportProtocol(k8sRule.Spec.Transport),
		Traffic:   models.Traffic(k8sRule.Spec.Traffic),
		Action:    models.RuleAction(k8sRule.Spec.Action),
		AddressGroupLocal: models.AddressGroupRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sRule.Spec.AddressGroupLocal.Name,
				Namespace: k8sRule.Namespace, // ObjectReference не имеет Namespace, используем namespace самого объекта
			},
		},
		AddressGroup: models.AddressGroupRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sRule.Spec.AddressGroup.Name,
				Namespace: k8sRule.Namespace, // ObjectReference не имеет Namespace, используем namespace самого объекта
			},
		},
	}

	// Конвертация портов - используем правильную логику
	for _, port := range k8sRule.Spec.Ports {
		var destination string
		if port.PortRange != nil {
			// Формируем строку диапазона портов
			destination = fmt.Sprintf("%d-%d", port.PortRange.From, port.PortRange.To)
		} else if port.Port > 0 {
			// Одиночный порт
			destination = fmt.Sprintf("%d", port.Port)
		}

		if destination != "" {
			// Проверяем что порт валидный используя validation.ParsePortRanges
			_, err := validation.ParsePortRanges(destination)
			if err != nil {
				log.Printf("Failed to validate IEAgAgRule port %s: %v", destination, err)
				continue
			}

			rule.Ports = append(rule.Ports, models.PortSpec{
				Destination: destination,
			})
		}
	}

	return rule
}

func (w *ValidationWebhook) allowResponse(uid types.UID, message string) *admissionv1.AdmissionResponse {
	return &admissionv1.AdmissionResponse{
		UID:     uid,
		Allowed: true,
		Result: &metav1.Status{
			Code:    200,
			Message: message,
		},
	}
}

func (w *ValidationWebhook) errorResponse(uid types.UID, message string) *admissionv1.AdmissionResponse {
	return &admissionv1.AdmissionResponse{
		UID:     uid,
		Allowed: false,
		Result: &metav1.Status{
			Code:    400,
			Message: message,
		},
	}
}
