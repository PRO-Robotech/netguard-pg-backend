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
	// 🔍 COMPREHENSIVE WEBHOOK TRACING - Start
	log.Printf("🔍 WEBHOOK DISPATCHER START: %s %s/%s operation=%s, UID=%s", req.Kind.Kind, req.Namespace, req.Name, req.Operation, req.UID)
	log.Printf("🔍 WEBHOOK DISPATCHER: Request details - GroupVersion=%s, Kind=%s", req.Kind, req.Kind.Kind)
	log.Printf("🔍 WEBHOOK DISPATCHER: Resource details - Namespace=%s, Name=%s", req.Namespace, req.Name)

	var response *admissionv1.AdmissionResponse
	switch req.Kind.Kind {
	case "Service":
		log.Printf("🔍 WEBHOOK DISPATCHER: Routing to validateService for %s/%s", req.Namespace, req.Name)
		response = w.validateService(ctx, req)
	case "AddressGroup":
		log.Printf("🔍 WEBHOOK DISPATCHER: Routing to validateAddressGroup for %s/%s", req.Namespace, req.Name)
		response = w.validateAddressGroup(ctx, req)
	case "AddressGroupBinding":
		log.Printf("🔍 WEBHOOK DISPATCHER: Routing to validateAddressGroupBinding for %s/%s", req.Namespace, req.Name)
		response = w.validateAddressGroupBinding(ctx, req)
	case "AddressGroupPortMapping":
		log.Printf("🔍 WEBHOOK DISPATCHER: Routing to validateAddressGroupPortMapping for %s/%s", req.Namespace, req.Name)
		response = w.validateAddressGroupPortMapping(ctx, req)
	case "RuleS2S":
		log.Printf("🔍 WEBHOOK DISPATCHER: Routing to validateRuleS2S for %s/%s", req.Namespace, req.Name)
		response = w.validateRuleS2S(ctx, req)
	case "ServiceAlias":
		log.Printf("🔍 WEBHOOK DISPATCHER: Routing to validateServiceAlias for %s/%s", req.Namespace, req.Name)
		response = w.validateServiceAlias(ctx, req)
	case "AddressGroupBindingPolicy":
		log.Printf("🔍 WEBHOOK DISPATCHER: Routing to validateAddressGroupBindingPolicy for %s/%s", req.Namespace, req.Name)
		response = w.validateAddressGroupBindingPolicy(ctx, req)
	case "IEAgAgRule":
		log.Printf("🔍 WEBHOOK DISPATCHER: Routing to validateIEAgAgRule for %s/%s", req.Namespace, req.Name)
		response = w.validateIEAgAgRule(ctx, req)
	case "Network":
		log.Printf("🔍 WEBHOOK DISPATCHER: Routing to validateNetwork for %s/%s", req.Namespace, req.Name)
		response = w.validateNetwork(ctx, req)
	case "NetworkBinding":
		log.Printf("🔍 WEBHOOK DISPATCHER: Routing to validateNetworkBinding for %s/%s", req.Namespace, req.Name)
		response = w.validateNetworkBinding(ctx, req)
	default:
		log.Printf("🔍 WEBHOOK DISPATCHER: Unknown resource kind: %s", req.Kind.Kind)
		response = w.errorResponse(req.UID, fmt.Sprintf("Unknown resource kind: %s", req.Kind.Kind))
	}

	// Log the response before returning
	if response.Allowed {
		log.Printf("🔍 WEBHOOK DISPATCHER END: %s %s/%s - ALLOWED", req.Kind.Kind, req.Namespace, req.Name)
	} else {
		log.Printf("🔍 WEBHOOK DISPATCHER END: %s %s/%s - DENIED: %s", req.Kind.Kind, req.Namespace, req.Name, response.Result.Message)
	}
	return response
}

func (w *ValidationWebhook) validateService(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	// 🔍 SERVICE WEBHOOK ENTRY POINT
	log.Printf("🔍 SERVICE WEBHOOK ENTRY: %s %s/%s operation=%s, UID=%s", req.Kind.Kind, req.Namespace, req.Name, req.Operation, req.UID)

	// CRITICAL CHECK: This should ONLY be called for Service resources
	if req.Kind.Kind != "Service" {
		log.Printf("🚨 SERVICE WEBHOOK ERROR: Called for non-Service resource %s! This is the cross-validation bug!", req.Kind.Kind)
		return w.errorResponse(req.UID, fmt.Sprintf("Service webhook incorrectly called for %s resource", req.Kind.Kind))
	}

	// 🔧 FIX: Handle DELETE operations separately - no object to unmarshal
	if req.Operation == admissionv1.Delete {
		log.Printf("🔧 FIX: DELETE operation for Service %s/%s - performing dependency validation", req.Namespace, req.Name)

		// Get validator for dependency checking
		validator := w.backendClient.GetDependencyValidator()
		serviceValidator := validator.GetServiceValidator()

		// Check dependencies before deletion
		serviceID := models.NewResourceIdentifier(req.Name, models.WithNamespace(req.Namespace))
		if err := serviceValidator.CheckDependencies(ctx, serviceID); err != nil {
			log.Printf("🔧 FIX: Service DELETE validation failed for %s/%s: %v", req.Namespace, req.Name, err)
			return w.errorResponse(req.UID, fmt.Sprintf("Cannot delete Service: %v", err))
		}

		log.Printf("🔧 FIX: Service DELETE validation passed for %s/%s", req.Namespace, req.Name)
		return w.allowResponse(req.UID, "Service deletion validation passed")
	}

	// For CREATE and UPDATE operations, unmarshal the object
	var service netguardv1beta1.Service
	if err := json.Unmarshal(req.Object.Raw, &service); err != nil {
		log.Printf("🔍 SERVICE WEBHOOK: Failed to unmarshal Service %s/%s: %v", req.Namespace, req.Name, err)
		return w.errorResponse(req.UID, fmt.Sprintf("Failed to unmarshal Service: %v", err))
	}
	log.Printf("🔍 SERVICE WEBHOOK: Successfully unmarshaled Service %s/%s", req.Namespace, req.Name)

	// Получаем Reader для валидации
	reader, err := w.backendClient.GetReader(ctx)
	if err != nil {
		log.Printf("🔍 SERVICE WEBHOOK: Failed to get reader for %s/%s: %v", req.Namespace, req.Name, err)
		return w.errorResponse(req.UID, fmt.Sprintf("Failed to get reader: %v", err))
	}
	defer reader.Close()

	// Получаем валидатор
	validator := w.backendClient.GetDependencyValidator()
	serviceValidator := validator.GetServiceValidator()
	log.Printf("🔍 SERVICE WEBHOOK: Got service validator for %s/%s", req.Namespace, req.Name)

	// Конвертируем в domain модель
	domainService := convertServiceToDomain(service)
	log.Printf("🔍 SERVICE WEBHOOK: Converted to domain model for %s/%s", req.Namespace, req.Name)

	switch req.Operation {
	case admissionv1.Create:
		log.Printf("🔧 FIX: Create operation for Service %s/%s - using proper backend validation with port overlap checking", req.Namespace, req.Name)

		// First run K8s-level validation for basic field validation
		k8sValidator := k8svalidation.NewServiceValidator()
		if errs := k8sValidator.ValidateCreate(ctx, &service); len(errs) > 0 {
			return w.errorResponse(req.UID, fmt.Sprintf("Service K8s validation failed: %v", errs.ToAggregate()))
		}

		// Then run backend validation for port overlap checking
		// This includes ValidateNoDuplicatePorts which checks for overlapping ranges
		if err := serviceValidator.ValidateForCreation(ctx, domainService); err != nil {
			log.Printf("🔧 FIX: Service CREATE validation failed for %s/%s: %v", req.Namespace, req.Name, err)
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
	// 🔧 FIX: Handle DELETE operations separately - no object to unmarshal
	if req.Operation == admissionv1.Delete {
		log.Printf("🔧 FIX: DELETE operation for AddressGroup %s/%s - performing dependency validation", req.Namespace, req.Name)

		// Get validator for dependency checking
		validator := w.backendClient.GetDependencyValidator()
		addressGroupValidator := validator.GetAddressGroupValidator()

		// Check dependencies before deletion
		addressGroupID := models.NewResourceIdentifier(req.Name, models.WithNamespace(req.Namespace))
		if err := addressGroupValidator.CheckDependencies(ctx, addressGroupID); err != nil {
			log.Printf("🔧 FIX: AddressGroup DELETE validation failed for %s/%s: %v", req.Namespace, req.Name, err)
			return w.errorResponse(req.UID, fmt.Sprintf("Cannot delete AddressGroup: %v", err))
		}

		log.Printf("🔧 FIX: AddressGroup DELETE validation passed for %s/%s", req.Namespace, req.Name)
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
		log.Printf("Delete operation for AddressGroup %s/%s - validation will be done in API Server", req.Namespace, req.Name)
	}

	return w.allowResponse(req.UID, "AddressGroup validation passed")
}

func (w *ValidationWebhook) validateAddressGroupBinding(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	// 🔍 ADDRESSGROUPBINDING WEBHOOK ENTRY POINT
	log.Printf("🔍 BINDING WEBHOOK ENTRY: %s %s/%s operation=%s, UID=%s", req.Kind.Kind, req.Namespace, req.Name, req.Operation, req.UID)

	// CRITICAL CHECK: This should ONLY be called for AddressGroupBinding resources
	if req.Kind.Kind != "AddressGroupBinding" {
		log.Printf("🚨 BINDING WEBHOOK ERROR: Called for non-AddressGroupBinding resource %s! This should not happen!", req.Kind.Kind)
		return w.errorResponse(req.UID, fmt.Sprintf("AddressGroupBinding webhook incorrectly called for %s resource", req.Kind.Kind))
	}

	// 🔧 FIX: Handle DELETE operations separately - no object to unmarshal
	if req.Operation == admissionv1.Delete {
		log.Printf("🔧 FIX: DELETE operation for AddressGroupBinding %s/%s - performing dependency validation", req.Namespace, req.Name)

		// Get validator for dependency checking
		validator := w.backendClient.GetDependencyValidator()
		bindingValidator := validator.GetAddressGroupBindingValidator()

		// Check dependencies before deletion
		bindingID := models.NewResourceIdentifier(req.Name, models.WithNamespace(req.Namespace))
		if err := bindingValidator.CheckDependencies(ctx, bindingID); err != nil {
			log.Printf("🔧 FIX: AddressGroupBinding DELETE validation failed for %s/%s: %v", req.Namespace, req.Name, err)
			return w.errorResponse(req.UID, fmt.Sprintf("Cannot delete AddressGroupBinding: %v", err))
		}

		log.Printf("🔧 FIX: AddressGroupBinding DELETE validation passed for %s/%s", req.Namespace, req.Name)
		return w.allowResponse(req.UID, "AddressGroupBinding deletion validation passed")
	}

	var binding netguardv1beta1.AddressGroupBinding
	if err := json.Unmarshal(req.Object.Raw, &binding); err != nil {
		log.Printf("🔍 BINDING WEBHOOK: Failed to unmarshal AddressGroupBinding %s/%s: %v", req.Namespace, req.Name, err)
		return w.errorResponse(req.UID, fmt.Sprintf("Failed to unmarshal AddressGroupBinding: %v", err))
	}
	log.Printf("🔍 BINDING WEBHOOK: Successfully unmarshaled AddressGroupBinding %s/%s", req.Namespace, req.Name)

	switch req.Operation {
	case admissionv1.Create:
		log.Printf("🔧 FIX: CREATE AddressGroupBinding %s/%s - using proper backend validation with port conflict checking", req.Namespace, req.Name)

		// First run K8s-level validation for basic field validation
		k8sValidator := k8svalidation.NewAddressGroupBindingValidator()
		if errs := k8sValidator.ValidateCreate(ctx, &binding); len(errs) > 0 {
			return w.errorResponse(req.UID, fmt.Sprintf("AddressGroupBinding K8s validation failed: %v", errs.ToAggregate()))
		}

		// Then run backend validation for cross-resource validation including port conflicts
		reader, err := w.backendClient.GetReader(ctx)
		if err != nil {
			log.Printf("🔧 FIX: Failed to get reader for AddressGroupBinding %s/%s: %v", req.Namespace, req.Name, err)
			return w.errorResponse(req.UID, fmt.Sprintf("Failed to get reader: %v", err))
		}
		defer reader.Close()

		validator := w.backendClient.GetDependencyValidator()
		bindingValidator := validator.GetAddressGroupBindingValidator()
		domainBinding := convertAddressGroupBindingToDomain(binding)

		// Use ValidateForCreation which includes port conflict checking
		if err := bindingValidator.ValidateForCreation(ctx, &domainBinding); err != nil {
			log.Printf("🔧 FIX: AddressGroupBinding CREATE validation failed for %s/%s: %v", req.Namespace, req.Name, err)
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
		log.Printf("Delete operation for AddressGroupPortMapping %s/%s - validation will be done in API Server", req.Namespace, req.Name)
	}

	return w.allowResponse(req.UID, "AddressGroupPortMapping validation passed")
}

func (w *ValidationWebhook) validateRuleS2S(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	// 🔧 FIX: Handle DELETE operations separately - no object to unmarshal
	if req.Operation == admissionv1.Delete {
		log.Printf("🔧 FIX: DELETE operation for RuleS2S %s/%s - performing dependency validation", req.Namespace, req.Name)

		// Get validator for dependency checking
		validator := w.backendClient.GetDependencyValidator()
		ruleValidator := validator.GetRuleS2SValidator()

		// Check dependencies before deletion
		ruleID := models.NewResourceIdentifier(req.Name, models.WithNamespace(req.Namespace))
		if err := ruleValidator.CheckDependencies(ctx, ruleID); err != nil {
			log.Printf("🔧 FIX: RuleS2S DELETE validation failed for %s/%s: %v", req.Namespace, req.Name, err)
			return w.errorResponse(req.UID, fmt.Sprintf("Cannot delete RuleS2S: %v", err))
		}

		log.Printf("🔧 FIX: RuleS2S DELETE validation passed for %s/%s", req.Namespace, req.Name)
		return w.allowResponse(req.UID, "RuleS2S deletion validation passed")
	}

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
		k8sValidator := k8svalidation.NewRuleS2SValidator()
		if errs := k8sValidator.ValidateCreate(ctx, &rule); len(errs) > 0 {
			return w.errorResponse(req.UID, fmt.Sprintf("RuleS2S K8s validation failed: %v", errs.ToAggregate()))
		}

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
	// 🔧 FIX: Handle DELETE operations separately - no object to unmarshal
	if req.Operation == admissionv1.Delete {
		log.Printf("🔧 FIX: DELETE operation for ServiceAlias %s/%s - performing dependency validation", req.Namespace, req.Name)

		// Get validator for dependency checking
		validator := w.backendClient.GetDependencyValidator()
		serviceAliasValidator := validator.GetServiceAliasValidator()

		// Check dependencies before deletion
		serviceAliasID := models.NewResourceIdentifier(req.Name, models.WithNamespace(req.Namespace))
		if err := serviceAliasValidator.CheckDependencies(ctx, serviceAliasID); err != nil {
			log.Printf("🔧 FIX: ServiceAlias DELETE validation failed for %s/%s: %v", req.Namespace, req.Name, err)
			return w.errorResponse(req.UID, fmt.Sprintf("Cannot delete ServiceAlias: %v", err))
		}

		log.Printf("🔧 FIX: ServiceAlias DELETE validation passed for %s/%s", req.Namespace, req.Name)
		return w.allowResponse(req.UID, "ServiceAlias deletion validation passed")
	}

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
		k8sValidator := k8svalidation.NewServiceAliasValidator()
		if errs := k8sValidator.ValidateCreate(ctx, &alias); len(errs) > 0 {
			return w.errorResponse(req.UID, fmt.Sprintf("ServiceAlias K8s validation failed: %v", errs.ToAggregate()))
		}

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
		// For DELETE operations, validate dependencies to prevent orphaned references
		log.Printf("Delete operation for ServiceAlias %s/%s - validating dependencies", req.Namespace, req.Name)

		// Get backend client to check dependencies
		if w.backendClient == nil {
			return w.errorResponse(req.UID, "Backend client not available for dependency validation")
		}

		// Use dependency validator to check if ServiceAlias can be deleted
		serviceAliasID := models.ResourceIdentifier{
			Name:      req.Name,
			Namespace: req.Namespace,
		}

		// Get a reader to check dependencies
		reader, err := w.backendClient.GetReader(ctx)
		if err != nil {
			log.Printf("Failed to get reader for ServiceAlias dependency validation: %v", err)
			return w.errorResponse(req.UID, fmt.Sprintf("Failed to access backend for dependency validation: %v", err))
		}
		defer reader.Close()

		validator := validation.NewDependencyValidator(reader)
		aliasValidator := validator.GetServiceAliasValidator()

		// Check if ServiceAlias has dependencies that would prevent deletion
		if err := aliasValidator.CheckDependencies(ctx, serviceAliasID); err != nil {
			log.Printf("ServiceAlias %s/%s cannot be deleted due to dependencies: %v", req.Namespace, req.Name, err)
			return w.errorResponse(req.UID, fmt.Sprintf("Cannot delete ServiceAlias: %v", err))
		}

		log.Printf("ServiceAlias %s/%s deletion validated - no blocking dependencies found", req.Namespace, req.Name)
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
		log.Printf("Delete operation for AddressGroupBindingPolicy %s/%s - validation will be done in API Server", req.Namespace, req.Name)
	}

	return w.allowResponse(req.UID, "AddressGroupBindingPolicy validation passed")
}

func (w *ValidationWebhook) validateIEAgAgRule(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	var ieAgAgRule netguardv1beta1.IEAgAgRule
	if err := json.Unmarshal(req.Object.Raw, &ieAgAgRule); err != nil {
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
	ieAgAgRuleValidator := validator.GetIEAgAgRuleValidator()

	// Конвертируем в domain модель
	domainIEAgAgRule := convertIEAgAgRuleToDomain(ieAgAgRule)

	switch req.Operation {
	case admissionv1.Create:
		k8sValidator := k8svalidation.NewIEAgAgRuleValidator()
		if errs := k8sValidator.ValidateCreate(ctx, &ieAgAgRule); len(errs) > 0 {
			return w.errorResponse(req.UID, fmt.Sprintf("IEAgAgRule K8s validation failed: %v", errs.ToAggregate()))
		}

		if err := ieAgAgRuleValidator.ValidateForCreation(ctx, domainIEAgAgRule); err != nil {
			return w.errorResponse(req.UID, fmt.Sprintf("IEAgAgRule validation failed: %v", err))
		}

	case admissionv1.Update:
		// Получаем старую версию для валидации обновления
		var oldIEAgAgRule netguardv1beta1.IEAgAgRule
		if err := json.Unmarshal(req.OldObject.Raw, &oldIEAgAgRule); err != nil {
			return w.errorResponse(req.UID, fmt.Sprintf("Failed to unmarshal old IEAgAgRule: %v", err))
		}

		oldDomainIEAgAgRule := convertIEAgAgRuleToDomain(oldIEAgAgRule)

		// Валидация для обновления
		if err := ieAgAgRuleValidator.ValidateForUpdate(ctx, oldDomainIEAgAgRule, domainIEAgAgRule); err != nil {
			return w.errorResponse(req.UID, fmt.Sprintf("IEAgAgRule update validation failed: %v", err))
		}

	case admissionv1.Delete:
		// Для Delete операций не используем валидацию - она будет в API Server при вызове backend
		log.Printf("Delete operation for IEAgAgRule %s/%s - validation will be done in API Server", req.Namespace, req.Name)
	}

	return w.allowResponse(req.UID, "IEAgAgRule validation passed")
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
		log.Printf("Delete operation for Network %s/%s - validation will be done in API Server", req.Namespace, req.Name)
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
		log.Printf("Delete operation for NetworkBinding %s/%s - validation will be done in API Server", req.Namespace, req.Name)
	}

	return w.allowResponse(req.UID, "NetworkBinding validation passed")
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
		AddressGroupName: k8sGroup.Status.AddressGroupName,
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

	// 🔍 EXTENSIVE DEBUG: Log the resulting domain model
	log.Printf("🔧   Domain model ServiceRef: name=%s, namespace=%s",
		domainBinding.ServiceRef.Name, domainBinding.ServiceRef.Namespace)
	log.Printf("🔧   Domain model AddressGroupRef: name=%s, namespace=%s",
		domainBinding.AddressGroupRef.Name, domainBinding.AddressGroupRef.Namespace)

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
	rule := models.RuleS2S{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sRule.Name,
				Namespace: k8sRule.Namespace,
			},
		},
		Traffic: models.Traffic(k8sRule.Spec.Traffic),
		Trace:   k8sRule.Spec.Trace,
		ServiceLocalRef: func() netguardv1beta1.NamespacedObjectReference {
			var ref netguardv1beta1.NamespacedObjectReference
			ref.Name = k8sRule.Spec.ServiceLocalRef.Name
			ref.Namespace = k8sRule.Spec.ServiceLocalRef.Namespace
			return ref
		}(),
		ServiceRef: func() netguardv1beta1.NamespacedObjectReference {
			var ref netguardv1beta1.NamespacedObjectReference
			ref.Name = k8sRule.Spec.ServiceRef.Name
			ref.Namespace = k8sRule.Spec.ServiceRef.Namespace
			return ref
		}(),
	}

	// Convert IEAgAgRuleRefs from status
	if len(k8sRule.Status.IEAgAgRuleRefs) > 0 {
		rule.IEAgAgRuleRefs = make([]netguardv1beta1.NamespacedObjectReference, len(k8sRule.Status.IEAgAgRuleRefs))
		for i, ref := range k8sRule.Status.IEAgAgRuleRefs {
			var objRef netguardv1beta1.NamespacedObjectReference
			objRef.Name = ref.Name
			objRef.Namespace = ref.Namespace
			rule.IEAgAgRuleRefs[i] = objRef
		}
	}

	return rule
}

func convertServiceAliasToDomain(k8sAlias netguardv1beta1.ServiceAlias) models.ServiceAlias {
	var serviceRef models.ServiceRef
	serviceRef.Name = k8sAlias.Spec.ServiceRef.Name
	serviceRef.Namespace = k8sAlias.Namespace // ObjectReference не имеет Namespace, используем namespace самого объекта

	return models.ServiceAlias{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      k8sAlias.Name,
				Namespace: k8sAlias.Namespace,
			},
		},
		ServiceRef: serviceRef,
	}
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

func convertIEAgAgRuleToDomain(k8sRule netguardv1beta1.IEAgAgRule) models.IEAgAgRule {
	// Create embedded struct instances using variable assignment
	var selfRefResourceId models.ResourceIdentifier
	selfRefResourceId.Name = k8sRule.Name
	selfRefResourceId.Namespace = k8sRule.Namespace

	var selfRef models.SelfRef
	selfRef.ResourceIdentifier = selfRefResourceId

	var addressGroupLocal models.AddressGroupRef
	addressGroupLocal.Name = k8sRule.Spec.AddressGroupLocal.Name
	addressGroupLocal.Namespace = k8sRule.Namespace // ObjectReference не имеет Namespace, используем namespace самого объекта

	var addressGroup models.AddressGroupRef
	addressGroup.Name = k8sRule.Spec.AddressGroup.Name
	addressGroup.Namespace = k8sRule.Namespace // ObjectReference не имеет Namespace, используем namespace самого объекта

	rule := models.IEAgAgRule{
		SelfRef:           selfRef,
		Transport:         models.TransportProtocol(k8sRule.Spec.Transport),
		Traffic:           models.Traffic(k8sRule.Spec.Traffic),
		Action:            models.RuleAction(k8sRule.Spec.Action),
		Trace:             k8sRule.Spec.Trace,
		AddressGroupLocal: addressGroupLocal,
		AddressGroup:      addressGroup,
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
