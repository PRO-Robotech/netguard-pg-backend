package models

import (
	"time"
)

// TransportProtocol represents the transport protocol (TCP, UDP)
type TransportProtocol string

const (
	// TCP protocol
	TCP TransportProtocol = "TCP"
	// UDP protocol
	UDP TransportProtocol = "UDP"
)

// Traffic represents the direction of traffic (ingress, egress)
type Traffic string

const (
	// INGRESS traffic direction
	INGRESS Traffic = "ingress"
	// EGRESS traffic direction
	EGRESS Traffic = "egress"
)

// PortRange represents a range of ports
type PortRange struct {
	Start int
	End   int
}

// ProtocolPorts maps protocols to port ranges
type ProtocolPorts map[TransportProtocol][]PortRange

// IngressPort defines a port configuration for ingress traffic
type IngressPort struct {
	Protocol    TransportProtocol
	Port        string
	Description string
}

// ServiceRef represents a reference to a Service
type ServiceRef struct {
	Name      string
	Namespace string
}

// AddressGroupRef represents a reference to an AddressGroup
type AddressGroupRef struct {
	Name      string
	Namespace string
}

// Service represents a service with its ports
type Service struct {
	Name          string
	Namespace     string
	Description   string
	IngressPorts  []IngressPort
	AddressGroups []AddressGroupRef
}

// AddressGroup represents a group of addresses
type AddressGroup struct {
	Name        string
	Namespace   string
	Description string
	Addresses   []string
	Services    []ServiceRef
}

// AddressGroupBinding represents a binding between a Service and an AddressGroup
type AddressGroupBinding struct {
	Name            string
	Namespace       string
	ServiceRef      ServiceRef
	AddressGroupRef AddressGroupRef
}

// ServicePortsRef defines a reference to a Service and its allowed ports
type ServicePortsRef struct {
	Name      string
	Namespace string
	Ports     ProtocolPorts
}

// AddressGroupPortMapping represents a mapping between an AddressGroup and allowed ports
type AddressGroupPortMapping struct {
	Name        string
	Namespace   string
	AccessPorts []ServicePortsRef
}

// RuleS2S represents a rule between two services
type RuleS2S struct {
	Name            string
	Namespace       string
	Traffic         Traffic
	ServiceLocalRef ServiceRef
	ServiceRef      ServiceRef
}

// SyncStatus represents the status of a synchronization operation
type SyncStatus struct {
	UpdatedAt time.Time
}
