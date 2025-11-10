package models

import (
	"testing"

	pb "github.com/PRO-Robotech/protos/pkg/api/sgroups"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

func TestServiceToSGroupsProtoClearsSgNamesWithSentinel(t *testing.T) {
	svc := &Service{
		SelfRef: SelfRef{
			ResourceIdentifier: ResourceIdentifier{
				Namespace: "incloud-sgroups",
				Name:      "demo-svc",
			},
		},
	}

	protoData, err := svc.ToSGroupsProto()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	protoSvc, ok := protoData.(*pb.Service)
	if !ok {
		t.Fatalf("expected *pb.Service, got %T", protoData)
	}

	if len(protoSvc.SgNames) != 1 || protoSvc.SgNames[0] != "" {
		t.Fatalf("expected sentinel sgNames with single empty string, got %v", protoSvc.SgNames)
	}
}

func TestServiceToSGroupsProtoAggregatedSgNames(t *testing.T) {
	svc := &Service{
		SelfRef: SelfRef{
			ResourceIdentifier: ResourceIdentifier{
				Namespace: "incloud-sgroups",
				Name:      "demo-svc",
			},
		},
		AggregatedAddressGroups: []AddressGroupReference{
			{
				Ref: netguardv1beta1.NamespacedObjectReference{
					Namespace: "incloud-sgroups",
					ObjectReference: netguardv1beta1.ObjectReference{
						Name: "ag-1",
					},
				},
			},
			{
				Ref: netguardv1beta1.NamespacedObjectReference{
					Namespace: "tenant-x",
					ObjectReference: netguardv1beta1.ObjectReference{
						Name: "ag-2",
					},
				},
			},
		},
	}

	protoData, err := svc.ToSGroupsProto()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	protoSvc, ok := protoData.(*pb.Service)
	if !ok {
		t.Fatalf("expected *pb.Service, got %T", protoData)
	}

	expected := []string{"incloud-sgroups/ag-1", "tenant-x/ag-2"}
	if len(protoSvc.SgNames) != len(expected) {
		t.Fatalf("expected %d sgNames, got %v", len(expected), protoSvc.SgNames)
	}
	for i, name := range expected {
		if protoSvc.SgNames[i] != name {
			t.Fatalf("expected sgNames[%d] = %s, got %s", i, name, protoSvc.SgNames[i])
		}
	}
}

func TestServiceToSGroupsProtoIncludesProtocols(t *testing.T) {
	svc := &Service{
		SelfRef: SelfRef{
			ResourceIdentifier: ResourceIdentifier{
				Namespace: "incloud-sgroups",
				Name:      "demo-svc",
			},
		},
		IngressPorts: []IngressPort{
			{
				Protocol:    TCP,
				Port:        "8080",
				Description: "http",
			},
			{
				Protocol:    UDP,
				Port:        "9090",
				Description: "syslog",
			},
		},
	}

	protoData, err := svc.ToSGroupsProto()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	protoSvc, ok := protoData.(*pb.Service)
	if !ok {
		t.Fatalf("expected *pb.Service, got %T", protoData)
	}

	if protoSvc.Protocols == nil {
		t.Fatalf("expected protocols to be populated, got nil")
	}

	if protoSvc.Protocols.Tcp == nil || len(protoSvc.Protocols.Tcp.Ports) != 1 {
		t.Fatalf("expected exactly one TCP port, got %+v", protoSvc.Protocols.Tcp)
	}
	if protoSvc.Protocols.Tcp.Ports[0].D != "8080" {
		t.Fatalf("expected TCP port 8080, got %s", protoSvc.Protocols.Tcp.Ports[0].D)
	}

	if protoSvc.Protocols.Udp == nil || len(protoSvc.Protocols.Udp.Ports) != 1 {
		t.Fatalf("expected exactly one UDP port, got %+v", protoSvc.Protocols.Udp)
	}
	if protoSvc.Protocols.Udp.Ports[0].D != "9090" {
		t.Fatalf("expected UDP port 9090, got %s", protoSvc.Protocols.Udp.Ports[0].D)
	}
}
