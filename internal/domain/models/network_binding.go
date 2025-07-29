package models

import (
	"fmt"

	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/sync/types"
)

// NetworkBinding represents a network binding resource in the domain
type NetworkBinding struct {
	SelfRef

	// Specification
	NetworkRef      v1beta1.ObjectReference `json:"networkRef"`
	AddressGroupRef v1beta1.ObjectReference `json:"addressGroupRef"`

	// NetworkItem contains the network information
	NetworkItem NetworkItem `json:"networkItem"`

	// Metadata
	Meta Meta `json:"meta"`
}

// NewNetworkBinding creates a new NetworkBinding with default values
func NewNetworkBinding(name, namespace string, networkRef, addressGroupRef v1beta1.ObjectReference) *NetworkBinding {
	return &NetworkBinding{
		SelfRef: SelfRef{
			ResourceIdentifier: ResourceIdentifier{
				Name:      name,
				Namespace: namespace,
			},
		},
		NetworkRef:      networkRef,
		AddressGroupRef: addressGroupRef,
		Meta:            Meta{},
	}
}

// GetName returns the name of the network binding
func (nb *NetworkBinding) GetName() string {
	return nb.Name
}

// GetNamespace returns the namespace of the network binding
func (nb *NetworkBinding) GetNamespace() string {
	return nb.Namespace
}

// GetMeta returns the metadata
func (nb *NetworkBinding) GetMeta() *Meta {
	return &nb.Meta
}

// SetNetworkItem sets the network item information
func (nb *NetworkBinding) SetNetworkItem(item NetworkItem) {
	nb.NetworkItem = item
}

// IsReady returns true if the network binding is ready
func (nb *NetworkBinding) IsReady() bool {
	return nb.Meta.IsReady()
}

// IsValidated returns true if the network binding is validated
func (nb *NetworkBinding) IsValidated() bool {
	return nb.Meta.IsValidated()
}

// IsSynced returns true if the network binding is synced
func (nb *NetworkBinding) IsSynced() bool {
	return nb.Meta.IsSynced()
}

// Key returns the unique key for the network binding (namespace/name)
func (nb *NetworkBinding) Key() string {
	if nb.Namespace == "" {
		return nb.Name
	}
	return fmt.Sprintf("%s/%s", nb.Namespace, nb.Name)
}

// GetID returns the unique identifier for the network binding
func (nb *NetworkBinding) GetID() string {
	return nb.Key()
}

// GetGeneration returns the generation of the network binding
func (nb *NetworkBinding) GetGeneration() int64 {
	return nb.Meta.Generation
}

// DeepCopy creates a deep copy of the network binding
func (nb *NetworkBinding) DeepCopy() Resource {
	if nb == nil {
		return nil
	}

	copy := *nb
	copy.Meta = nb.Meta // Meta is a struct, so this is a shallow copy

	// Deep copy slices and maps if needed
	copy.NetworkRef = nb.NetworkRef
	copy.AddressGroupRef = nb.AddressGroupRef
	copy.NetworkItem = nb.NetworkItem

	return &copy
}

// SyncableEntity interface implementation for NetworkBinding

// GetSyncSubjectType returns the sync subject type for NetworkBinding
func (nb *NetworkBinding) GetSyncSubjectType() types.SyncSubjectType {
	return types.SyncSubjectTypeNetworkBindings
}

// GetSyncKey returns a unique key for the NetworkBinding
func (nb *NetworkBinding) GetSyncKey() string {
	if nb.Namespace != "" {
		return fmt.Sprintf("networkbinding-%s/%s", nb.Namespace, nb.Name)
	}
	return fmt.Sprintf("networkbinding-%s", nb.Name)
}

// ToSGroupsProto converts the NetworkBinding to sgroups protobuf format
func (nb *NetworkBinding) ToSGroupsProto() (interface{}, error) {
	if nb == nil {
		return nil, fmt.Errorf("NetworkBinding cannot be nil")
	}

	protoBinding := map[string]interface{}{
		"name":            nb.Name,
		"namespace":       nb.Namespace,
		"networkRef":      nb.NetworkRef,
		"addressGroupRef": nb.AddressGroupRef,
		"networkItem":     nb.NetworkItem,
	}

	return protoBinding, nil
}
