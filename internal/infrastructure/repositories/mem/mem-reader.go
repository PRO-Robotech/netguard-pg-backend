package mem

import (
	"context"
	"log"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

type reader struct {
	registry *Registry
	ctx      context.Context
	writer   *writer // If not nil, use data from this writer instead of registry.db
}

func (r *reader) Close() error {
	return nil
}

func (r *reader) ListServices(ctx context.Context, consume func(models.Service) error, scope ports.Scope) error {
	var services map[string]models.Service
	var bindings map[string]models.AddressGroupBinding

	// Use data from writer if available
	if r.writer != nil && r.writer.services != nil {
		services = r.writer.services
		log.Printf("📋 LIST: using writer data with %d services", len(services))
	} else {
		services = r.registry.db.GetServices()
		log.Printf("📋 LIST: using registry db data with %d services", len(services))
	}

	// Use data from writer if available
	if r.writer != nil && r.writer.addressGroupBindings != nil {
		bindings = r.writer.addressGroupBindings
	} else {
		bindings = r.registry.db.GetAddressGroupBindings()
	}

	// 🔍 КРАТКАЯ ДИАГНОСТИКА: проверяем только общее количество conditions
	totalConditions := 0
	for key, service := range services {
		condCount := len(service.Meta.Conditions)
		totalConditions += condCount
		log.Printf("🔍 LIST_DB_BRIEF: Service[%s] has %d conditions", key, condCount)
	}
	log.Printf("🔍 LIST_TOTAL: Found %d services with total %d conditions", len(services), totalConditions)

	if scope != nil && !scope.IsEmpty() {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
			for _, id := range ris.Identifiers {
				// If only namespace is set, return all services in that namespace
				if id.Name == "" && id.Namespace != "" {
					for _, service := range services {
						if service.Namespace == id.Namespace {
							log.Printf("🔍 COPY_TEST: Before copy - %s has %d conditions", service.Key(), len(service.Meta.Conditions))

							// Create a copy of the service to avoid modifying the original
							serviceCopy := service

							log.Printf("🔍 COPY_TEST: After copy - %s has %d conditions", serviceCopy.Key(), len(serviceCopy.Meta.Conditions))

							// Clear the address groups to avoid duplicates
							serviceCopy.AddressGroups = []models.AddressGroupRef{}

							// Populate address groups from bindings
							for _, binding := range bindings {
								if binding.ServiceRef.Key() == serviceCopy.Key() {
									serviceCopy.AddressGroups = append(serviceCopy.AddressGroups, binding.AddressGroupRef)
								}
							}

							log.Printf("🔍 CONSUME_TEST: Before consume - %s has %d conditions", serviceCopy.Key(), len(serviceCopy.Meta.Conditions))

							if err := consume(serviceCopy); err != nil {
								return err
							}

							log.Printf("✅ CONSUME_OK: After consume - %s processed", serviceCopy.Key())
						}
					}
					return nil
				}

				// Otherwise, look for the service by exact key
				if service, ok := services[id.Key()]; ok {
					log.Printf("🔍 EXACT_COPY: Service %s has %d conditions", service.Key(), len(service.Meta.Conditions))

					// Create a copy of the service to avoid modifying the original
					serviceCopy := service

					// Clear the address groups to avoid duplicates
					serviceCopy.AddressGroups = []models.AddressGroupRef{}

					// Populate address groups from bindings
					for _, binding := range bindings {
						if binding.ServiceRef.Key() == serviceCopy.Key() {
							serviceCopy.AddressGroups = append(serviceCopy.AddressGroups, binding.AddressGroupRef)
						}
					}

					if err := consume(serviceCopy); err != nil {
						return err
					}
				}
			}
			return nil
		}
	}
	for _, service := range services {
		log.Printf("🔍 ALL_COPY: Service %s has %d conditions", service.Key(), len(service.Meta.Conditions))

		// Create a copy of the service to avoid modifying the original
		serviceCopy := service

		// Clear the address groups to avoid duplicates
		serviceCopy.AddressGroups = []models.AddressGroupRef{}

		// Populate address groups from bindings
		for _, binding := range bindings {
			if binding.ServiceRef.Key() == serviceCopy.Key() {
				serviceCopy.AddressGroups = append(serviceCopy.AddressGroups, binding.AddressGroupRef)
			}
		}

		log.Printf("🔍 ALL_CONSUME: Before consume - %s has %d conditions", serviceCopy.Key(), len(serviceCopy.Meta.Conditions))

		if err := consume(serviceCopy); err != nil {
			return err
		}

		log.Printf("✅ ALL_OK: Service %s processed successfully", serviceCopy.Key())
	}
	return nil
}

func (r *reader) ListAddressGroups(ctx context.Context, consume func(models.AddressGroup) error, scope ports.Scope) error {
	var addressGroups map[string]models.AddressGroup

	// Use data from writer if available
	if r.writer != nil && r.writer.addressGroups != nil {
		addressGroups = r.writer.addressGroups
	} else {
		addressGroups = r.registry.db.GetAddressGroups()
	}
	if scope != nil && !scope.IsEmpty() {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
			for _, id := range ris.Identifiers {
				// If only namespace is set, return all address groups in that namespace
				if id.Name == "" && id.Namespace != "" {
					for _, addressGroup := range addressGroups {
						if addressGroup.Namespace == id.Namespace {
							if err := consume(addressGroup); err != nil {
								return err
							}
						}
					}
					return nil
				}

				// Otherwise, look for the address group by exact key
				if addressGroup, ok := addressGroups[id.Key()]; ok {
					if err := consume(addressGroup); err != nil {
						return err
					}
				}
			}
			return nil
		}
	}
	for _, addressGroup := range addressGroups {
		if err := consume(addressGroup); err != nil {
			return err
		}
	}
	return nil
}

func (r *reader) ListAddressGroupBindings(ctx context.Context, consume func(models.AddressGroupBinding) error, scope ports.Scope) error {
	var bindings map[string]models.AddressGroupBinding

	// Use data from writer if available
	if r.writer != nil && r.writer.addressGroupBindings != nil {
		bindings = r.writer.addressGroupBindings
	} else {
		bindings = r.registry.db.GetAddressGroupBindings()
	}
	if scope != nil && !scope.IsEmpty() {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
			for _, id := range ris.Identifiers {
				// If only namespace is set, return all bindings in that namespace
				if id.Name == "" && id.Namespace != "" {
					for _, binding := range bindings {
						if binding.Namespace == id.Namespace {
							if err := consume(binding); err != nil {
								return err
							}
						}
					}
					return nil
				}

				// Otherwise, look for the binding by exact key
				if binding, ok := bindings[id.Key()]; ok {
					if err := consume(binding); err != nil {
						return err
					}
				}
			}
			return nil
		}
	}
	for _, binding := range bindings {
		if err := consume(binding); err != nil {
			return err
		}
	}
	return nil
}

func (r *reader) ListAddressGroupPortMappings(ctx context.Context, consume func(models.AddressGroupPortMapping) error, scope ports.Scope) error {
	var mappings map[string]models.AddressGroupPortMapping

	// Use data from writer if available
	if r.writer != nil && r.writer.addressGroupPortMappings != nil {
		mappings = r.writer.addressGroupPortMappings
	} else {
		mappings = r.registry.db.GetAddressGroupPortMappings()
	}
	if scope != nil && !scope.IsEmpty() {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
			for _, id := range ris.Identifiers {
				// If only namespace is set, return all port mappings in that namespace
				if id.Name == "" && id.Namespace != "" {
					for _, mapping := range mappings {
						if mapping.Namespace == id.Namespace {
							if err := consume(mapping); err != nil {
								return err
							}
						}
					}
					return nil
				}

				// Otherwise, look for the port mapping by exact key
				if mapping, ok := mappings[id.Key()]; ok {
					if err := consume(mapping); err != nil {
						return err
					}
				}
			}
			return nil
		}
	}
	for _, mapping := range mappings {
		if err := consume(mapping); err != nil {
			return err
		}
	}
	return nil
}

func (r *reader) ListRuleS2S(ctx context.Context, consume func(models.RuleS2S) error, scope ports.Scope) error {
	var rules map[string]models.RuleS2S

	// Use data from writer if available
	if r.writer != nil && r.writer.ruleS2S != nil {
		rules = r.writer.ruleS2S
	} else {
		rules = r.registry.db.GetRuleS2S()
	}
	if scope != nil && !scope.IsEmpty() {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
			for _, id := range ris.Identifiers {
				// If only namespace is set, return all rules in that namespace
				if id.Name == "" && id.Namespace != "" {
					for _, rule := range rules {
						if rule.Namespace == id.Namespace {
							if err := consume(rule); err != nil {
								return err
							}
						}
					}
					return nil
				}

				// Otherwise, look for the rule by exact key
				if rule, ok := rules[id.Key()]; ok {
					if err := consume(rule); err != nil {
						return err
					}
				}
			}
			return nil
		}
	}
	for _, rule := range rules {
		if err := consume(rule); err != nil {
			return err
		}
	}
	return nil
}

func (r *reader) ListServiceAliases(ctx context.Context, consume func(models.ServiceAlias) error, scope ports.Scope) error {
	var aliases map[string]models.ServiceAlias

	// Use data from writer if available
	if r.writer != nil && r.writer.serviceAliases != nil {
		aliases = r.writer.serviceAliases
	} else {
		aliases = r.registry.db.GetServiceAliases()
	}
	if scope != nil && !scope.IsEmpty() {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
			for _, id := range ris.Identifiers {
				// If only namespace is set, return all aliases in that namespace
				if id.Name == "" && id.Namespace != "" {
					for _, alias := range aliases {
						if alias.Namespace == id.Namespace {
							if err := consume(alias); err != nil {
								return err
							}
						}
					}
					return nil
				}

				// Otherwise, look for the alias by exact key
				if alias, ok := aliases[id.Key()]; ok {
					if err := consume(alias); err != nil {
						return err
					}
				}
			}
			return nil
		}
	}
	for _, alias := range aliases {
		if err := consume(alias); err != nil {
			return err
		}
	}
	return nil
}

func (r *reader) GetSyncStatus(ctx context.Context) (*models.SyncStatus, error) {
	status := r.registry.db.GetSyncStatus()
	return &status, nil
}

// GetServiceByID gets a service by ID
func (r *reader) GetServiceByID(ctx context.Context, id models.ResourceIdentifier) (*models.Service, error) {
	log.Printf("GetServiceByID: looking for ns=%q name=%q key=%s", id.Namespace, id.Name, id.Key())

	var services map[string]models.Service
	var bindings map[string]models.AddressGroupBinding

	// Use data from writer if available
	if r.writer != nil && r.writer.services != nil {
		services = r.writer.services
		log.Printf("GetServiceByID: using writer data")
	} else {
		services = r.registry.db.GetServices()
		log.Printf("GetServiceByID: using registry db data")
	}

	log.Printf("GetServiceByID: services map size=%d", len(services))
	for k := range services {
		if k == id.Key() {
			log.Printf("GetServiceByID: found matching key in map: %s", k)
		}
	}

	if service, ok := services[id.Key()]; ok {
		log.Printf("GetServiceByID: EXACT match found for key=%s meta.uid=%s", id.Key(), service.Meta.UID)

		// 🔍 ДИАГНОСТИКА: логируем что читаем из БД
		log.Printf("🔍 READER: Service from DB %s has %d conditions", service.Key(), len(service.Meta.Conditions))
		for i, cond := range service.Meta.Conditions {
			log.Printf("  🔍 READER: db[%d] Type=%s Status=%s Reason=%s", i, cond.Type, cond.Status, cond.Reason)
		}

		// Create a copy of the service to avoid modifying the original
		serviceCopy := service

		// 🔍 ДИАГНОСТИКА: проверяем что скопировано
		log.Printf("🔍 READER: After copy %s has %d conditions", serviceCopy.Key(), len(serviceCopy.Meta.Conditions))
		for i, cond := range serviceCopy.Meta.Conditions {
			log.Printf("  🔍 READER: copy[%d] Type=%s Status=%s Reason=%s", i, cond.Type, cond.Status, cond.Reason)
		}

		// Clear the address groups to avoid duplicates
		serviceCopy.AddressGroups = []models.AddressGroupRef{}

		// Get all bindings to populate the address groups
		if r.writer != nil && r.writer.addressGroupBindings != nil {
			bindings = r.writer.addressGroupBindings
		} else {
			bindings = r.registry.db.GetAddressGroupBindings()
		}
		for _, binding := range bindings {
			// Check if the binding is for this service
			if binding.ServiceRef.Key() == id.Key() {
				// Add the address group to the service
				serviceCopy.AddressGroups = append(serviceCopy.AddressGroups, binding.AddressGroupRef)
			}
		}

		// 🔍 ДИАГНОСТИКА: финальная проверка перед возвратом
		log.Printf("✅ READER: RETURNING service %s with %d conditions", serviceCopy.Key(), len(serviceCopy.Meta.Conditions))
		for i, cond := range serviceCopy.Meta.Conditions {
			log.Printf("  ✅ READER: return[%d] Type=%s Status=%s Reason=%s", i, cond.Type, cond.Status, cond.Reason)
		}

		return &serviceCopy, nil
	}
	log.Printf("GetServiceByID: NOT FOUND key=%s", id.Key())
	return nil, ports.ErrNotFound
}

// GetAddressGroupByID gets an address group by ID
func (r *reader) GetAddressGroupByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroup, error) {
	var addressGroups map[string]models.AddressGroup

	// Use data from writer if available
	if r.writer != nil && r.writer.addressGroups != nil {
		addressGroups = r.writer.addressGroups
	} else {
		addressGroups = r.registry.db.GetAddressGroups()
	}

	if addressGroup, ok := addressGroups[id.Key()]; ok {
		return &addressGroup, nil
	}
	return nil, ports.ErrNotFound
}

// GetAddressGroupBindingByID gets an address group binding by ID
func (r *reader) GetAddressGroupBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBinding, error) {
	var bindings map[string]models.AddressGroupBinding

	// Use data from writer if available
	if r.writer != nil && r.writer.addressGroupBindings != nil {
		bindings = r.writer.addressGroupBindings
	} else {
		bindings = r.registry.db.GetAddressGroupBindings()
	}

	if binding, ok := bindings[id.Key()]; ok {
		return &binding, nil
	}
	return nil, ports.ErrNotFound
}

// GetAddressGroupPortMappingByID gets an address group port mapping by ID
func (r *reader) GetAddressGroupPortMappingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupPortMapping, error) {
	var mappings map[string]models.AddressGroupPortMapping

	// Use data from writer if available
	if r.writer != nil && r.writer.addressGroupPortMappings != nil {
		mappings = r.writer.addressGroupPortMappings
	} else {
		mappings = r.registry.db.GetAddressGroupPortMappings()
	}

	if mapping, ok := mappings[id.Key()]; ok {
		return &mapping, nil
	}
	return nil, ports.ErrNotFound
}

// GetRuleS2SByID gets a rule s2s by ID
func (r *reader) GetRuleS2SByID(ctx context.Context, id models.ResourceIdentifier) (*models.RuleS2S, error) {
	var rules map[string]models.RuleS2S

	// Use data from writer if available
	if r.writer != nil && r.writer.ruleS2S != nil {
		rules = r.writer.ruleS2S
	} else {
		rules = r.registry.db.GetRuleS2S()
	}

	if rule, ok := rules[id.Key()]; ok {
		return &rule, nil
	}
	return nil, ports.ErrNotFound
}

// GetServiceAliasByID gets a service alias by ID
func (r *reader) GetServiceAliasByID(ctx context.Context, id models.ResourceIdentifier) (*models.ServiceAlias, error) {
	var aliases map[string]models.ServiceAlias

	// Use data from writer if available
	if r.writer != nil && r.writer.serviceAliases != nil {
		aliases = r.writer.serviceAliases
	} else {
		aliases = r.registry.db.GetServiceAliases()
	}

	if alias, ok := aliases[id.Key()]; ok {
		return &alias, nil
	}
	return nil, ports.ErrNotFound
}

func (r *reader) ListAddressGroupBindingPolicies(ctx context.Context, consume func(models.AddressGroupBindingPolicy) error, scope ports.Scope) error {
	var policies map[string]models.AddressGroupBindingPolicy

	// Use data from writer if available
	if r.writer != nil && r.writer.addressGroupBindingPolicies != nil {
		policies = r.writer.addressGroupBindingPolicies
	} else {
		policies = r.registry.db.GetAddressGroupBindingPolicies()
	}
	if scope != nil && !scope.IsEmpty() {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
			for _, id := range ris.Identifiers {
				// Если установлен только namespace, возвращаем все политики в этом namespace
				if id.Name == "" && id.Namespace != "" {
					for _, policy := range policies {
						if policy.Namespace == id.Namespace {
							if err := consume(policy); err != nil {
								return err
							}
						}
					}
					return nil
				}

				// Иначе ищем политику по точному ключу
				if policy, ok := policies[id.Key()]; ok {
					if err := consume(policy); err != nil {
						return err
					}
				}
			}
			return nil
		}
	}
	for _, policy := range policies {
		if err := consume(policy); err != nil {
			return err
		}
	}
	return nil
}

func (r *reader) GetAddressGroupBindingPolicyByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBindingPolicy, error) {
	var policies map[string]models.AddressGroupBindingPolicy

	// Use data from writer if available
	if r.writer != nil && r.writer.addressGroupBindingPolicies != nil {
		policies = r.writer.addressGroupBindingPolicies
	} else {
		policies = r.registry.db.GetAddressGroupBindingPolicies()
	}

	if policy, ok := policies[id.Key()]; ok {
		return &policy, nil
	}
	return nil, ports.ErrNotFound
}

func (r *reader) ListIEAgAgRules(ctx context.Context, consume func(models.IEAgAgRule) error, scope ports.Scope) error {
	var rules map[string]models.IEAgAgRule

	// Use data from writer if available
	if r.writer != nil && r.writer.ieAgAgRules != nil {
		rules = r.writer.ieAgAgRules
	} else {
		rules = r.registry.db.GetIEAgAgRules()
	}
	if scope != nil && !scope.IsEmpty() {
		if ris, ok := scope.(ports.ResourceIdentifierScope); ok && !ris.IsEmpty() {
			for _, id := range ris.Identifiers {
				// If only namespace is set, return all rules in that namespace
				if id.Name == "" && id.Namespace != "" {
					for _, rule := range rules {
						if rule.Namespace == id.Namespace {
							if err := consume(rule); err != nil {
								return err
							}
						}
					}
					return nil
				}

				// Otherwise, look for the rule by exact key
				if rule, ok := rules[id.Key()]; ok {
					if err := consume(rule); err != nil {
						return err
					}
				}
			}
			return nil
		}
	}
	for _, rule := range rules {
		if err := consume(rule); err != nil {
			return err
		}
	}
	return nil
}

func (r *reader) GetIEAgAgRuleByID(ctx context.Context, id models.ResourceIdentifier) (*models.IEAgAgRule, error) {
	var rules map[string]models.IEAgAgRule

	// Use data from writer if available
	if r.writer != nil && r.writer.ieAgAgRules != nil {
		rules = r.writer.ieAgAgRules
	} else {
		rules = r.registry.db.GetIEAgAgRules()
	}

	if rule, ok := rules[id.Key()]; ok {
		return &rule, nil
	}
	return nil, ports.ErrNotFound
}
