package models

import (
	"fmt"

	pb "github.com/PRO-Robotech/protos/pkg/api/sgroups"

	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	"netguard-pg-backend/internal/sync/types"
)

// Service represents a service with its ports
type Service struct {
	SelfRef
	Description   string
	IngressPorts  []IngressPort
	AddressGroups []AddressGroupRef

	// AggregatedAddressGroups contains all address groups from both spec and bindings
	AggregatedAddressGroups []AddressGroupReference

	// XSvcSvcRules contains rules where this Service participates (READ-ONLY)
	// Populated automatically by PostgreSQL triggers via junction table
	XSvcSvcRules *XSvcSvcRules

	// XSvcFqdnRules contains service-to-FQDN rules referencing this Service (READ-ONLY)
	XSvcFqdnRules *XSvcFqdnRules

	Meta Meta
}

// ServiceRef represents a reference to a Service
type ServiceRef = netguardv1beta1.NamespacedObjectReference

// ServiceRefKey generates a key from ServiceRef for maps
func ServiceRefKey(ref ServiceRef) string {
	return ref.Namespace + "/" + ref.Name
}

// NewServiceRef creates a new ServiceRef
func NewServiceRef(name string, opts ...ResourceIdentifierOption) ServiceRef {
	id := NewResourceIdentifier(name, opts...)
	return netguardv1beta1.NamespacedObjectReference{
		ObjectReference: netguardv1beta1.ObjectReference{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "Service",
			Name:       id.Name,
		},
		Namespace: id.Namespace,
	}
}

// AddressGroupReference represents a reference to an AddressGroup with source tracking
type AddressGroupReference struct {
	Ref    netguardv1beta1.NamespacedObjectReference
	Source AddressGroupRegistrationSource
}

// AddressGroupRegistrationSource indicates how an address group was registered
type AddressGroupRegistrationSource string

const (
	// AddressGroupSourceSpec indicates the address group was registered via Service.spec.addressGroups
	AddressGroupSourceSpec AddressGroupRegistrationSource = "spec"
	// AddressGroupSourceBinding indicates the address group was registered via AddressGroupBinding
	AddressGroupSourceBinding AddressGroupRegistrationSource = "binding"
)

// SyncableEntity interface implementation for Service

// GetSyncSubjectType returns the sync subject type for Service
func (s *Service) GetSyncSubjectType() types.SyncSubjectType {
	return types.SyncSubjectTypeServices
}

// GetSyncKey returns a unique key for the Service
func (s *Service) GetSyncKey() string {
	if s.Namespace != "" {
		return fmt.Sprintf("service-%s/%s", s.Namespace, s.Name)
	}
	return fmt.Sprintf("service-%s", s.Name)
}

// ToSGroupsProto converts the Service to sgroups protobuf format
func (s *Service) ToSGroupsProto() (interface{}, error) {
	if s == nil {
		return nil, fmt.Errorf("Service cannot be nil")
	}

	// Build service name with namespace if present
	serviceName := s.Name
	if s.Namespace != "" {
		serviceName = fmt.Sprintf("%s/%s", s.Namespace, s.Name)
	}

	// Build ProtoSpec from IngressPorts
	// Only create tcp/udp structures if there are corresponding ports
	var tcpPorts []*pb.AccPorts
	var udpPorts []*pb.AccPorts

	for _, ingressPort := range s.IngressPorts {
		accPort := &pb.AccPorts{
			S: "", // Source port is always empty for Service
			D: ingressPort.Port,
		}

		switch ingressPort.Protocol {
		case TCP:
			tcpPorts = append(tcpPorts, accPort)
		case UDP:
			udpPorts = append(udpPorts, accPort)
		}
	}

	// Create ProtoSpec with only non-empty port lists
	protoSpec := &pb.ProtoSpec{}
	if len(tcpPorts) > 0 {
		protoSpec.Tcp = &pb.ProtoSpec_Ports{Ports: tcpPorts}
	}
	if len(udpPorts) > 0 {
		protoSpec.Udp = &pb.ProtoSpec_Ports{Ports: udpPorts}
	}

	fmt.Printf("  Protocol summary: tcp_ports=%d udp_ports=%d\n", len(tcpPorts), len(udpPorts))
	for idx, port := range tcpPorts {
		fmt.Printf("    TCP[%d]: s=%q d=%q\n", idx, port.S, port.D)
	}
	for idx, port := range udpPorts {
		fmt.Printf("    UDP[%d]: s=%q d=%q\n", idx, port.S, port.D)
	}
	if protoSpec.Tcp == nil && protoSpec.Udp == nil && protoSpec.Icmpv4 == nil && protoSpec.Icmpv6 == nil {
		fmt.Printf("  WARNING: ProtoSpec has no protocols defined — SGROUP will keep previous ports unless sync_op=FullSync\n")
	}

	// 🔍 DEBUG POINT 2: Log before building sgNames
	fmt.Printf("🔍 [TOSGROUPS_DEBUG] Service.ToSGroupsProto: name=%s, AggregatedAddressGroups count=%d\n",
		serviceName, len(s.AggregatedAddressGroups))
	for i, ag := range s.AggregatedAddressGroups {
		fmt.Printf("  [%d] AG: %s/%s (source: %s)\n", i, ag.Ref.Namespace, ag.Ref.Name, ag.Source)
	}

	// Build sg_names from AggregatedAddressGroups
	// NOTE: SGROUP API interprets sg_names = [""] as an explicit request to clear bindings.
	// If we send an empty slice, SGROUP treats it as "no change" and keeps stale data.
	sgNames := make([]string, 0)
	for _, agRef := range s.AggregatedAddressGroups {
		agName := agRef.Ref.Name
		if agRef.Ref.Namespace != "" {
			agName = fmt.Sprintf("%s/%s", agRef.Ref.Namespace, agRef.Ref.Name)
		}
		sgNames = append(sgNames, agName)
	}

	if len(sgNames) == 0 {
		// Use sentinel value recognized by SGROUP to remove all bindings.
		sgNames = []string{""}
		fmt.Printf("  sgNames empty → sending sentinel [\"\"] to SGROUP to clear bindings\n")
	}

	// Convert to sgroups protobuf element
	protoService := &pb.Service{
		Name:      serviceName,
		Protocols: protoSpec,
		SgNames:   sgNames,
	}

	// 🔍 DEBUG POINT 3: Log final proto
	fmt.Printf("🔍 [TOSGROUPS_DEBUG] Final proto.Service: name=%s, sgNames=%v\n",
		protoService.Name, protoService.SgNames)
	if protoService.Protocols != nil {
		if protoService.Protocols.Tcp != nil {
			fmt.Printf("  protoService.Protocols.Tcp=%v\n", protoService.Protocols.Tcp.Ports)
		}
		if protoService.Protocols.Udp != nil {
			fmt.Printf("  protoService.Protocols.Udp=%v\n", protoService.Protocols.Udp.Ports)
		}
	}

	return protoService, nil
}
