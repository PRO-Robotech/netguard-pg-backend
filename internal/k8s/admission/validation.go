package admission

import (
	"context"
	"encoding/json"
	"fmt"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"netguard-pg-backend/internal/application/validation"
	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/k8s/client"
	k8svalidation "netguard-pg-backend/internal/k8s/registry/validation"
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
	var response *admissionv1.AdmissionResponse
	switch req.Kind.Kind {
	case "Service":
		response = w.validateService(ctx, req)
	case "AddressGroup":
		response = w.validateAddressGroup(ctx, req)
	case "AddressGroupBinding":
		response = w.validateAddressGroupBinding(ctx, req)
	case "AddressGroupPortMapping":
		response = w.validateAddressGroupPortMapping(ctx, req)
	case "SvcSvcRule":
		response = w.validateSvcSvcRule(ctx, req)
	case "SvcFqdnRule":
		response = w.validateSvcFqdnRule(ctx, req)
	case "AddressGroupBindingPolicy":
		response = w.validateAddressGroupBindingPolicy(ctx, req)
	case "Network":
		response = w.validateNetwork(ctx, req)
	case "NetworkBinding":
		response = w.validateNetworkBinding(ctx, req)
	case "HostBinding":
		response = w.validateHostBinding(ctx, req)
	case "IECidrSvcRule":
		response = w.validateIECidrSvcRule(ctx, req)
	default:
		response = w.errorResponse(req.UID, fmt.Sprintf("Unknown resource kind: %s", req.Kind.Kind))
	}

	// Log the response before returning
	return response
}

func (w *ValidationWebhook) validateService(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	if req.Kind.Kind != "Service" {
		return w.errorResponse(req.UID, fmt.Sprintf("Service webhook incorrectly called for %s resource", req.Kind.Kind))
	}

	if req.Operation == admissionv1.Delete {

		// Get validator for dependency checking
		validator := w.backendClient.GetDependencyValidator()
		serviceValidator := validator.GetServiceValidator()

		// Check dependencies before deletion
		serviceID := models.NewResourceIdentifier(req.Name, models.WithNamespace(req.Namespace))
		if err := serviceValidator.CheckDependencies(ctx, serviceID); err != nil {
			return w.errorResponse(req.UID, fmt.Sprintf("Cannot delete Service: %v", err))
		}

		return w.allowResponse(req.UID, "Service deletion validation passed")
	}

	// For CREATE and UPDATE operations, unmarshal the object
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

		k8sValidator := k8svalidation.NewServiceValidator()
		if errs := k8sValidator.ValidateCreate(ctx, &service); len(errs) > 0 {
			return w.errorResponse(req.UID, fmt.Sprintf("Service K8s validation failed: %v", errs.ToAggregate()))
		}

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
	}

	return w.allowResponse(req.UID, "Service validation passed")
}

func (w *ValidationWebhook) validateAddressGroup(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	// DELETE requests do not include an object payload, so handle them separately before unmarshalling.
	if req.Operation == admissionv1.Delete {

		// Get validator for dependency checking
		validator := w.backendClient.GetDependencyValidator()
		addressGroupValidator := validator.GetAddressGroupValidator()

		// Check dependencies before deletion
		addressGroupID := models.NewResourceIdentifier(req.Name, models.WithNamespace(req.Namespace))
		if err := addressGroupValidator.CheckDependencies(ctx, addressGroupID); err != nil {
			return w.errorResponse(req.UID, fmt.Sprintf("Cannot delete AddressGroup: %v", err))
		}

		return w.allowResponse(req.UID, "AddressGroup deletion validation passed")
	}

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
		k8sValidator := k8svalidation.NewAddressGroupValidator()
		if errs := k8sValidator.ValidateCreate(ctx, &addressGroup); len(errs) > 0 {
			return w.errorResponse(req.UID, fmt.Sprintf("AddressGroup K8s validation failed: %v", errs.ToAggregate()))
		}

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
	}

	return w.allowResponse(req.UID, "AddressGroup validation passed")
}

func (w *ValidationWebhook) validateAddressGroupBinding(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	if req.Kind.Kind != "AddressGroupBinding" {
		return w.errorResponse(req.UID, fmt.Sprintf("AddressGroupBinding webhook incorrectly called for %s resource", req.Kind.Kind))
	}

	// DELETE requests do not include an object payload, so handle them separately before unmarshalling.
	if req.Operation == admissionv1.Delete {

		// Get validator for dependency checking
		validator := w.backendClient.GetDependencyValidator()
		bindingValidator := validator.GetAddressGroupBindingValidator()

		// Check dependencies before deletion
		bindingID := models.NewResourceIdentifier(req.Name, models.WithNamespace(req.Namespace))
		if err := bindingValidator.CheckDependencies(ctx, bindingID); err != nil {
			return w.errorResponse(req.UID, fmt.Sprintf("Cannot delete AddressGroupBinding: %v", err))
		}

		return w.allowResponse(req.UID, "AddressGroupBinding deletion validation passed")
	}

	var binding netguardv1beta1.AddressGroupBinding
	if err := json.Unmarshal(req.Object.Raw, &binding); err != nil {
		return w.errorResponse(req.UID, fmt.Sprintf("Failed to unmarshal AddressGroupBinding: %v", err))
	}

	switch req.Operation {
	case admissionv1.Create:

		// First run K8s-level validation for basic field validation
		k8sValidator := k8svalidation.NewAddressGroupBindingValidator()
		if errs := k8sValidator.ValidateCreate(ctx, &binding); len(errs) > 0 {
			return w.errorResponse(req.UID, fmt.Sprintf("AddressGroupBinding K8s validation failed: %v", errs.ToAggregate()))
		}

		// Then run backend validation for cross-resource validation including port conflicts
		reader, err := w.backendClient.GetReader(ctx)
		if err != nil {
			return w.errorResponse(req.UID, fmt.Sprintf("Failed to get reader: %v", err))
		}
		defer reader.Close()

		validator := w.backendClient.GetDependencyValidator()
		bindingValidator := validator.GetAddressGroupBindingValidator()
		domainBinding := convertAddressGroupBindingToDomain(binding)

		// Use ValidateForCreation which includes port conflict checking
		if err := bindingValidator.ValidateForCreation(ctx, &domainBinding); err != nil {
			return w.errorResponse(req.UID, fmt.Sprintf("AddressGroupBinding validation failed: %v", err))
		}

	case admissionv1.Update:
		// Получаем Reader для валидации обновления
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
		k8sValidator := k8svalidation.NewAddressGroupPortMappingValidator()
		if errs := k8sValidator.ValidateCreate(ctx, &mapping); len(errs) > 0 {
			return w.errorResponse(req.UID, fmt.Sprintf("AddressGroupPortMapping K8s validation failed: %v", errs.ToAggregate()))
		}

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
	}

	return w.allowResponse(req.UID, "AddressGroupPortMapping validation passed")
}

func (w *ValidationWebhook) validateSvcSvcRule(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	var svcsvcRule netguardv1beta1.SvcSvcRule
	if err := json.Unmarshal(req.Object.Raw, &svcsvcRule); err != nil {
		return w.errorResponse(req.UID, fmt.Sprintf("Failed to unmarshal SvcSvcRule: %v", err))
	}

	switch req.Operation {
	case admissionv1.Create:
		k8sValidator := k8svalidation.NewSvcSvcRuleValidator()
		if errs := k8sValidator.ValidateCreate(ctx, &svcsvcRule); len(errs) > 0 {
			return w.errorResponse(req.UID, fmt.Sprintf("SvcSvcRule K8s validation failed: %v", errs.ToAggregate()))
		}

	case admissionv1.Update:
		var oldSvcSvcRule netguardv1beta1.SvcSvcRule
		if err := json.Unmarshal(req.OldObject.Raw, &oldSvcSvcRule); err != nil {
			return w.errorResponse(req.UID, fmt.Sprintf("Failed to unmarshal old SvcSvcRule: %v", err))
		}

		k8sValidator := k8svalidation.NewSvcSvcRuleValidator()
		if errs := k8sValidator.ValidateUpdate(ctx, &svcsvcRule, &oldSvcSvcRule); len(errs) > 0 {
			return w.errorResponse(req.UID, fmt.Sprintf("SvcSvcRule K8s validation failed: %v", errs.ToAggregate()))
		}

	case admissionv1.Delete:
		// No validation needed for delete
	}

	return w.allowResponse(req.UID, "SvcSvcRule validation passed")
}

func (w *ValidationWebhook) validateSvcFqdnRule(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	var rule netguardv1beta1.SvcFqdnRule
	if err := json.Unmarshal(req.Object.Raw, &rule); err != nil {
		return w.errorResponse(req.UID, fmt.Sprintf("Failed to unmarshal SvcFqdnRule: %v", err))
	}

	switch req.Operation {
	case admissionv1.Create:
		k8sValidator := k8svalidation.NewSvcFqdnRuleValidator()
		if errs := k8sValidator.ValidateCreate(ctx, &rule); len(errs) > 0 {
			return w.errorResponse(req.UID, fmt.Sprintf("SvcFqdnRule K8s validation failed: %v", errs.ToAggregate()))
		}
	case admissionv1.Update:
		var oldRule netguardv1beta1.SvcFqdnRule
		if err := json.Unmarshal(req.OldObject.Raw, &oldRule); err != nil {
			return w.errorResponse(req.UID, fmt.Sprintf("Failed to unmarshal old SvcFqdnRule: %v", err))
		}

		k8sValidator := k8svalidation.NewSvcFqdnRuleValidator()
		if errs := k8sValidator.ValidateUpdate(ctx, &rule, &oldRule); len(errs) > 0 {
			return w.errorResponse(req.UID, fmt.Sprintf("SvcFqdnRule K8s validation failed: %v", errs.ToAggregate()))
		}
	case admissionv1.Delete:
		// No additional validation for deletes.
	}

	return w.allowResponse(req.UID, "SvcFqdnRule validation passed")
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
		k8sValidator := k8svalidation.NewAddressGroupBindingPolicyValidator()
		if errs := k8sValidator.ValidateCreate(ctx, &policy); len(errs) > 0 {
			return w.errorResponse(req.UID, fmt.Sprintf("AddressGroupBindingPolicy K8s validation failed: %v", errs.ToAggregate()))
		}

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
	}

	return w.allowResponse(req.UID, "AddressGroupBindingPolicy validation passed")
}

func (w *ValidationWebhook) validateNetwork(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	var network netguardv1beta1.Network
	if err := json.Unmarshal(req.Object.Raw, &network); err != nil {
		return w.errorResponse(req.UID, fmt.Sprintf("Failed to unmarshal Network: %v", err))
	}

	// Получаем Reader для валидации
	reader, err := w.backendClient.GetReader(ctx)
	if err != nil {
		return w.errorResponse(req.UID, fmt.Sprintf("Failed to get reader: %v", err))
	}
	defer reader.Close()

	// Получаем валидатор
	validator := w.backendClient.GetDependencyValidator()
	networkValidator := validator.GetNetworkValidator()

	// Конвертируем в domain модель
	domainNetwork := convertNetworkToDomain(network)

	switch req.Operation {
	case admissionv1.Create:
		k8sValidator := k8svalidation.NewNetworkValidator()
		if errs := k8sValidator.ValidateCreate(ctx, &network); len(errs) > 0 {
			return w.errorResponse(req.UID, fmt.Sprintf("Network K8s validation failed: %v", errs.ToAggregate()))
		}

		if err := networkValidator.ValidateForCreation(ctx, domainNetwork); err != nil {
			return w.errorResponse(req.UID, fmt.Sprintf("Network validation failed: %v", err))
		}

	case admissionv1.Update:
		// Получаем старую версию для валидации обновления
		var oldNetwork netguardv1beta1.Network
		if err := json.Unmarshal(req.OldObject.Raw, &oldNetwork); err != nil {
			return w.errorResponse(req.UID, fmt.Sprintf("Failed to unmarshal old Network: %v", err))
		}

		oldDomainNetwork := convertNetworkToDomain(oldNetwork)

		// Валидация для обновления
		if err := networkValidator.ValidateForUpdate(ctx, oldDomainNetwork, domainNetwork); err != nil {
			return w.errorResponse(req.UID, fmt.Sprintf("Network update validation failed: %v", err))
		}

	case admissionv1.Delete:
		// Для Delete операций не используем валидацию - она будет в API Server при вызове backend
	}

	return w.allowResponse(req.UID, "Network validation passed")
}

func (w *ValidationWebhook) validateNetworkBinding(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	var networkBinding netguardv1beta1.NetworkBinding
	if err := json.Unmarshal(req.Object.Raw, &networkBinding); err != nil {
		return w.errorResponse(req.UID, fmt.Sprintf("Failed to unmarshal NetworkBinding: %v", err))
	}

	// Получаем Reader для валидации
	reader, err := w.backendClient.GetReader(ctx)
	if err != nil {
		return w.errorResponse(req.UID, fmt.Sprintf("Failed to get reader: %v", err))
	}
	defer reader.Close()

	// Получаем валидатор
	validator := w.backendClient.GetDependencyValidator()
	networkBindingValidator := validator.GetNetworkBindingValidator()

	// Конвертируем в domain модель
	domainNetworkBinding := convertNetworkBindingToDomain(networkBinding)

	switch req.Operation {
	case admissionv1.Create:
		k8sValidator := k8svalidation.NewNetworkBindingValidator()
		if errs := k8sValidator.ValidateCreate(ctx, &networkBinding); len(errs) > 0 {
			return w.errorResponse(req.UID, fmt.Sprintf("NetworkBinding K8s validation failed: %v", errs.ToAggregate()))
		}

		if err := networkBindingValidator.ValidateForCreation(ctx, domainNetworkBinding); err != nil {
			return w.errorResponse(req.UID, fmt.Sprintf("NetworkBinding validation failed: %v", err))
		}

	case admissionv1.Update:
		// Получаем старую версию для валидации обновления
		var oldNetworkBinding netguardv1beta1.NetworkBinding
		if err := json.Unmarshal(req.OldObject.Raw, &oldNetworkBinding); err != nil {
			return w.errorResponse(req.UID, fmt.Sprintf("Failed to unmarshal old NetworkBinding: %v", err))
		}

		oldDomainNetworkBinding := convertNetworkBindingToDomain(oldNetworkBinding)

		// Валидация для обновления
		if err := networkBindingValidator.ValidateForUpdate(ctx, oldDomainNetworkBinding, domainNetworkBinding); err != nil {
			return w.errorResponse(req.UID, fmt.Sprintf("NetworkBinding update validation failed: %v", err))
		}

	case admissionv1.Delete:
		// Для Delete операций не используем валидацию - она будет в API Server при вызове backend
	}

	return w.allowResponse(req.UID, "NetworkBinding validation passed")
}

func (w *ValidationWebhook) validateHostBinding(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	var hostBinding netguardv1beta1.HostBinding
	if err := json.Unmarshal(req.Object.Raw, &hostBinding); err != nil {
		return w.errorResponse(req.UID, fmt.Sprintf("Failed to unmarshal HostBinding: %v", err))
	}

	switch req.Operation {
	case admissionv1.Create:
		k8sValidator := k8svalidation.NewHostBindingValidator()
		if errs := k8sValidator.ValidateCreate(ctx, &hostBinding); len(errs) > 0 {
			return w.errorResponse(req.UID, fmt.Sprintf("HostBinding K8s validation failed: %v", errs.ToAggregate()))
		}

	case admissionv1.Update:
		// Получаем старую версию для валидации обновления
		var oldHostBinding netguardv1beta1.HostBinding
		if err := json.Unmarshal(req.OldObject.Raw, &oldHostBinding); err != nil {
			return w.errorResponse(req.UID, fmt.Sprintf("Failed to unmarshal old HostBinding: %v", err))
		}

		k8sValidator := k8svalidation.NewHostBindingValidator()
		if errs := k8sValidator.ValidateUpdate(ctx, &hostBinding, &oldHostBinding); len(errs) > 0 {
			return w.errorResponse(req.UID, fmt.Sprintf("HostBinding K8s validation failed: %v", errs.ToAggregate()))
		}

	case admissionv1.Delete:
		// Для Delete операций не используем валидацию - она будет в API Server при вызове backend
	}

	return w.allowResponse(req.UID, "HostBinding validation passed")
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
	// Конвертация Networks
	networks := make([]models.NetworkItem, len(k8sGroup.Networks))
	for i, item := range k8sGroup.Networks {
		networks[i] = models.NetworkItem{
			Name:       item.Name,
			CIDR:       item.CIDR,
			ApiVersion: item.ApiVersion,
			Kind:       item.Kind,
			Namespace:  item.Namespace,
		}
	}

	// Compute the AddressGroupName pattern
	var addressGroupName string
	if k8sGroup.Namespace != "" {
		addressGroupName = fmt.Sprintf("%s/%s", k8sGroup.Namespace, k8sGroup.Name)
	} else {
		addressGroupName = k8sGroup.Name
	}

	// Use status field if provided, otherwise use computed value
	if k8sGroup.Status.AddressGroupName != "" {
		addressGroupName = k8sGroup.Status.AddressGroupName
	}

	return models.AddressGroup{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sGroup.Name,
				Namespace: k8sGroup.Namespace,
			},
		},
		DefaultAction:    models.RuleAction(k8sGroup.Spec.DefaultAction),
		Logs:             k8sGroup.Spec.Logs,
		Trace:            k8sGroup.Spec.Trace,
		Networks:         networks,
		AddressGroupName: addressGroupName,
	}
}

func convertAddressGroupBindingToDomain(k8sBinding netguardv1beta1.AddressGroupBinding) models.AddressGroupBinding {
	domainBinding := models.AddressGroupBinding{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sBinding.Name,
				Namespace: k8sBinding.Namespace,
			},
		},
		ServiceRef:      k8sBinding.Spec.ServiceRef,      // Direct assignment - preserves namespace!
		AddressGroupRef: k8sBinding.Spec.AddressGroupRef, // Direct assignment - preserves namespace!
	}

	return domainBinding
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
		var serviceRef models.ServiceRef
		serviceRef.Name = accessPort.Name
		serviceRef.Namespace = accessPort.Namespace

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

func convertAddressGroupBindingPolicyToDomain(k8sPolicy netguardv1beta1.AddressGroupBindingPolicy) models.AddressGroupBindingPolicy {
	var serviceRef models.ServiceRef
	serviceRef.Name = k8sPolicy.Spec.ServiceRef.Name
	serviceRef.Namespace = k8sPolicy.Spec.ServiceRef.Namespace

	var addressGroupRef models.AddressGroupRef
	addressGroupRef.Name = k8sPolicy.Spec.AddressGroupRef.Name
	addressGroupRef.Namespace = k8sPolicy.Spec.AddressGroupRef.Namespace

	return models.AddressGroupBindingPolicy{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sPolicy.Name,
				Namespace: k8sPolicy.Namespace,
			},
		},
		ServiceRef:      serviceRef,
		AddressGroupRef: addressGroupRef,
		// Ports поля нет в domain модели AddressGroupBindingPolicy
	}
}

func convertNetworkToDomain(k8sNetwork netguardv1beta1.Network) models.Network {
	return models.Network{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sNetwork.Name,
				Namespace: k8sNetwork.Namespace,
			},
		},
		CIDR: k8sNetwork.Spec.CIDR,
		Meta: models.Meta{
			Generation:  k8sNetwork.Generation,
			Labels:      k8sNetwork.Labels,
			Annotations: k8sNetwork.Annotations,
		},
	}
}

func convertNetworkBindingToDomain(k8sBinding netguardv1beta1.NetworkBinding) models.NetworkBinding {
	return models.NetworkBinding{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sBinding.Name,
				Namespace: k8sBinding.Namespace,
			},
		},
		NetworkRef:      k8sBinding.Spec.NetworkRef,
		AddressGroupRef: k8sBinding.Spec.AddressGroupRef,
		Meta: models.Meta{
			Generation:  k8sBinding.Generation,
			Labels:      k8sBinding.Labels,
			Annotations: k8sBinding.Annotations,
		},
	}
}

func (w *ValidationWebhook) validateIECidrSvcRule(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	var rule netguardv1beta1.IECidrSvcRule
	if err := json.Unmarshal(req.Object.Raw, &rule); err != nil {
		return w.errorResponse(req.UID, fmt.Sprintf("Failed to unmarshal IECidrSvcRule: %v", err))
	}

	// Получаем Reader для валидации
	reader, err := w.backendClient.GetReader(ctx)
	if err != nil {
		return w.errorResponse(req.UID, fmt.Sprintf("Failed to get reader: %v", err))
	}
	defer reader.Close()

	// Получаем валидатор
	validator := w.backendClient.GetDependencyValidator()
	ruleValidator := validator.GetIECidrSvcRuleValidator()

	// Конвертируем в domain модель
	domainRule := convertIECidrSvcRuleToDomain(rule)

	switch req.Operation {
	case admissionv1.Create:
		// K8s structural validation
		k8sValidator := k8svalidation.NewIECidrSvcRuleValidator()
		if errs := k8sValidator.ValidateCreate(ctx, &rule); len(errs) > 0 {
			return w.errorResponse(req.UID, fmt.Sprintf("IECidrSvcRule K8s validation failed: %v", errs.ToAggregate()))
		}

		// Application validation (service exists + Ready)
		if err := ruleValidator.ValidateForCreation(ctx, domainRule); err != nil {
			return w.errorResponse(req.UID, fmt.Sprintf("IECidrSvcRule validation failed: %v", err))
		}

	case admissionv1.Update:
		var oldRule netguardv1beta1.IECidrSvcRule
		if err := json.Unmarshal(req.OldObject.Raw, &oldRule); err != nil {
			return w.errorResponse(req.UID, fmt.Sprintf("Failed to unmarshal old IECidrSvcRule: %v", err))
		}

		k8sValidator := k8svalidation.NewIECidrSvcRuleValidator()
		if errs := k8sValidator.ValidateUpdate(ctx, &rule, &oldRule); len(errs) > 0 {
			return w.errorResponse(req.UID, fmt.Sprintf("IECidrSvcRule K8s validation failed: %v", errs.ToAggregate()))
		}

		oldDomainRule := convertIECidrSvcRuleToDomain(oldRule)
		if err := ruleValidator.ValidateForUpdate(ctx, oldDomainRule, domainRule); err != nil {
			return w.errorResponse(req.UID, fmt.Sprintf("IECidrSvcRule validation failed: %v", err))
		}

	case admissionv1.Delete:
		// No additional validation for deletes
	}

	return w.allowResponse(req.UID, "IECidrSvcRule validation passed")
}

func convertIECidrSvcRuleToDomain(rule netguardv1beta1.IECidrSvcRule) models.IECidrSvcRule {
	ports := make([]models.IECidrSvcPortSpec, len(rule.Spec.Ports))
	for i, p := range rule.Spec.Ports {
		ports[i] = models.IECidrSvcPortSpec{S: p.S, D: p.D}
	}

	return models.IECidrSvcRule{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier(rule.Name, models.WithNamespace(rule.Namespace)),
		},
		Transport:   models.TransportProtocol(rule.Spec.Transport),
		CIDR:        rule.Spec.CIDR,
		ServiceRef:  rule.Spec.Svc,
		Traffic:     models.Traffic(rule.Spec.Traffic),
		Ports:       ports,
		Logs:        rule.Spec.Logs,
		Trace:       rule.Spec.Trace,
		Action:      models.RuleAction(rule.Spec.Action),
		Priority:    rule.Spec.Priority,
		Description: rule.Spec.Description,
		Comment:     rule.Spec.Comment,
	}
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
