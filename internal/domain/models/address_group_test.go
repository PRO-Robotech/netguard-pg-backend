package models

import (
	"testing"

	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"

	pb "github.com/PRO-Robotech/protos/pkg/api/sgroups"
)

func TestAddressGroupToSGroupsProtoOmitsAggregatedHosts(t *testing.T) {
	ag := &AddressGroup{
		SelfRef: SelfRef{
			ResourceIdentifier: ResourceIdentifier{
				Name:      "host-bind-ag",
				Namespace: "incloud-sgroups",
			},
		},
		DefaultAction: ActionAccept,
		AggregatedHosts: []HostReference{
			{
				Ref: netguardv1beta1.NamespacedObjectReference{
					ObjectReference: netguardv1beta1.ObjectReference{
						APIVersion: "netguard.sgroups.io/v1beta1",
						Kind:       "Host",
						Name:       "host-bind-host",
					},
					Namespace: "incloud-sgroups",
				},
				UUID:   "550e8400-e29b-41d4-a716-4466554400ab",
				Source: HostSourceBinding,
			},
		},
	}

	protoData, err := ag.ToSGroupsProto()
	if err != nil {
		t.Fatalf("ToSGroupsProto returned error: %v", err)
	}

	sg, ok := protoData.(*pb.SecGroup)
	if !ok {
		t.Fatalf("expected *pb.SecGroup, got %T", protoData)
	}

	if len(sg.Networks) != 0 {
		t.Fatalf("expected Networks to be empty, got %v", sg.Networks)
	}

	if sg.Hosts != nil {
		t.Fatalf("expected Hosts field to be nil, got %v", sg.Hosts)
	}
}

func TestAddressGroupToSGroupsProtoClearsNetworksWithSentinel(t *testing.T) {
	ag := &AddressGroup{
		SelfRef: SelfRef{
			ResourceIdentifier: ResourceIdentifier{
				Name:      "host-bind-ag",
				Namespace: "incloud-sgroups",
			},
		},
		DefaultAction: ActionAccept,
	}

	protoData, err := ag.ToSGroupsProto()
	if err != nil {
		t.Fatalf("ToSGroupsProto returned error: %v", err)
	}

	sg, ok := protoData.(*pb.SecGroup)
	if !ok {
		t.Fatalf("expected *pb.SecGroup, got %T", protoData)
	}

	if len(sg.Networks) != 0 {
		t.Fatalf("expected Networks to be empty, got %v", sg.Networks)
	}

	if sg.Hosts != nil {
		t.Fatalf("expected Hosts field to be nil, got %v", sg.Hosts)
	}
}

func TestAddressGroupToSGroupsProtoIncludesNetworksWhenPresent(t *testing.T) {
	ag := &AddressGroup{
		SelfRef: SelfRef{
			ResourceIdentifier: ResourceIdentifier{
				Name:      "net-ag",
				Namespace: "incloud-sgroups",
			},
		},
		DefaultAction: ActionAccept,
		Networks: []NetworkItem{
			{
				Name:      "net-1",
				Namespace: "incloud-sgroups",
				CIDR:      "10.0.0.0/24",
			},
		},
	}

	protoData, err := ag.ToSGroupsProto()
	if err != nil {
		t.Fatalf("ToSGroupsProto returned error: %v", err)
	}

	sg, ok := protoData.(*pb.SecGroup)
	if !ok {
		t.Fatalf("expected *pb.SecGroup, got %T", protoData)
	}

	if len(sg.Networks) != 1 {
		t.Fatalf("expected Networks length 1, got %d", len(sg.Networks))
	}

	if sg.Networks[0] != "incloud-sgroups/net-1" {
		t.Fatalf("expected Networks[0] to be incloud-sgroups/net-1, got %s", sg.Networks[0])
	}
}
