// +k8s:deepcopy-gen=package
// +groupName=netguard.sgroups.io

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
)

// TransportProtocol represents protocols for transport layer
// +kubebuilder:validation:Enum=TCP;UDP
// +k8s:openapi-gen=true
type TransportProtocol string

const (
	ProtocolTCP TransportProtocol = "TCP"
	ProtocolUDP TransportProtocol = "UDP"
)

// Traffic represents traffic direction for rules
// +kubebuilder:validation:Enum=INGRESS;EGRESS
// +k8s:openapi-gen=true
type Traffic string

const (
	// INGRESS represents ingress traffic
	INGRESS Traffic = "INGRESS"
	// EGRESS represents egress traffic
	EGRESS Traffic = "EGRESS"
)

// RuleAction represents the action to take for a rule
// +kubebuilder:validation:Enum=ACCEPT;DROP
// +k8s:openapi-gen=true
type RuleAction string

const (
	// ActionAccept accepts network packets
	ActionAccept RuleAction = "ACCEPT"
	// ActionDrop drops network packets
	ActionDrop RuleAction = "DROP"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Service defines a network service with its ports and protocol
type Service struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServiceSpec   `json:"spec,omitempty"`
	Status ServiceStatus `json:"status,omitempty"`

	// xAggregatedAddressGroups contains all address groups from both spec.addressGroups and AddressGroupBindings.
	// This field is automatically populated by PostgreSQL triggers and is READ-ONLY.
	// Users should NOT modify this field directly - changes will be ignored.
	// Source field values: "spec" = direct registration via spec.addressGroups, "binding" = registration via AddressGroupBinding
	// +optional
	AggregatedAddressGroups []AddressGroupReference `json:"xAggregatedAddressGroups,omitempty"`

	// xSvcSvcRules contains references to all SvcSvcRule resources where this Service participates.
	// This field is automatically populated by PostgreSQL triggers and is READ-ONLY.
	// Users should NOT modify this field directly - changes will be ignored.
	// +optional
	XSvcSvcRules *XSvcSvcRules `json:"xSvcSvcRules,omitempty"`

	// xSvcFqdnRules contains references to all SvcFqdnRule resources where this Service is the source.
	// This field is automatically populated by PostgreSQL triggers and is READ-ONLY.
	// Users should NOT modify this field directly - changes will be ignored.
	// +optional
	XSvcFqdnRules *XSvcFqdnRules `json:"xSvcFqdnRules,omitempty"`

	// xIECidrSvcRules contains references to all IECidrSvcRule resources where this Service is the target.
	// This field is automatically populated by PostgreSQL triggers and is READ-ONLY.
	// Users should NOT modify this field directly - changes will be ignored.
	// +optional
	XIECidrSvcRules *XIECidrSvcRules `json:"xIECidrSvcRules,omitempty"`
}

// ServiceSpec defines the desired state of Service
type ServiceSpec struct {
	// Description of the service
	// +optional
	Description string `json:"description,omitempty"`

	// IngressPorts defines the ports that are allowed for ingress traffic
	// +optional
	IngressPorts []IngressPort `json:"ingressPorts,omitempty"`

	// AddressGroups is a list of address group references
	// +optional
	AddressGroups []NamespacedObjectReference `json:"addressGroups,omitempty"`

	// Comment - optional user comment (Netguard-only, not synced to SGROUPS)
	// +optional
	Comment string `json:"comment,omitempty"`
}

// IngressPort defines a port configuration for ingress traffic
type IngressPort struct {
	// Transport protocol for the rule
	// +kubebuilder:validation:Enum=TCP;UDP
	Protocol TransportProtocol `json:"protocol"`

	// Port or port range (e.g., "80", "8080-9090")
	Port string `json:"port"`

	// Description of this port configuration
	// +optional
	Description string `json:"description,omitempty"`
}

// PortRange defines a range of ports
type PortRange struct {
	// From port (inclusive)
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	From int32 `json:"from"`

	// To port (inclusive)
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	To int32 `json:"to"`
}

// ServiceStatus defines the observed state of Service
type ServiceStatus struct {
	// Conditions represent the latest available observations of the service's current state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// XSvcSvcRules - READ-ONLY field for Service resource
// Contains references to all rules where this Service participates
// Populated automatically by PostgreSQL triggers via junction table
type XSvcSvcRules struct {
	// AsServiceFrom contains rules where this Service is the source
	// Full NamespacedObjectReference for each rule
	// +optional
	AsServiceFrom []NamespacedObjectReference `json:"asServiceFrom,omitempty"`

	// AsServiceTo contains rules where this Service is the destination
	// Full NamespacedObjectReference for each rule
	// +optional
	AsServiceTo []NamespacedObjectReference `json:"asServiceTo,omitempty"`
}

// XSvcFqdnRules - READ-ONLY field for Service resource
// Contains references to all FQDN rules where this Service is the source
// Populated automatically by PostgreSQL triggers
type XSvcFqdnRules struct {
	// Rules contains the list of FQDN rules where this Service is the source
	// Full NamespacedObjectReference for each FQDN rule
	// +optional
	Rules []NamespacedObjectReference `json:"rules,omitempty"`
}

// AddressGroupsSpec defines the address groups associated with a Service
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type AddressGroupsSpec struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Items contains the list of address groups
	Items []NamespacedObjectReference `json:"items,omitempty"`
}

// AddressGroupsSpecList contains a list of AddressGroupsSpec
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type AddressGroupsSpecList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AddressGroupsSpec `json:"items"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ServiceList contains a list of Service
type ServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Service `json:"items"`
}

// NetworkItem represents a network item in an address group
type NetworkItem struct {
	Name       string `json:"name"`
	CIDR       string `json:"cidr"`
	ApiVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Namespace  string `json:"namespace"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// NetworkItemList contains a list of NetworkItem
type NetworkItemList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NetworkItem `json:"items"`
}

// NetworksSpec defines the networks associated with an address group
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type NetworksSpec struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Items contains the list of network items
	Items []NetworkItem `json:"items,omitempty"`
}

// NetworksSpecList contains a list of NetworksSpec
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type NetworksSpecList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NetworksSpec `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AddressGroup defines a group of network addresses
type AddressGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec     AddressGroupSpec   `json:"spec,omitempty"`
	Status   AddressGroupStatus `json:"status,omitempty"`
	Networks []NetworkItem      `json:"networks,omitempty"` // Networks list

	// AggregatedHosts contains all hosts that belong to this AddressGroup,
	// aggregated from both spec.hosts and HostBinding resources
	// +optional
	AggregatedHosts []HostReference `json:"xAggregatedHosts"`
}

// AddressGroupSpec defines the desired state of AddressGroup
type AddressGroupSpec struct {
	// Default action for the address group
	// +kubebuilder:validation:Enum=ACCEPT;DROP
	// +kubebuilder:validation:Required
	DefaultAction RuleAction `json:"defaultAction"`

	// Whether to enable logs
	// +optional
	Logs bool `json:"logs"`

	// Whether to enable trace
	// +optional
	Trace bool `json:"trace"`

	// Description - optional human-readable description (Netguard-only)
	// +optional
	Description string `json:"description,omitempty"`

	// Hosts that belong exclusively to this AddressGroup
	// Each host can belong to only one AddressGroup
	// Host namespace MUST match AddressGroup namespace
	// +optional
	Hosts []NamespacedObjectReference `json:"hosts,omitempty"`

	// Comment - optional user comment (Netguard-only, not synced to SGROUPS)
	// +optional
	Comment string `json:"comment,omitempty"`
}

// AddressGroupStatus defines the observed state of AddressGroup
type AddressGroupStatus struct {
	// AddressGroupName is the name used in sgroups synchronization
	// +optional
	AddressGroupName string `json:"addressGroupName,omitempty"`

	// Conditions represent the latest available observations of the address group's current state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AddressGroupList contains a list of AddressGroup
type AddressGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AddressGroup `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AddressGroupBinding binds an address group to specific services
type AddressGroupBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AddressGroupBindingSpec   `json:"spec,omitempty"`
	Status AddressGroupBindingStatus `json:"status,omitempty"`
}

// AddressGroupBindingSpec defines the desired state of AddressGroupBinding
type AddressGroupBindingSpec struct {
	// ServiceRef is a reference to the Service resource
	ServiceRef NamespacedObjectReference `json:"serviceRef"`

	// AddressGroupRef is a reference to the AddressGroup resource
	AddressGroupRef NamespacedObjectReference `json:"addressGroupRef"`

	// Comment - optional user comment (Netguard-only)
	// +optional
	Comment string `json:"comment,omitempty"`
}

// ObjectReference contains enough information to let you inspect or modify the referred object
type ObjectReference struct {
	// APIVersion of the referenced object
	APIVersion string `json:"apiVersion"`

	// Kind of the referenced object
	Kind string `json:"kind"`

	// Name of the referenced object
	Name string `json:"name"`
}

// NamespacedObjectReference extends ObjectReference with a Namespace field
type NamespacedObjectReference struct {
	// Embedded ObjectReference
	ObjectReference `json:",inline"`

	// Namespace of the referenced object
	Namespace string `json:"namespace,omitempty"`
}

// HostRegistrationSource represents the source of host registration
// +kubebuilder:validation:Enum=spec;binding
type HostRegistrationSource string

const (
	// HostSourceSpec indicates the host was registered via AddressGroup.spec.hosts
	HostSourceSpec HostRegistrationSource = "spec"
	// HostSourceBinding indicates the host was registered via HostBinding
	HostSourceBinding HostRegistrationSource = "binding"
)

// HostReference represents a reference to a Host with additional metadata
type HostReference struct {
	// Reference to the Host object
	// Host namespace MUST match AddressGroup namespace
	Ref NamespacedObjectReference `json:"ref"`

	// UUID of the host (for efficient lookup and SGroup sync)
	UUID string `json:"uuid"`

	// Source indicates how this host was registered (spec or binding)
	Source HostRegistrationSource `json:"source"`
}

// AddressGroupRegistrationSource represents the source of address group registration
// +kubebuilder:validation:Enum=spec;binding
type AddressGroupRegistrationSource string

const (
	// AddressGroupSourceSpec indicates the address group was registered via Service.spec.addressGroups
	AddressGroupSourceSpec AddressGroupRegistrationSource = "spec"
	// AddressGroupSourceBinding indicates the address group was registered via AddressGroupBinding
	AddressGroupSourceBinding AddressGroupRegistrationSource = "binding"
)

// AddressGroupReference represents a reference to an AddressGroup with source tracking
type AddressGroupReference struct {
	// Ref contains the full Kubernetes object reference
	Ref NamespacedObjectReference `json:"ref"`

	// Source indicates how this address group was registered
	// +kubebuilder:validation:Enum=spec;binding
	// +kubebuilder:validation:Required
	Source AddressGroupRegistrationSource `json:"source"`
}

// PortConfig defines a port or port range configuration
type PortConfig struct {
	// Port or port range (e.g., "80", "8080-9090")
	Port string `json:"port"`

	// Description of this port configuration
	// +optional
	Description string `json:"description,omitempty"`
}

// ProtocolPorts defines ports by protocol
type ProtocolPorts struct {
	// TCP ports
	// +optional
	TCP []PortConfig `json:"TCP,omitempty"`

	// UDP ports
	// +optional
	UDP []PortConfig `json:"UDP,omitempty"`
}

// ServicePortsRef defines a reference to a Service and its allowed ports
type ServicePortsRef struct {
	// Reference to the service
	NamespacedObjectReference `json:",inline"`

	// Ports defines the allowed ports by protocol
	Ports ProtocolPorts `json:"ports"`
}

// AccessPortsSpec defines the services and their ports that are allowed access
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type AccessPortsSpec struct {
	// MappingName optionally identifies the parent AddressGroupPortMapping (used by subresource responses)
	// +optional
	MappingName string `json:"mappingName,omitempty"`

	// MappingNamespace optionally identifies the parent AddressGroupPortMapping namespace (used by subresource responses)
	// +optional
	MappingNamespace string `json:"mappingNamespace,omitempty"`

	// Items contains the list of service ports references
	Items []ServicePortsRef `json:"items,omitempty"`
}

// AccessPortsSpecList contains a list of AccessPortsSpec entries
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type AccessPortsSpecList struct {
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AccessPortsSpec `json:"items"`
}

// GetObjectKind implements runtime.Object without inlining TypeMeta in JSON responses
func (AccessPortsSpec) GetObjectKind() schema.ObjectKind {
	return schema.EmptyObjectKind
}

// GetObjectKind implements runtime.Object for the list variant without extra JSON fields
func (AccessPortsSpecList) GetObjectKind() schema.ObjectKind {
	return schema.EmptyObjectKind
}

// AddressGroupBindingStatus defines the observed state of AddressGroupBinding
type AddressGroupBindingStatus struct {
	// Conditions represent the latest available observations of the binding's current state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AddressGroupBindingList contains a list of AddressGroupBinding
type AddressGroupBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AddressGroupBinding `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AddressGroupPortMapping defines port mappings for address groups
type AddressGroupPortMapping struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec        AddressGroupPortMappingSpec   `json:"spec,omitempty"`
	Status      AddressGroupPortMappingStatus `json:"status,omitempty"`
	AccessPorts AccessPortsSpec               `json:"accessPorts,omitempty"`
}

// AddressGroupPortMappingSpec defines the desired state of AddressGroupPortMapping
type AddressGroupPortMappingSpec struct {
	// Empty spec as in controller
}

// AddressGroupPortMappingStatus defines the observed state of AddressGroupPortMapping
type AddressGroupPortMappingStatus struct {
	// Conditions represent the latest available observations of the port mapping's current state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AddressGroupPortMappingList contains a list of AddressGroupPortMapping
type AddressGroupPortMappingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AddressGroupPortMapping `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AddressGroupBindingPolicy defines policies for address group bindings
type AddressGroupBindingPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AddressGroupBindingPolicySpec   `json:"spec,omitempty"`
	Status AddressGroupBindingPolicyStatus `json:"status,omitempty"`
}

// AddressGroupBindingPolicySpec defines the desired state of AddressGroupBindingPolicy
type AddressGroupBindingPolicySpec struct {
	// AddressGroupRef is a reference to the AddressGroup resource
	AddressGroupRef NamespacedObjectReference `json:"addressGroupRef"`

	// ServiceRef is a reference to the Service resource
	ServiceRef NamespacedObjectReference `json:"serviceRef"`
}

// AddressGroupBindingPolicyStatus defines the observed state of AddressGroupBindingPolicy
type AddressGroupBindingPolicyStatus struct {
	// Conditions represent the latest available observations of the policy's current state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AddressGroupBindingPolicyList contains a list of AddressGroupBindingPolicy
type AddressGroupBindingPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AddressGroupBindingPolicy `json:"items"`
}

// PortSpec defines a port specification
type PortSpec struct {
	// Port number
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	Port int32 `json:"port,omitempty"`

	// PortRange defines a range of ports
	// +optional
	PortRange *PortRange `json:"portRange,omitempty"`
}

// NetworkSpec defines the desired state of Network
type NetworkSpec struct {
	// CIDR is the IP range in CIDR notation
	CIDR string `json:"cidr"`

	// Comment - optional user comment (Netguard-only, not synced to SGROUPS)
	// +optional
	Comment string `json:"comment,omitempty"`
}

// NetworkStatus defines the observed state of Network
type NetworkStatus struct {
	// NetworkName is the name of the network
	NetworkName string `json:"networkName,omitempty"`

	// IsBound indicates if the network is bound to an AddressGroup
	IsBound bool `json:"isBound"`

	// BindingRef is a reference to the NetworkBinding that binds this network
	BindingRef *NamespacedObjectReference `json:"bindingRef,omitempty"`

	// AddressGroupRef is a reference to the AddressGroup this network is bound to
	AddressGroupRef *NamespacedObjectReference `json:"addressGroupRef,omitempty"`

	// Conditions represent the latest available observations of the resource's state
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +genclient

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Network is the Schema for the networks API
type Network struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NetworkSpec   `json:"spec,omitempty"`
	Status NetworkStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// NetworkList contains a list of Network
type NetworkList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Network `json:"items"`
}

// NetworkBindingSpec defines the desired state of NetworkBinding
type NetworkBindingSpec struct {
	// NetworkRef is a reference to the Network resource
	NetworkRef NamespacedObjectReference `json:"networkRef"`

	// AddressGroupRef is a reference to the AddressGroup resource
	AddressGroupRef NamespacedObjectReference `json:"addressGroupRef"`

	// Comment - optional user comment (Netguard-only)
	// +optional
	Comment string `json:"comment,omitempty"`
}

// NetworkBindingStatus defines the observed state of NetworkBinding
type NetworkBindingStatus struct {
	// Conditions represent the latest available observations of the resource's state
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// NetworkBinding is the Schema for the networkbindings API
type NetworkBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NetworkBindingSpec   `json:"spec,omitempty"`
	Status NetworkBindingStatus `json:"status,omitempty"`
	//NetworkItem NetworkItem          `json:"network,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// NetworkBindingList contains a list of NetworkBinding
type NetworkBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NetworkBinding `json:"items"`
}

// HostSpec defines the desired state of Host
type HostSpec struct {
	// UUID is the unique identifier of the host
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`
	UUID string `json:"uuid"`

	// Comment - optional user comment (Netguard-only, not synced to SGROUPS)
	// +optional
	Comment string `json:"comment,omitempty"`
}

// HostStatus defines the observed state of Host
type HostStatus struct {
	// HostName is the name used for host synchronization
	// +optional
	HostName string `json:"hostName,omitempty"`

	// AddressGroupName is the name of bound AddressGroup
	// +optional
	AddressGroupName string `json:"addressGroupName,omitempty"`

	// IsBound indicates if the host is bound to an AddressGroup
	IsBound bool `json:"isBound"`

	// BindingRef is a reference to the HostBinding that binds this host
	// +optional
	BindingRef *NamespacedObjectReference `json:"bindingRef,omitempty"`

	// AddressGroupRef is a reference to the AddressGroup this host is bound to
	// +optional
	AddressGroupRef *NamespacedObjectReference `json:"addressGroupRef,omitempty"`

	// Conditions represent the latest available observations of the resource's state
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	// ObservedGeneration is the most recent generation observed by the controller
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

type IPItem struct {
	IP string `json:"ip"`
}

// HostMetaInfo contains meta information about the host from SGROUP (read-only)
type HostMetaInfo struct {
	// HostName is the hostname reported by the agent
	// +optional
	HostName string `json:"hostName,omitempty"`

	// Os is the operating system (e.g., linux, windows)
	// +optional
	Os string `json:"os,omitempty"`

	// Platform is the platform (e.g., ubuntu, centos)
	// +optional
	Platform string `json:"platform,omitempty"`

	// PlatformFamily is the platform family (e.g., debian, rhel)
	// +optional
	PlatformFamily string `json:"platformFamily,omitempty"`

	// PlatformVersion is the platform version
	// +optional
	PlatformVersion string `json:"platformVersion,omitempty"`

	// KernelVersion is the kernel version
	// +optional
	KernelVersion string `json:"kernelVersion,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Host is the Schema for the hosts API
type Host struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HostSpec   `json:"spec,omitempty"`
	Status HostStatus `json:"status,omitempty"`

	// IPList contains IP addresses for this Host, synchronized from SGROUP
	// +optional
	IPList []IPItem `json:"xIPList"`

	// MetaInfo contains meta information for this Host, synchronized from SGROUP (read-only)
	// +optional
	MetaInfo *HostMetaInfo `json:"xMetaInfo"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// HostList contains a list of Host
type HostList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Host `json:"items"`
}

// HostBindingSpec defines the desired state of HostBinding
type HostBindingSpec struct {
	// HostRef is a reference to the Host resource
	HostRef NamespacedObjectReference `json:"hostRef"`

	// AddressGroupRef is a reference to the AddressGroup resource
	AddressGroupRef NamespacedObjectReference `json:"addressGroupRef"`

	// Comment - optional user comment (Netguard-only)
	// +optional
	Comment string `json:"comment,omitempty"`
}

// HostBindingStatus defines the observed state of HostBinding
type HostBindingStatus struct {
	// Conditions represent the latest available observations of the resource's state
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	// ObservedGeneration is the most recent generation observed by the controller
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// HostBinding is the Schema for the hostbindings API
type HostBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HostBindingSpec   `json:"spec,omitempty"`
	Status HostBindingStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// HostBindingList contains a list of HostBinding
type HostBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HostBinding `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SvcSvcRule represents a service-to-service firewall rule
type SvcSvcRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SvcSvcRuleSpec   `json:"spec"`
	Status SvcSvcRuleStatus `json:"status,omitempty"`
}

// SvcSvcRuleSpec defines the desired state of SvcSvcRule
type SvcSvcRuleSpec struct {
	// ServiceFrom - source service reference
	// +kubebuilder:validation:Required
	ServiceFrom NamespacedObjectReference `json:"serviceFrom"`

	// ServiceTo - destination service reference
	// +kubebuilder:validation:Required
	ServiceTo NamespacedObjectReference `json:"serviceTo"`

	// Action for the rule (ACCEPT, DROP)
	// +kubebuilder:validation:Enum=ACCEPT;DROP
	// +optional
	Action RuleAction `json:"action"`

	// Priority - rule priority (0-1000, lower = higher priority)
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=1000
	// +optional
	Priority int32 `json:"priority,omitempty"`

	// Logs - enable traffic logging
	// +optional
	Logs bool `json:"logs"`

	// Trace - enable detailed tracing
	// +optional
	Trace bool `json:"trace"`

	// Description - optional human-readable description (Netguard-only)
	// +optional
	Description string `json:"description,omitempty"`

	// Comment - optional user comment (Netguard-only, not synced to SGROUPS)
	// +optional
	Comment string `json:"comment,omitempty"`
}

// SvcSvcRuleStatus defines the observed state
type SvcSvcRuleStatus struct {
	// Conditions represent the latest available observations of the rule's current state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// SyncReady indicates if the rule is ready for SGROUP synchronization
	// +optional
	SyncReady bool `json:"syncReady,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SvcSvcRuleList contains a list of SvcSvcRule
type SvcSvcRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SvcSvcRule `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SvcFqdnRule represents a service-to-FQDN firewall rule
type SvcFqdnRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SvcFqdnRuleSpec   `json:"spec"`
	Status SvcFqdnRuleStatus `json:"status,omitempty"`
}

// SvcFqdnRuleSpec defines the desired state of SvcFqdnRule
type SvcFqdnRuleSpec struct {
	// ServiceFrom - source service reference (NamespacedObjectReference)
	// +kubebuilder:validation:Required
	ServiceFrom NamespacedObjectReference `json:"serviceFrom"`

	// FQDN - fully qualified domain name
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^([a-z0-9]([-a-z0-9]*[a-z0-9])?\.)*[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	FQDN string `json:"fqdn"`

	// Transport protocol (TCP or UDP)
	// +kubebuilder:validation:Enum=TCP;UDP
	Transport TransportProtocol `json:"transport"`

	// Ports - list of port expressions. Each entry defines allowed ports.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Ports []SvcFqdnPortSpec `json:"ports"`

	// Logs - enable traffic logging
	// +optional
	Logs bool `json:"logs"`

	// Trace - enable detailed tracing
	// +optional
	Trace bool `json:"trace"`

	// Action - firewall action (ACCEPT or DROP)
	// +kubebuilder:validation:Enum=ACCEPT;DROP
	Action RuleAction `json:"action"`

	// Priority - rule priority (0-1000, lower = higher priority)
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=1000
	// +optional
	Priority int32 `json:"priority,omitempty"`

	// Description - optional human-readable description
	// +optional
	Description string `json:"description,omitempty"`

	// Comment - optional user comment (Netguard-only, not synced to SGROUPS)
	// +optional
	Comment string `json:"comment,omitempty"`
}

// SvcFqdnPortSpec represents a single port specification for FQDN rule
type SvcFqdnPortSpec struct {
	// Port - single port or port range (e.g., "80", "1000-2000", "443,8080")
	// Can specify multiple ports/ranges separated by commas.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Port string `json:"port"`
}

// SvcFqdnRuleStatus defines the observed state of SvcFqdnRule
type SvcFqdnRuleStatus struct {
	// Conditions represent the latest available observations of the rule's current state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// SyncReady indicates if the rule is ready for SGROUP synchronization
	// +optional
	SyncReady bool `json:"syncReady,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SvcFqdnRuleList contains a list of SvcFqdnRule
type SvcFqdnRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SvcFqdnRule `json:"items"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// IECidrSvcRule represents an ingress/egress CIDR-to-service firewall rule
type IECidrSvcRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IECidrSvcRuleSpec   `json:"spec"`
	Status IECidrSvcRuleStatus `json:"status,omitempty"`
}

// IECidrSvcRuleSpec defines the desired state of IECidrSvcRule
type IECidrSvcRuleSpec struct {
	// Transport protocol (TCP or UDP)
	// +kubebuilder:validation:Enum=TCP;UDP
	Transport TransportProtocol `json:"transport"`

	// CIDR - IPv4 or IPv6 CIDR notation (e.g., "10.0.0.0/8", "2001:db8::/32")
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^([0-9]{1,3}\.){3}[0-9]{1,3}/[0-9]{1,2}$|^([0-9a-fA-F]{0,4}:){2,7}[0-9a-fA-F]{0,4}/[0-9]{1,3}$`
	CIDR string `json:"cidr"`

	// Svc - service reference (NamespacedObjectReference)
	// +kubebuilder:validation:Required
	Svc NamespacedObjectReference `json:"svc"`

	// Traffic direction (INGRESS or EGRESS)
	// +kubebuilder:validation:Enum=INGRESS;EGRESS
	Traffic Traffic `json:"traffic"`

	// Ports - list of port specifications. Each entry can have source and destination ports.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Ports []IECidrSvcPortSpec `json:"ports"`

	// Logs - enable traffic logging
	// +optional
	Logs bool `json:"logs"`

	// Trace - enable detailed tracing
	// +optional
	Trace bool `json:"trace"`

	// Action - firewall action (ACCEPT or DROP)
	// +kubebuilder:validation:Enum=ACCEPT;DROP
	Action RuleAction `json:"action"`

	// Priority - rule priority (0-1000, lower = higher priority)
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=1000
	// +optional
	Priority int32 `json:"priority,omitempty"`

	// Description - optional human-readable description
	// +optional
	Description string `json:"description,omitempty"`

	// Comment - optional user comment (Netguard-only, not synced to SGROUPS)
	// +optional
	Comment string `json:"comment,omitempty"`
}

// IECidrSvcPortSpec represents a port specification for CIDR-to-service rule
type IECidrSvcPortSpec struct {
	// S - source port expression (optional)
	// Can be a single port, range, or comma-separated list
	// +optional
	S string `json:"s,omitempty"`

	// D - destination port expression (required)
	// Can be a single port, range, or comma-separated list
	// +kubebuilder:validation:Required
	D string `json:"d"`
}

// IECidrSvcRuleStatus defines the observed state of IECidrSvcRule
type IECidrSvcRuleStatus struct {
	// Conditions represent the latest available observations of the rule's current state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// SyncReady indicates if the rule is ready for SGROUP synchronization
	// +optional
	SyncReady bool `json:"syncReady,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// IECidrSvcRuleList contains a list of IECidrSvcRule
type IECidrSvcRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IECidrSvcRule `json:"items"`
}

// XIECidrSvcRules - READ-ONLY field for Service resource
// Contains references to all IECidrSvcRule resources where this Service is the target
// Populated automatically by PostgreSQL triggers
type XIECidrSvcRules struct {
	// Rules contains the list of CIDR-to-service rules where this Service is the target
	// Full NamespacedObjectReference for each rule
	// +optional
	Rules []NamespacedObjectReference `json:"rules,omitempty"`
}
