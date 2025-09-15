package models

import (
	"fmt"

	pb "github.com/PRO-Robotech/protos/pkg/api/sgroups"

	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/sync/types"
)

// Host represents a host resource in the domain (K8s representation of Agent)
type Host struct {
	SelfRef

	// Specification from HostSpec
	UUID string `json:"uuid"`

	// Status
	HostName         string                   `json:"hostName,omitempty"`
	AddressGroupName string                   `json:"addressGroupName,omitempty"`
	IsBound          bool                     `json:"isBound"`
	BindingRef       *v1beta1.ObjectReference `json:"bindingRef,omitempty"`
	AddressGroupRef  *v1beta1.ObjectReference `json:"addressGroupRef,omitempty"`

	// Metadata
	Meta Meta `json:"meta"`
}

// NewHost creates a new Host with default values
func NewHost(name, namespace, uuid string) *Host {
	return &Host{
		SelfRef: SelfRef{
			ResourceIdentifier: ResourceIdentifier{
				Name:      name,
				Namespace: namespace,
			},
		},
		UUID:    uuid,
		IsBound: false,
		Meta:    Meta{},
	}
}

// GetName returns the name of the host
func (h *Host) GetName() string {
	return h.Name
}

// GetNamespace returns the namespace of the host
func (h *Host) GetNamespace() string {
	return h.Namespace
}

// GetMeta returns the metadata
func (h *Host) GetMeta() *Meta {
	return &h.Meta
}

// SetBindingRef sets the reference to the HostBinding
func (h *Host) SetBindingRef(ref *v1beta1.ObjectReference) {
	h.BindingRef = ref
}

// SetAddressGroupRef sets the reference to the AddressGroup
func (h *Host) SetAddressGroupRef(ref *v1beta1.ObjectReference) {
	h.AddressGroupRef = ref
}

// SetIsBound sets the binding status
func (h *Host) SetIsBound(bound bool) {
	h.IsBound = bound
}

// ClearBinding clears all binding-related fields
func (h *Host) ClearBinding() {
	h.IsBound = false
	h.BindingRef = nil
	h.AddressGroupRef = nil
}

// IsReady returns true if the host is ready
func (h *Host) IsReady() bool {
	return h.Meta.IsReady()
}

// IsValidated returns true if the host is validated
func (h *Host) IsValidated() bool {
	return h.Meta.IsValidated()
}

// IsSynced returns true if the host is synced
func (h *Host) IsSynced() bool {
	return h.Meta.IsSynced()
}

// Key returns the unique key for the host (namespace/name)
func (h *Host) Key() string {
	if h.Namespace == "" {
		return h.Name
	}
	return fmt.Sprintf("%s/%s", h.Namespace, h.Name)
}

// GetID returns the unique identifier for the host
func (h *Host) GetID() string {
	return h.Key()
}

// GetGeneration returns the generation of the host
func (h *Host) GetGeneration() int64 {
	return h.Meta.Generation
}

// DeepCopy creates a deep copy of the host
func (h *Host) DeepCopy() Resource {
	if h == nil {
		return nil
	}

	copy := *h
	copy.Meta = h.Meta // Meta is a struct, so this is a shallow copy

	// Deep copy slices and maps if needed
	if h.BindingRef != nil {
		bindingRefCopy := *h.BindingRef
		copy.BindingRef = &bindingRefCopy
	}

	if h.AddressGroupRef != nil {
		addressGroupRefCopy := *h.AddressGroupRef
		copy.AddressGroupRef = &addressGroupRefCopy
	}

	return &copy
}

// SyncableEntity interface implementation for Host

// GetSyncSubjectType returns the sync subject type for Host
func (h *Host) GetSyncSubjectType() types.SyncSubjectType {
	return types.SyncSubjectTypeHosts // Use the proper constant
}

// GetSyncKey returns a unique key for the Host
func (h *Host) GetSyncKey() string {
	if h.Namespace != "" {
		return fmt.Sprintf("host-%s/%s", h.Namespace, h.Name)
	}
	return fmt.Sprintf("host-%s", h.Name)
}

// ToSGroupsProto converts the Host to sgroups protobuf format
func (h *Host) ToSGroupsProto() (interface{}, error) {
	if h == nil {
		return nil, fmt.Errorf("Host cannot be nil")
	}

	// Build host name with namespace if present
	hostName := h.Name
	if h.Namespace != "" {
		hostName = fmt.Sprintf("%s/%s", h.Namespace, h.Name)
	}

	// Set SgName based on binding status
	sgName := ""
	if h.IsBound && h.AddressGroupRef != nil {
		// Use AddressGroup name as SgName when host is bound
		sgName = h.AddressGroupRef.Name
		if h.Namespace != "" && sgName != "" {
			// Include namespace in SgName for uniqueness
			sgName = fmt.Sprintf("%s/%s", h.Namespace, sgName)
		}
	}

	fmt.Printf("🔍 DEBUG: Host.ToSGroupsProto - Computed values: hostName=%s, sgName=%s\n",
		hostName, sgName)

	// Convert to sgroups protobuf element
	protoHost := &pb.Host{
		Name:   hostName,
		Uuid:   h.UUID,
		SgName: sgName,
		// IpList will be updated by agents
		IpList: &pb.IPList{IPs: []string{}}, // Initialize empty IPList to avoid nil
	}

	fmt.Printf("🔍 DEBUG: Host.ToSGroupsProto - Created protoHost: %+v\n", protoHost)

	// Return single host element
	return protoHost, nil
}
