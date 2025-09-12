package models

import (
	"fmt"

	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/sync/types"
)

// HostBinding represents a host binding resource in the domain
type HostBinding struct {
	SelfRef

	// Specification from HostBindingSpec
	HostRef         v1beta1.NamespacedObjectReference `json:"hostRef"`
	AddressGroupRef v1beta1.NamespacedObjectReference `json:"addressGroupRef"`

	// Metadata
	Meta Meta `json:"meta"`
}

// NewHostBinding creates a new HostBinding with default values
func NewHostBinding(name, namespace string, hostRef, addressGroupRef v1beta1.NamespacedObjectReference) *HostBinding {
	return &HostBinding{
		SelfRef: SelfRef{
			ResourceIdentifier: ResourceIdentifier{
				Name:      name,
				Namespace: namespace,
			},
		},
		HostRef:         hostRef,
		AddressGroupRef: addressGroupRef,
		Meta:            Meta{},
	}
}

// GetName returns the name of the host binding
func (hb *HostBinding) GetName() string {
	return hb.Name
}

// GetNamespace returns the namespace of the host binding
func (hb *HostBinding) GetNamespace() string {
	return hb.Namespace
}

// GetMeta returns the metadata
func (hb *HostBinding) GetMeta() *Meta {
	return &hb.Meta
}

// IsReady returns true if the host binding is ready
func (hb *HostBinding) IsReady() bool {
	return hb.Meta.IsReady()
}

// IsValidated returns true if the host binding is validated
func (hb *HostBinding) IsValidated() bool {
	return hb.Meta.IsValidated()
}

// IsSynced returns true if the host binding is synced
func (hb *HostBinding) IsSynced() bool {
	return hb.Meta.IsSynced()
}

// Key returns the unique key for the host binding (namespace/name)
func (hb *HostBinding) Key() string {
	if hb.Namespace == "" {
		return hb.Name
	}
	return fmt.Sprintf("%s/%s", hb.Namespace, hb.Name)
}

// GetID returns the unique identifier for the host binding
func (hb *HostBinding) GetID() string {
	return hb.Key()
}

// GetGeneration returns the generation of the host binding
func (hb *HostBinding) GetGeneration() int64 {
	return hb.Meta.Generation
}

// DeepCopy creates a deep copy of the host binding
func (hb *HostBinding) DeepCopy() Resource {
	if hb == nil {
		return nil
	}

	copy := *hb
	copy.Meta = hb.Meta // Meta is a struct, so this is a shallow copy

	// Deep copy object references
	copy.HostRef = hb.HostRef
	copy.AddressGroupRef = hb.AddressGroupRef

	return &copy
}

// SyncableEntity interface implementation for HostBinding

// GetSyncSubjectType returns the sync subject type for HostBinding
func (hb *HostBinding) GetSyncSubjectType() types.SyncSubjectType {
	return "HostBindings" // Host bindings are now their own sync subject type
}

// GetSyncKey returns a unique key for the HostBinding
func (hb *HostBinding) GetSyncKey() string {
	if hb.Namespace != "" {
		return fmt.Sprintf("hostbinding-%s/%s", hb.Namespace, hb.Name)
	}
	return fmt.Sprintf("hostbinding-%s", hb.Name)
}

// GetHostReference returns the host reference
func (hb *HostBinding) GetHostReference() v1beta1.NamespacedObjectReference {
	return hb.HostRef
}

// GetAddressGroupReference returns the address group reference
func (hb *HostBinding) GetAddressGroupReference() v1beta1.NamespacedObjectReference {
	return hb.AddressGroupRef
}

// ToSGroupsProto converts the HostBinding to sgroups protobuf format
func (hb *HostBinding) ToSGroupsProto() (interface{}, error) {
	if hb == nil {
		return nil, fmt.Errorf("HostBinding cannot be nil")
	}

	// Build host reference name
	hostRefName := hb.HostRef.Name
	if hb.HostRef.Namespace != "" {
		hostRefName = fmt.Sprintf("%s/%s", hb.HostRef.Namespace, hb.HostRef.Name)
	}

	// Build address group reference name
	agRefName := hb.AddressGroupRef.Name
	if hb.AddressGroupRef.Namespace != "" {
		agRefName = fmt.Sprintf("%s/%s", hb.AddressGroupRef.Namespace, hb.AddressGroupRef.Name)
	}

	protoBinding := map[string]interface{}{
		"name":            hb.Name,
		"namespace":       hb.Namespace,
		"hostRef":         hostRefName,
		"addressGroupRef": agRefName,
	}

	return protoBinding, nil
}
