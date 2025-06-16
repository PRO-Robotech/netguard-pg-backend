package validation

import (
	"context"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"

	"github.com/pkg/errors"
)

// ValidateExists checks if a service exists
func (v *ServiceValidator) ValidateExists(ctx context.Context, id models.ResourceIdentifier) error {
	exists := false
	err := v.reader.ListServices(ctx, func(service models.Service) error {
		if service.Key() == id.Key() {
			exists = true
		}
		return nil
	}, ports.NewResourceIdentifierScope(id))

	if err != nil {
		return errors.Wrap(err, "failed to check service existence")
	}

	if !exists {
		return NewEntityNotFoundError("service", id.Key())
	}

	return nil
}

// ValidateReferences checks if all references in a service are valid
func (v *ServiceValidator) ValidateReferences(ctx context.Context, service models.Service) error {
	agValidator := NewAddressGroupValidator(v.reader)

	for _, agRef := range service.AddressGroups {
		if err := agValidator.ValidateExists(ctx, agRef.ResourceIdentifier); err != nil {
			return errors.Wrapf(err, "invalid address group reference in service %s", service.Key())
		}
	}

	return nil
}

// ValidateForCreation validates a service before creation
func (v *ServiceValidator) ValidateForCreation(ctx context.Context, service models.Service) error {
	return v.ValidateReferences(ctx, service)
}

// CheckDependencies checks if there are dependencies before deleting a service
func (v *ServiceValidator) CheckDependencies(ctx context.Context, id models.ResourceIdentifier) error {
	// Check ServiceAliases referencing the service to be deleted
	hasAliases := false
	err := v.reader.ListServiceAliases(ctx, func(alias models.ServiceAlias) error {
		if alias.ServiceRef.Key() == id.Key() {
			hasAliases = true
		}
		return nil
	}, nil)

	if err != nil {
		return errors.Wrap(err, "failed to check service aliases")
	}

	if hasAliases {
		return NewDependencyExistsError("service", id.Key(), "service_alias")
	}

	// Check AddressGroupBindings referencing the service to be deleted
	hasBindings := false
	err = v.reader.ListAddressGroupBindings(ctx, func(binding models.AddressGroupBinding) error {
		if binding.ServiceRef.Key() == id.Key() {
			hasBindings = true
		}
		return nil
	}, nil)

	if err != nil {
		return errors.Wrap(err, "failed to check address group bindings")
	}

	if hasBindings {
		return NewDependencyExistsError("service", id.Key(), "address_group_binding")
	}

	return nil
}
