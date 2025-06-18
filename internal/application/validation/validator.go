package validation

import (
	"context"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

// Validator defines a common interface for all validators
type Validator interface {
	// Validate checks if the object complies with the rules
	Validate(ctx context.Context) error
}

// DependencyValidator provides methods for validating dependencies between objects
type DependencyValidator struct {
	reader ports.Reader
}

// NewDependencyValidator creates a new dependency validator
func NewDependencyValidator(reader ports.Reader) *DependencyValidator {
	return &DependencyValidator{
		reader: reader,
	}
}

// GetServiceValidator returns a validator for services
func (v *DependencyValidator) GetServiceValidator() *ServiceValidator {
	return NewServiceValidator(v.reader)
}

// GetAddressGroupValidator returns a validator for address groups
func (v *DependencyValidator) GetAddressGroupValidator() *AddressGroupValidator {
	return NewAddressGroupValidator(v.reader)
}

// GetAddressGroupBindingValidator returns a validator for address group bindings
func (v *DependencyValidator) GetAddressGroupBindingValidator() *AddressGroupBindingValidator {
	return NewAddressGroupBindingValidator(v.reader)
}

// GetServiceAliasValidator returns a validator for service aliases
func (v *DependencyValidator) GetServiceAliasValidator() *ServiceAliasValidator {
	return NewServiceAliasValidator(v.reader)
}

// GetRuleS2SValidator returns a validator for rule s2s
func (v *DependencyValidator) GetRuleS2SValidator() *RuleS2SValidator {
	return NewRuleS2SValidator(v.reader)
}

// GetAddressGroupPortMappingValidator returns a validator for address group port mappings
func (v *DependencyValidator) GetAddressGroupPortMappingValidator() *AddressGroupPortMappingValidator {
	return NewAddressGroupPortMappingValidator(v.reader)
}

// GetAddressGroupBindingPolicyValidator returns a validator for address group binding policies
func (v *DependencyValidator) GetAddressGroupBindingPolicyValidator() *AddressGroupBindingPolicyValidator {
	return NewAddressGroupBindingPolicyValidator(v.reader)
}

// GetIEAgAgRuleValidator returns a validator for IEAgAgRule
func (v *DependencyValidator) GetIEAgAgRuleValidator() *IEAgAgRuleValidator {
	return NewIEAgAgRuleValidator(v.reader)
}

// ServiceValidator provides methods for validating services
type ServiceValidator struct {
	reader        ports.Reader
	BaseValidator *BaseValidator
}

// NewServiceValidator creates a new service validator
func NewServiceValidator(reader ports.Reader) *ServiceValidator {
	listFunction := func(ctx context.Context, consume func(entity interface{}) error, scope ports.Scope) error {
		return reader.ListServices(ctx, func(service models.Service) error {
			return consume(service)
		}, scope)
	}

	return &ServiceValidator{
		reader:        reader,
		BaseValidator: NewBaseValidator(reader, "service", listFunction),
	}
}

// AddressGroupValidator provides methods for validating address groups
type AddressGroupValidator struct {
	reader        ports.Reader
	BaseValidator *BaseValidator
}

// NewAddressGroupValidator creates a new address group validator
func NewAddressGroupValidator(reader ports.Reader) *AddressGroupValidator {
	listFunction := func(ctx context.Context, consume func(entity interface{}) error, scope ports.Scope) error {
		return reader.ListAddressGroups(ctx, func(group models.AddressGroup) error {
			return consume(group)
		}, scope)
	}

	return &AddressGroupValidator{
		reader:        reader,
		BaseValidator: NewBaseValidator(reader, "address_group", listFunction),
	}
}

// AddressGroupBindingValidator provides methods for validating address group bindings
type AddressGroupBindingValidator struct {
	reader        ports.Reader
	BaseValidator *BaseValidator
}

// NewAddressGroupBindingValidator creates a new address group binding validator
func NewAddressGroupBindingValidator(reader ports.Reader) *AddressGroupBindingValidator {
	listFunction := func(ctx context.Context, consume func(entity interface{}) error, scope ports.Scope) error {
		return reader.ListAddressGroupBindings(ctx, func(binding models.AddressGroupBinding) error {
			return consume(binding)
		}, scope)
	}

	return &AddressGroupBindingValidator{
		reader:        reader,
		BaseValidator: NewBaseValidator(reader, "address_group_binding", listFunction),
	}
}

// ServiceAliasValidator provides methods for validating service aliases
type ServiceAliasValidator struct {
	reader        ports.Reader
	BaseValidator *BaseValidator
}

// NewServiceAliasValidator creates a new service alias validator
func NewServiceAliasValidator(reader ports.Reader) *ServiceAliasValidator {
	listFunction := func(ctx context.Context, consume func(entity interface{}) error, scope ports.Scope) error {
		return reader.ListServiceAliases(ctx, func(alias models.ServiceAlias) error {
			return consume(alias)
		}, scope)
	}

	return &ServiceAliasValidator{
		reader:        reader,
		BaseValidator: NewBaseValidator(reader, "service_alias", listFunction),
	}
}

// RuleS2SValidator provides methods for validating rule s2s
type RuleS2SValidator struct {
	reader        ports.Reader
	BaseValidator *BaseValidator
}

// NewRuleS2SValidator creates a new rule s2s validator
func NewRuleS2SValidator(reader ports.Reader) *RuleS2SValidator {
	listFunction := func(ctx context.Context, consume func(entity interface{}) error, scope ports.Scope) error {
		return reader.ListRuleS2S(ctx, func(rule models.RuleS2S) error {
			return consume(rule)
		}, scope)
	}

	return &RuleS2SValidator{
		reader:        reader,
		BaseValidator: NewBaseValidator(reader, "rule_s2s", listFunction),
	}
}

// AddressGroupPortMappingValidator provides methods for validating address group port mappings
type AddressGroupPortMappingValidator struct {
	reader        ports.Reader
	BaseValidator *BaseValidator
}

// NewAddressGroupPortMappingValidator creates a new address group port mapping validator
func NewAddressGroupPortMappingValidator(reader ports.Reader) *AddressGroupPortMappingValidator {
	listFunction := func(ctx context.Context, consume func(entity interface{}) error, scope ports.Scope) error {
		return reader.ListAddressGroupPortMappings(ctx, func(mapping models.AddressGroupPortMapping) error {
			return consume(mapping)
		}, scope)
	}

	return &AddressGroupPortMappingValidator{
		reader:        reader,
		BaseValidator: NewBaseValidator(reader, "address_group_port_mapping", listFunction),
	}
}

// AddressGroupBindingPolicyValidator provides methods for validating address group binding policies
type AddressGroupBindingPolicyValidator struct {
	reader        ports.Reader
	BaseValidator *BaseValidator
}

// NewAddressGroupBindingPolicyValidator creates a new address group binding policy validator
func NewAddressGroupBindingPolicyValidator(reader ports.Reader) *AddressGroupBindingPolicyValidator {
	listFunction := func(ctx context.Context, consume func(entity interface{}) error, scope ports.Scope) error {
		return reader.ListAddressGroupBindingPolicies(ctx, func(policy models.AddressGroupBindingPolicy) error {
			return consume(policy)
		}, scope)
	}

	return &AddressGroupBindingPolicyValidator{
		reader:        reader,
		BaseValidator: NewBaseValidator(reader, "address_group_binding_policy", listFunction),
	}
}
