package models

import (
	"fmt"

	pb "netguard-pg-backend/protos/pkg/api/agent/v1"

	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/sync/types"
)

// Agent represents an agent resource in the domain
type Agent struct {
	SelfRef

	// Specification
	UUID string `json:"uuid"`
	Name string `json:"name"`

	// Status
	IsBound         bool                     `json:"isBound"`
	BindingRef      *v1beta1.ObjectReference `json:"bindingRef,omitempty"`
	AddressGroupRef *v1beta1.ObjectReference `json:"addressGroupRef,omitempty"`

	// Metadata
	Meta Meta `json:"meta"`
}

// NewAgent creates a new Agent with default values
func NewAgent(name, namespace, uuid, hostname string) *Agent {
	return &Agent{
		SelfRef: SelfRef{
			ResourceIdentifier: ResourceIdentifier{
				Name:      name,
				Namespace: namespace,
			},
		},
		UUID:    uuid,
		Name:    hostname,
		IsBound: false,
		Meta:    Meta{},
	}
}

// GetName returns the name of the agent
func (a *Agent) GetName() string {
	return a.Name
}

// GetNamespace returns the namespace of the agent
func (a *Agent) GetNamespace() string {
	return a.Namespace
}

// GetMeta returns the metadata
func (a *Agent) GetMeta() *Meta {
	return &a.Meta
}

// SetBindingRef sets the reference to the AgentBinding
func (a *Agent) SetBindingRef(ref *v1beta1.ObjectReference) {
	a.BindingRef = ref
}

// SetAddressGroupRef sets the reference to the AddressGroup
func (a *Agent) SetAddressGroupRef(ref *v1beta1.ObjectReference) {
	a.AddressGroupRef = ref
}

// SetIsBound sets the binding status
func (a *Agent) SetIsBound(bound bool) {
	a.IsBound = bound
}

// ClearBinding clears all binding-related fields
func (a *Agent) ClearBinding() {
	a.IsBound = false
	a.BindingRef = nil
	a.AddressGroupRef = nil
}

// IsReady returns true if the agent is ready
func (a *Agent) IsReady() bool {
	return a.Meta.IsReady()
}

// IsValidated returns true if the agent is validated
func (a *Agent) IsValidated() bool {
	return a.Meta.IsValidated()
}

// IsSynced returns true if the agent is synced
func (a *Agent) IsSynced() bool {
	return a.Meta.IsSynced()
}

// Key returns the unique key for the agent (namespace/name)
func (a *Agent) Key() string {
	if a.Namespace == "" {
		return a.Name
	}
	return fmt.Sprintf("%s/%s", a.Namespace, a.Name)
}

// GetID returns the unique identifier for the agent
func (a *Agent) GetID() string {
	return a.Key()
}

// GetGeneration returns the generation of the agent
func (a *Agent) GetGeneration() int64 {
	return a.Meta.Generation
}

// DeepCopy creates a deep copy of the agent
func (a *Agent) DeepCopy() Resource {
	if a == nil {
		return nil
	}

	copy := *a
	copy.Meta = a.Meta // Meta is a struct, so this is a shallow copy

	// Deep copy slices and maps if needed
	if a.BindingRef != nil {
		bindingRefCopy := *a.BindingRef
		copy.BindingRef = &bindingRefCopy
	}

	if a.AddressGroupRef != nil {
		addressGroupRefCopy := *a.AddressGroupRef
		copy.AddressGroupRef = &addressGroupRefCopy
	}

	return &copy
}

// SyncableEntity interface implementation for Agent

// GetSyncSubjectType returns the sync subject type for Agent
func (a *Agent) GetSyncSubjectType() types.SyncSubjectType {
	return types.SyncSubjectTypeAgents
}

// GetSyncKey returns a unique key for the Agent
func (a *Agent) GetSyncKey() string {
	if a.Namespace != "" {
		return fmt.Sprintf("agent-%s/%s", a.Namespace, a.Name)
	}
	return fmt.Sprintf("agent-%s", a.Name)
}

// ToSGroupsProto converts the Agent to sgroups protobuf format
func (a *Agent) ToSGroupsProto() (interface{}, error) {
	if a == nil {
		return nil, fmt.Errorf("Agent cannot be nil")
	}

	// Convert to single sgroups protobuf element (batch aggregation will be handled by syncer)
	protoAgent := &pb.RegInfo{
		Uuid: a.UUID,
		Name: a.Name,
	}

	// Return single agent element (not wrapped in SyncAgents)
	// Batch aggregation will be handled by AgentSyncer.SyncBatch()
	return protoAgent, nil
}
