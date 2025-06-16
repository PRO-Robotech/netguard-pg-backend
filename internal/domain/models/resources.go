package models

import (
	"fmt"
	"time"
)

// ResourceIdentifier uniquely identifies a resource by name and namespace
type ResourceIdentifier struct {
	Name      string
	Namespace string
}

// NewResourceIdentifier creates a new ResourceIdentifier
func NewResourceIdentifier(name string, opts ...ResourceIdentifierOption) ResourceIdentifier {
	ri := ResourceIdentifier{Name: name}
	for _, o := range opts {
		o(&ri)
	}
	return ri
}

// Key возвращает:
// - "Name", если Namespace пустой
// - "Namespace/Name" в противном случае
func (r ResourceIdentifier) Key() string {
	if r.Namespace == "" {
		return r.Name
	}
	return fmt.Sprintf("%s/%s", r.Namespace, r.Name)
}

type ResourceIdentifierOption func(*ResourceIdentifier)

//Общие опции для ResourceIdentifier

func WithNamespace(ns string) ResourceIdentifierOption {
	return func(r *ResourceIdentifier) {
		r.Namespace = ns
	}
}

type SelfRef struct {
	ResourceIdentifier
}

func NewSelfRef(identifier ResourceIdentifier) SelfRef {
	return SelfRef{ResourceIdentifier: identifier}
}

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

// SyncStatus represents the status of a synchronization operation
type SyncStatus struct {
	UpdatedAt time.Time
}
