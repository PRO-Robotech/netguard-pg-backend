package validation

import (
	"context"

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

// ServiceValidator provides methods for validating services
type ServiceValidator struct {
	reader ports.Reader
}

// NewServiceValidator creates a new service validator
func NewServiceValidator(reader ports.Reader) *ServiceValidator {
	return &ServiceValidator{
		reader: reader,
	}
}

// AddressGroupValidator provides methods for validating address groups
type AddressGroupValidator struct {
	reader ports.Reader
}

// NewAddressGroupValidator creates a new address group validator
func NewAddressGroupValidator(reader ports.Reader) *AddressGroupValidator {
	return &AddressGroupValidator{
		reader: reader,
	}
}

// AddressGroupBindingValidator provides methods for validating address group bindings
type AddressGroupBindingValidator struct {
	reader ports.Reader
}

// NewAddressGroupBindingValidator creates a new address group binding validator
func NewAddressGroupBindingValidator(reader ports.Reader) *AddressGroupBindingValidator {
	return &AddressGroupBindingValidator{
		reader: reader,
	}
}

// ServiceAliasValidator provides methods for validating service aliases
type ServiceAliasValidator struct {
	reader ports.Reader
}

// NewServiceAliasValidator creates a new service alias validator
func NewServiceAliasValidator(reader ports.Reader) *ServiceAliasValidator {
	return &ServiceAliasValidator{
		reader: reader,
	}
}

// RuleS2SValidator provides methods for validating rule s2s
type RuleS2SValidator struct {
	reader ports.Reader
}

// NewRuleS2SValidator creates a new rule s2s validator
func NewRuleS2SValidator(reader ports.Reader) *RuleS2SValidator {
	return &RuleS2SValidator{
		reader: reader,
	}
}

// AddressGroupPortMappingValidator provides methods for validating address group port mappings
type AddressGroupPortMappingValidator struct {
	reader ports.Reader
}

// NewAddressGroupPortMappingValidator creates a new address group port mapping validator
func NewAddressGroupPortMappingValidator(reader ports.Reader) *AddressGroupPortMappingValidator {
	return &AddressGroupPortMappingValidator{
		reader: reader,
	}
}
