package services

import (
	"context"

	"github.com/pkg/errors"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

// NetguardService provides operations for managing netguard resources
type NetguardService struct {
	registry ports.Registry
}

// NewNetguardService creates a new NetguardService
func NewNetguardService(registry ports.Registry) *NetguardService {
	return &NetguardService{
		registry: registry,
	}
}

// GetServices returns all services
func (s *NetguardService) GetServices(ctx context.Context, scope ports.Scope) ([]models.Service, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	var services []models.Service
	err = reader.ListServices(ctx, func(service models.Service) error {
		services = append(services, service)
		return nil
	}, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list services")
	}
	return services, nil
}

// GetAddressGroups returns all address groups
func (s *NetguardService) GetAddressGroups(ctx context.Context, scope ports.Scope) ([]models.AddressGroup, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	var addressGroups []models.AddressGroup
	err = reader.ListAddressGroups(ctx, func(addressGroup models.AddressGroup) error {
		addressGroups = append(addressGroups, addressGroup)
		return nil
	}, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list address groups")
	}
	return addressGroups, nil
}

// GetAddressGroupBindings returns all address group bindings
func (s *NetguardService) GetAddressGroupBindings(ctx context.Context, scope ports.Scope) ([]models.AddressGroupBinding, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	var bindings []models.AddressGroupBinding
	err = reader.ListAddressGroupBindings(ctx, func(binding models.AddressGroupBinding) error {
		bindings = append(bindings, binding)
		return nil
	}, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list address group bindings")
	}
	return bindings, nil
}

// GetAddressGroupPortMappings returns all address group port mappings
func (s *NetguardService) GetAddressGroupPortMappings(ctx context.Context, scope ports.Scope) ([]models.AddressGroupPortMapping, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	var mappings []models.AddressGroupPortMapping
	err = reader.ListAddressGroupPortMappings(ctx, func(mapping models.AddressGroupPortMapping) error {
		mappings = append(mappings, mapping)
		return nil
	}, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list address group port mappings")
	}
	return mappings, nil
}

// GetRuleS2S returns all rule s2s
func (s *NetguardService) GetRuleS2S(ctx context.Context, scope ports.Scope) ([]models.RuleS2S, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	var rules []models.RuleS2S
	err = reader.ListRuleS2S(ctx, func(rule models.RuleS2S) error {
		rules = append(rules, rule)
		return nil
	}, scope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list rule s2s")
	}
	return rules, nil
}

// SyncServices syncs services
func (s *NetguardService) SyncServices(ctx context.Context, services []models.Service, scope ports.Scope) error {
	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	if err = writer.SyncServices(ctx, services, scope); err != nil {
		return errors.Wrap(err, "failed to sync services")
	}
	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit")
	}
	return nil
}

// SyncAddressGroups syncs address groups
func (s *NetguardService) SyncAddressGroups(ctx context.Context, addressGroups []models.AddressGroup, scope ports.Scope) error {
	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	if err = writer.SyncAddressGroups(ctx, addressGroups, scope); err != nil {
		return errors.Wrap(err, "failed to sync address groups")
	}
	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit")
	}
	return nil
}

// SyncAddressGroupBindings syncs address group bindings
func (s *NetguardService) SyncAddressGroupBindings(ctx context.Context, bindings []models.AddressGroupBinding, scope ports.Scope) error {
	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	if err = writer.SyncAddressGroupBindings(ctx, bindings, scope); err != nil {
		return errors.Wrap(err, "failed to sync address group bindings")
	}
	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit")
	}
	return nil
}

// SyncAddressGroupPortMappings syncs address group port mappings
func (s *NetguardService) SyncAddressGroupPortMappings(ctx context.Context, mappings []models.AddressGroupPortMapping, scope ports.Scope) error {
	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	if err = writer.SyncAddressGroupPortMappings(ctx, mappings, scope); err != nil {
		return errors.Wrap(err, "failed to sync address group port mappings")
	}
	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit")
	}
	return nil
}

// SyncRuleS2S syncs rule s2s
func (s *NetguardService) SyncRuleS2S(ctx context.Context, rules []models.RuleS2S, scope ports.Scope) error {
	writer, err := s.registry.Writer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}
	defer func() {
		if err != nil {
			writer.Abort()
		}
	}()

	if err = writer.SyncRuleS2S(ctx, rules, scope); err != nil {
		return errors.Wrap(err, "failed to sync rule s2s")
	}
	if err = writer.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit")
	}
	return nil
}

// GetSyncStatus returns the status of the last synchronization
func (s *NetguardService) GetSyncStatus(ctx context.Context) (*models.SyncStatus, error) {
	reader, err := s.registry.Reader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}
	defer reader.Close()

	return reader.GetSyncStatus(ctx)
}
