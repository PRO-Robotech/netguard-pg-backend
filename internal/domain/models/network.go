package models

import (
	"fmt"

	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/sync/types"
)

// Network represents a network resource in the domain
type Network struct {
	SelfRef

	// Specification
	CIDR string `json:"cidr"`

	// Status
	NetworkName     string                   `json:"networkName,omitempty"`
	IsBound         bool                     `json:"isBound"`
	BindingRef      *v1beta1.ObjectReference `json:"bindingRef,omitempty"`
	AddressGroupRef *v1beta1.ObjectReference `json:"addressGroupRef,omitempty"`

	// Metadata
	Meta Meta `json:"meta"`
}

// NewNetwork creates a new Network with default values
func NewNetwork(name, namespace, cidr string) *Network {
	return &Network{
		SelfRef: SelfRef{
			ResourceIdentifier: ResourceIdentifier{
				Name:      name,
				Namespace: namespace,
			},
		},
		CIDR:    cidr,
		IsBound: false,
		Meta:    Meta{},
	}
}

// GetName returns the name of the network
func (n *Network) GetName() string {
	return n.Name
}

// GetNamespace returns the namespace of the network
func (n *Network) GetNamespace() string {
	return n.Namespace
}

// GetMeta returns the metadata
func (n *Network) GetMeta() *Meta {
	return &n.Meta
}

// SetNetworkName sets the network name (typically namespace-name)
func (n *Network) SetNetworkName(name string) {
	n.NetworkName = name
}

// SetBindingRef sets the reference to the NetworkBinding
func (n *Network) SetBindingRef(ref *v1beta1.ObjectReference) {
	n.BindingRef = ref
}

// SetAddressGroupRef sets the reference to the AddressGroup
func (n *Network) SetAddressGroupRef(ref *v1beta1.ObjectReference) {
	n.AddressGroupRef = ref
}

// SetIsBound sets the binding status
func (n *Network) SetIsBound(bound bool) {
	n.IsBound = bound
}

// ClearBinding clears all binding-related fields
func (n *Network) ClearBinding() {
	n.IsBound = false
	n.BindingRef = nil
	n.AddressGroupRef = nil
}

// IsReady returns true if the network is ready
func (n *Network) IsReady() bool {
	return n.Meta.IsReady()
}

// IsValidated returns true if the network is validated
func (n *Network) IsValidated() bool {
	return n.Meta.IsValidated()
}

// IsSynced returns true if the network is synced
func (n *Network) IsSynced() bool {
	return n.Meta.IsSynced()
}

// Key returns the unique key for the network (namespace/name)
func (n *Network) Key() string {
	if n.Namespace == "" {
		return n.Name
	}
	return fmt.Sprintf("%s/%s", n.Namespace, n.Name)
}

// GetID returns the unique identifier for the network
func (n *Network) GetID() string {
	return n.Key()
}

// GetGeneration returns the generation of the network
func (n *Network) GetGeneration() int64 {
	return n.Meta.Generation
}

// DeepCopy creates a deep copy of the network
func (n *Network) DeepCopy() Resource {
	if n == nil {
		return nil
	}

	copy := *n
	copy.Meta = n.Meta // Meta is a struct, so this is a shallow copy

	// Deep copy slices and maps if needed
	if n.BindingRef != nil {
		bindingRefCopy := *n.BindingRef
		copy.BindingRef = &bindingRefCopy
	}

	if n.AddressGroupRef != nil {
		addressGroupRefCopy := *n.AddressGroupRef
		copy.AddressGroupRef = &addressGroupRefCopy
	}

	return &copy
}

// SyncableEntity interface implementation for Network

// GetSyncSubjectType returns the sync subject type for Network
func (n *Network) GetSyncSubjectType() types.SyncSubjectType {
	return types.SyncSubjectTypeNetworks
}

// GetSyncKey returns a unique key for the Network
func (n *Network) GetSyncKey() string {
	if n.Namespace != "" {
		return fmt.Sprintf("network-%s/%s", n.Namespace, n.Name)
	}
	return fmt.Sprintf("network-%s", n.Name)
}

// ToSGroupsProto converts the Network to sgroups protobuf format
func (n *Network) ToSGroupsProto() (interface{}, error) {
	if n == nil {
		return nil, fmt.Errorf("Network cannot be nil")
	}

	protoNetwork := map[string]interface{}{
		"name":      n.Name,
		"namespace": n.Namespace,
		"cidr":      n.CIDR,
		"isBound":   n.IsBound,
	}

	return protoNetwork, nil
}
