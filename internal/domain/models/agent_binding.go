package models

import (
	"fmt"

	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/sync/types"
)

// AgentBinding represents an agent binding resource in the domain
type AgentBinding struct {
	SelfRef

	// Specification
	AgentRef        v1beta1.ObjectReference `json:"agentRef"`
	AddressGroupRef v1beta1.ObjectReference `json:"addressGroupRef"`

	// AgentItem contains the agent information
	AgentItem AgentItem `json:"agentItem"`

	// Metadata
	Meta Meta `json:"meta"`
}

// AgentItem represents agent information stored in bindings
type AgentItem struct {
	Name       string `json:"name"`
	UUID       string `json:"uuid"`
	Hostname   string `json:"hostname"`
	ApiVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Namespace  string `json:"namespace"`
}

// NewAgentBinding creates a new AgentBinding with default values
func NewAgentBinding(name, namespace string, agentRef, addressGroupRef v1beta1.ObjectReference) *AgentBinding {
	return &AgentBinding{
		SelfRef: SelfRef{
			ResourceIdentifier: ResourceIdentifier{
				Name:      name,
				Namespace: namespace,
			},
		},
		AgentRef:        agentRef,
		AddressGroupRef: addressGroupRef,
		Meta:            Meta{},
	}
}

// GetName returns the name of the agent binding
func (ab *AgentBinding) GetName() string {
	return ab.Name
}

// GetNamespace returns the namespace of the agent binding
func (ab *AgentBinding) GetNamespace() string {
	return ab.Namespace
}

// GetMeta returns the metadata
func (ab *AgentBinding) GetMeta() *Meta {
	return &ab.Meta
}

// SetAgentItem sets the agent item information
func (ab *AgentBinding) SetAgentItem(item AgentItem) {
	ab.AgentItem = item
}

// IsReady returns true if the agent binding is ready
func (ab *AgentBinding) IsReady() bool {
	return ab.Meta.IsReady()
}

// IsValidated returns true if the agent binding is validated
func (ab *AgentBinding) IsValidated() bool {
	return ab.Meta.IsValidated()
}

// IsSynced returns true if the agent binding is synced
func (ab *AgentBinding) IsSynced() bool {
	return ab.Meta.IsSynced()
}

// Key returns the unique key for the agent binding (namespace/name)
func (ab *AgentBinding) Key() string {
	if ab.Namespace == "" {
		return ab.Name
	}
	return fmt.Sprintf("%s/%s", ab.Namespace, ab.Name)
}

// GetID returns the unique identifier for the agent binding
func (ab *AgentBinding) GetID() string {
	return ab.Key()
}

// GetGeneration returns the generation of the agent binding
func (ab *AgentBinding) GetGeneration() int64 {
	return ab.Meta.Generation
}

// DeepCopy creates a deep copy of the agent binding
func (ab *AgentBinding) DeepCopy() Resource {
	if ab == nil {
		return nil
	}

	copy := *ab
	copy.Meta = ab.Meta // Meta is a struct, so this is a shallow copy

	// Deep copy slices and maps if needed
	copy.AgentRef = ab.AgentRef
	copy.AddressGroupRef = ab.AddressGroupRef
	copy.AgentItem = ab.AgentItem

	return &copy
}

// SyncableEntity interface implementation for AgentBinding

// GetSyncSubjectType returns the sync subject type for AgentBinding
func (ab *AgentBinding) GetSyncSubjectType() types.SyncSubjectType {
	return types.SyncSubjectTypeAgentBindings
}

// GetSyncKey returns a unique key for the AgentBinding
func (ab *AgentBinding) GetSyncKey() string {
	if ab.Namespace != "" {
		return fmt.Sprintf("agentbinding-%s/%s", ab.Namespace, ab.Name)
	}
	return fmt.Sprintf("agentbinding-%s", ab.Name)
}

// ToSGroupsProto converts the AgentBinding to sgroups protobuf format
func (ab *AgentBinding) ToSGroupsProto() (interface{}, error) {
	if ab == nil {
		return nil, fmt.Errorf("AgentBinding cannot be nil")
	}

	protoBinding := map[string]interface{}{
		"name":            ab.Name,
		"namespace":       ab.Namespace,
		"agentRef":        ab.AgentRef,
		"addressGroupRef": ab.AddressGroupRef,
		"agentItem":       ab.AgentItem,
	}

	return protoBinding, nil
}

// Finalizer management methods

// HasFinalizer returns true if the AgentBinding has the specified finalizer
func (ab *AgentBinding) HasFinalizer(finalizer string) bool {
	for _, f := range ab.Meta.Finalizers {
		if f == finalizer {
			return true
		}
	}
	return false
}

// AddFinalizer adds a finalizer to the AgentBinding if it doesn't already exist
func (ab *AgentBinding) AddFinalizer(finalizer string) {
	if !ab.HasFinalizer(finalizer) {
		ab.Meta.Finalizers = append(ab.Meta.Finalizers, finalizer)
	}
}

// RemoveFinalizer removes a finalizer from the AgentBinding
func (ab *AgentBinding) RemoveFinalizer(finalizer string) {
	var newFinalizers []string
	for _, f := range ab.Meta.Finalizers {
		if f != finalizer {
			newFinalizers = append(newFinalizers, f)
		}
	}
	ab.Meta.Finalizers = newFinalizers
}

// HasAddressGroupSyncFinalizer returns true if the AgentBinding has the AddressGroupSyncFinalizer
func (ab *AgentBinding) HasAddressGroupSyncFinalizer() bool {
	return ab.HasFinalizer(AddressGroupSyncFinalizer)
}

// AddAddressGroupSyncFinalizer adds the AddressGroupSyncFinalizer to the AgentBinding
func (ab *AgentBinding) AddAddressGroupSyncFinalizer() {
	ab.AddFinalizer(AddressGroupSyncFinalizer)
}

// RemoveAddressGroupSyncFinalizer removes the AddressGroupSyncFinalizer from the AgentBinding
func (ab *AgentBinding) RemoveAddressGroupSyncFinalizer() {
	ab.RemoveFinalizer(AddressGroupSyncFinalizer)
}
