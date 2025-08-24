// +k8s:deepcopy-gen=package
// +groupName=netguard.sgroups.io

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	Spec          ServiceSpec       `json:"spec,omitempty"`
	Status        ServiceStatus     `json:"status,omitempty"`
	AddressGroups AddressGroupsSpec `json:"addressGroups,omitempty"`
}

// ServiceSpec defines the desired state of Service
type ServiceSpec struct {
	// Description of the service
	// +optional
	Description string `json:"description,omitempty"`

	// IngressPorts defines the ports that are allowed for ingress traffic
	// +optional
	IngressPorts []IngressPort `json:"ingressPorts,omitempty"`
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

// RuleS2SDstOwnRefSpec defines the RuleS2S objects that reference this Service from other namespaces
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type RuleS2SDstOwnRefSpec struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Items contains the list of RuleS2S references
	Items []NamespacedObjectReference `json:"items,omitempty"`
}

// RuleS2SDstOwnRefSpecList contains a list of RuleS2SDstOwnRefSpec
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type RuleS2SDstOwnRefSpecList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RuleS2SDstOwnRefSpec `json:"items"`
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
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Items contains the list of service ports references
	Items []ServicePortsRef `json:"items,omitempty"`
}

// AccessPortsSpecList contains a list of AccessPortsSpec
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type AccessPortsSpecList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AccessPortsSpec `json:"items"`
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

// RuleS2S defines service-to-service rules
type RuleS2S struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RuleS2SSpec   `json:"spec,omitempty"`
	Status RuleS2SStatus `json:"status,omitempty"`
}

// RuleS2SSpec defines the desired state of RuleS2S
type RuleS2SSpec struct {
	// Traffic direction: ingress or egress
	// +kubebuilder:validation:Enum=INGRESS;EGRESS
	// +kubebuilder:validation:Required
	Traffic Traffic `json:"traffic"`

	// ServiceLocalRef is a reference to the local service
	// +kubebuilder:validation:Required
	ServiceLocalRef NamespacedObjectReference `json:"serviceLocalRef"`

	// ServiceRef is a reference to the target service
	// +kubebuilder:validation:Required
	ServiceRef NamespacedObjectReference `json:"serviceRef"`

	// Whether to enable trace
	// +optional
	Trace bool `json:"trace"`
}

// RuleS2SStatus defines the observed state of RuleS2S
type RuleS2SStatus struct {
	// Conditions represent the latest available observations of the rule's current state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// IEAgAgRuleRefs contains references to the IEAgAgRules created for this RuleS2S
	// +optional
	IEAgAgRuleRefs []NamespacedObjectReference `json:"ieAgAgRuleRefs,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RuleS2SList contains a list of RuleS2S
type RuleS2SList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RuleS2S `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ServiceAlias defines an alias for a service
type ServiceAlias struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServiceAliasSpec   `json:"spec,omitempty"`
	Status ServiceAliasStatus `json:"status,omitempty"`
}

// ServiceAliasSpec defines the desired state of ServiceAlias
type ServiceAliasSpec struct {
	// ServiceRef is a reference to the Service resource this alias points to
	// +kubebuilder:validation:Required
	ServiceRef NamespacedObjectReference `json:"serviceRef"`
}

// ServiceAliasStatus defines the observed state of ServiceAlias
type ServiceAliasStatus struct {
	// Conditions represent the latest available observations of the alias's current state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ServiceAliasList contains a list of ServiceAlias
type ServiceAliasList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServiceAlias `json:"items"`
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

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// IEAgAgRule defines ingress/egress address group to address group rules
type IEAgAgRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IEAgAgRuleSpec   `json:"spec,omitempty"`
	Status IEAgAgRuleStatus `json:"status,omitempty"`
}

// IEAgAgRuleSpec defines the desired state of IEAgAgRule
type IEAgAgRuleSpec struct {
	// Description of the rule
	// +optional
	Description string `json:"description,omitempty"`

	// Transport protocol (TCP, UDP, etc.)
	// +kubebuilder:validation:Enum=TCP;UDP
	// +kubebuilder:validation:Required
	Transport TransportProtocol `json:"transport"`

	// Traffic direction (Ingress, Egress)
	// +kubebuilder:validation:Enum=INGRESS;EGRESS
	// +kubebuilder:validation:Required
	Traffic Traffic `json:"traffic"`

	// AddressGroupLocal is the local address group reference
	AddressGroupLocal NamespacedObjectReference `json:"addressGroupLocal"`

	// AddressGroup is the remote address group reference
	AddressGroup NamespacedObjectReference `json:"addressGroup"`

	// Ports defines the port specifications
	// +optional
	Ports []PortSpec `json:"ports,omitempty"`

	// Action for the rule (ACCEPT, DROP)
	// +kubebuilder:validation:Enum=ACCEPT;DROP
	// +optional
	Action RuleAction `json:"action,omitempty"`

	// Priority of the rule
	// +optional
	Priority int32 `json:"priority,omitempty"`

	// Whether to enable trace
	// +optional
	Trace bool `json:"trace"`
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

// IEAgAgRuleStatus defines the observed state of IEAgAgRule
type IEAgAgRuleStatus struct {
	// Conditions represent the latest available observations of the rule's current state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// IEAgAgRuleList contains a list of IEAgAgRule
type IEAgAgRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IEAgAgRule `json:"items"`
}

// NetworkSpec defines the desired state of Network
type NetworkSpec struct {
	// CIDR is the IP range in CIDR notation
	CIDR string `json:"cidr"`
}

// NetworkStatus defines the observed state of Network
type NetworkStatus struct {
	// NetworkName is the name of the network
	NetworkName string `json:"networkName,omitempty"`

	// IsBound indicates if the network is bound to an AddressGroup
	IsBound bool `json:"isBound"`

	// BindingRef is a reference to the NetworkBinding that binds this network
	BindingRef *ObjectReference `json:"bindingRef,omitempty"`

	// AddressGroupRef is a reference to the AddressGroup this network is bound to
	AddressGroupRef *ObjectReference `json:"addressGroupRef,omitempty"`

	// Conditions represent the latest available observations of the resource's state
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Network is the Schema for the networks API
type Network struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NetworkSpec   `json:"spec,omitempty"`
	Status NetworkStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
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
	NetworkRef ObjectReference `json:"networkRef"`

	// AddressGroupRef is a reference to the AddressGroup resource
	AddressGroupRef ObjectReference `json:"addressGroupRef"`
}

// NetworkBindingStatus defines the observed state of NetworkBinding
type NetworkBindingStatus struct {
	// Conditions represent the latest available observations of the resource's state
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// NetworkBinding is the Schema for the networkbindings API
type NetworkBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec        NetworkBindingSpec   `json:"spec,omitempty"`
	Status      NetworkBindingStatus `json:"status,omitempty"`
	NetworkItem NetworkItem          `json:"network,omitempty"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// NetworkBindingList contains a list of NetworkBinding
type NetworkBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NetworkBinding `json:"items"`
}
