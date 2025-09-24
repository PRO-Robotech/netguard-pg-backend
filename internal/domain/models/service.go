package models

import (
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

// Service represents a service with its ports
type Service struct {
	SelfRef
	Description   string
	IngressPorts  []IngressPort
	AddressGroups []AddressGroupRef

	// XAggregatedAddressGroups contains all AddressGroup references from both spec and bindings
	// aggregated from both spec.addressGroups and AddressGroupBinding resources
	XAggregatedAddressGroups []AddressGroupReference

	Meta Meta
}

// ServiceRef represents a reference to a Service
type ServiceRef = netguardv1beta1.NamespacedObjectReference

// ServiceRefKey generates a key from ServiceRef for maps
func ServiceRefKey(ref ServiceRef) string {
	return ref.Namespace + "/" + ref.Name
}

// NewServiceRef creates a new ServiceRef
func NewServiceRef(name string, opts ...ResourceIdentifierOption) ServiceRef {
	id := NewResourceIdentifier(name, opts...)
	return netguardv1beta1.NamespacedObjectReference{
		ObjectReference: netguardv1beta1.ObjectReference{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "Service",
			Name:       id.Name,
		},
		Namespace: id.Namespace,
	}
}

type AddressGroupReference struct {
	// Reference to the AddressGroup object
	Ref netguardv1beta1.NamespacedObjectReference `json:"ref"`

	// Source indicates how this address group was registered (spec or binding)
	Source netguardv1beta1.AddressGroupRegistrationSource `json:"source"`
}

// NewAddressGroupReference creates a new AddressGroupReference from a NamespacedObjectReference
func NewAddressGroupReference(ref netguardv1beta1.NamespacedObjectReference, source netguardv1beta1.AddressGroupRegistrationSource) AddressGroupReference {
	return AddressGroupReference{
		Ref:    ref,
		Source: source,
	}
}

// NewAddressGroupReferenceFromRef creates a new AddressGroupReference from an AddressGroupRef
func NewAddressGroupReferenceFromRef(ref AddressGroupRef, source netguardv1beta1.AddressGroupRegistrationSource) AddressGroupReference {
	return AddressGroupReference{
		Ref:    ref,
		Source: source,
	}
}

// Key returns the unique key for the AddressGroupReference
func (agr *AddressGroupReference) Key() string {
	if agr.Ref.Namespace == "" {
		return agr.Ref.Name
	}
	return agr.Ref.Namespace + "/" + agr.Ref.Name
}

// GetName returns the name of the referenced AddressGroup
func (agr *AddressGroupReference) GetName() string {
	return agr.Ref.Name
}

// GetNamespace returns the namespace of the referenced AddressGroup
func (agr *AddressGroupReference) GetNamespace() string {
	return agr.Ref.Namespace
}

// IsFromSpec returns true if this reference came from Service.spec.addressGroups
func (agr *AddressGroupReference) IsFromSpec() bool {
	return agr.Source == netguardv1beta1.AddressGroupSourceSpec
}

// IsFromBinding returns true if this reference came from AddressGroupBinding
func (agr *AddressGroupReference) IsFromBinding() bool {
	return agr.Source == netguardv1beta1.AddressGroupSourceBinding
}

// Service helper methods for dual registration functionality

// GetAggregatedAddressGroups returns all aggregated AddressGroup references
func (s *Service) GetAggregatedAddressGroups() []AddressGroupReference {
	return s.XAggregatedAddressGroups
}

// GetSpecAddressGroups returns AddressGroups from spec only (filtered from aggregated)
func (s *Service) GetSpecAddressGroups() []AddressGroupReference {
	var specGroups []AddressGroupReference
	for _, ref := range s.XAggregatedAddressGroups {
		if ref.IsFromSpec() {
			specGroups = append(specGroups, ref)
		}
	}
	return specGroups
}

// GetBindingAddressGroups returns AddressGroups from bindings only (filtered from aggregated)
func (s *Service) GetBindingAddressGroups() []AddressGroupReference {
	var bindingGroups []AddressGroupReference
	for _, ref := range s.XAggregatedAddressGroups {
		if ref.IsFromBinding() {
			bindingGroups = append(bindingGroups, ref)
		}
	}
	return bindingGroups
}

// HasAddressGroup checks if the service has a specific AddressGroup reference
func (s *Service) HasAddressGroup(namespace, name string) bool {
	key := namespace + "/" + name
	if namespace == "" {
		key = name
	}

	for _, ref := range s.XAggregatedAddressGroups {
		if ref.Key() == key {
			return true
		}
	}
	return false
}

// HasAddressGroupFromSource checks if the service has a specific AddressGroup from a specific source
func (s *Service) HasAddressGroupFromSource(namespace, name string, source netguardv1beta1.AddressGroupRegistrationSource) bool {
	key := namespace + "/" + name
	if namespace == "" {
		key = name
	}

	for _, ref := range s.XAggregatedAddressGroups {
		if ref.Key() == key && ref.Source == source {
			return true
		}
	}
	return false
}

// HasConflicts returns true if there are any dual registration conflicts
// (same AddressGroup referenced from both spec and binding)
func (s *Service) HasConflicts() bool {
	// Create a map to track AddressGroups by their key
	groupSources := make(map[string][]netguardv1beta1.AddressGroupRegistrationSource)

	for _, ref := range s.XAggregatedAddressGroups {
		key := ref.Key()
		groupSources[key] = append(groupSources[key], ref.Source)
	}

	// Check if any AddressGroup has references from multiple sources
	for _, sources := range groupSources {
		if len(sources) > 1 {
			// Check if we have both spec and binding sources
			hasSpec := false
			hasBinding := false
			for _, source := range sources {
				if source == netguardv1beta1.AddressGroupSourceSpec {
					hasSpec = true
				}
				if source == netguardv1beta1.AddressGroupSourceBinding {
					hasBinding = true
				}
			}
			if hasSpec && hasBinding {
				return true
			}
		}
	}

	return false
}

// GetConflictingAddressGroups returns a list of AddressGroups that have dual registration conflicts
func (s *Service) GetConflictingAddressGroups() []string {
	// Create a map to track AddressGroups by their key
	groupSources := make(map[string][]netguardv1beta1.AddressGroupRegistrationSource)

	for _, ref := range s.XAggregatedAddressGroups {
		key := ref.Key()
		groupSources[key] = append(groupSources[key], ref.Source)
	}

	var conflicts []string
	// Check if any AddressGroup has references from multiple sources
	for key, sources := range groupSources {
		if len(sources) > 1 {
			// Check if we have both spec and binding sources
			hasSpec := false
			hasBinding := false
			for _, source := range sources {
				if source == netguardv1beta1.AddressGroupSourceSpec {
					hasSpec = true
				}
				if source == netguardv1beta1.AddressGroupSourceBinding {
					hasBinding = true
				}
			}
			if hasSpec && hasBinding {
				conflicts = append(conflicts, key)
			}
		}
	}

	return conflicts
}

// ValidateReferences validates all AddressGroup references for consistency and conflicts
func (s *Service) ValidateReferences() error {
	if s.HasConflicts() {
		conflicts := s.GetConflictingAddressGroups()
		return &DualRegistrationConflictError{
			ServiceKey:        s.Namespace + "/" + s.Name,
			ConflictingGroups: conflicts,
		}
	}
	return nil
}

// DualRegistrationConflictError represents an error when the same AddressGroup
// is referenced from both spec and binding
type DualRegistrationConflictError struct {
	ServiceKey        string
	ConflictingGroups []string
}

func (e *DualRegistrationConflictError) Error() string {
	return "dual registration conflict: AddressGroups " +
		joinStrings(e.ConflictingGroups, ", ") +
		" are referenced by Service " + e.ServiceKey +
		" via both spec.addressGroups and AddressGroupBinding"
}

// Helper function to join strings (avoiding external dependencies)
func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	if len(strs) == 1 {
		return strs[0]
	}

	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}
