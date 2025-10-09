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

	// Build TransportSpec from IngressPorts
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

	// Create TransportSpec with only non-empty port lists
	transportSpec := &pb.TransportSpec{}
	if len(tcpPorts) > 0 {
		transportSpec.Tcp = &pb.TransportSpec_Ports{Ports: tcpPorts}
	}
	if len(udpPorts) > 0 {
		transportSpec.Udp = &pb.TransportSpec_Ports{Ports: udpPorts}
	}

	// Build sg_names from AggregatedAddressGroups
	var sgNames []string
	for _, agRef := range s.AggregatedAddressGroups {
		agName := agRef.Ref.Name
		if agRef.Ref.Namespace != "" {
			agName = fmt.Sprintf("%s/%s", agRef.Ref.Namespace, agRef.Ref.Name)
		}
		sgNames = append(sgNames, agName)
	}

	// Convert to sgroups protobuf element
	protoService := &pb.Service{
		Name:    serviceName,
		Ports:   transportSpec,
		SgNames: sgNames,
	}

	return protoService, nil
}
