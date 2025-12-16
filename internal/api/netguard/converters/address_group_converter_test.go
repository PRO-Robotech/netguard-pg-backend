package converters

import (
	"testing"

	"github.com/stretchr/testify/require"

	"netguard-pg-backend/internal/domain/models"
	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"
)

func TestConvertAddressGroupNetworksFromProto(t *testing.T) {
	pb := &netguardpb.AddressGroup{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      "example",
			Namespace: "ns",
		},
		DefaultAction:    netguardpb.RuleAction_DROP,
		AddressGroupName: "ns/example",
		Networks: []*netguardpb.NetworkItem{
			{
				Name:      "net-a",
				Cidr:      "10.0.0.0/32",
				Namespace: "ns",
			},
		},
	}

	result := ConvertAddressGroup(pb)

	require.Len(t, result.Networks, 1, "networks must be carried over from proto")
	require.Equal(t, "net-a", result.Networks[0].Name)
	require.Equal(t, "10.0.0.0/32", result.Networks[0].CIDR)
	require.Equal(t, "Network", result.Networks[0].Kind, "empty kind should default to Network")
	require.Equal(t, "netguard.sgroups.io/v1beta1", result.Networks[0].ApiVersion, "empty apiVersion should default")
	require.Equal(t, "ns/example", result.AddressGroupName)
}

func TestConvertAddressGroupToProtoNetworks(t *testing.T) {
	model := models.AddressGroup{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier(
				"example",
				models.WithNamespace("ns"),
			),
		},
		DefaultAction:    models.ActionDrop,
		AddressGroupName: "ns/example",
		Networks: []models.NetworkItem{
			{
				Name:       "net-a",
				CIDR:       "10.0.0.0/32",
				Kind:       "Network",
				ApiVersion: "netguard.sgroups.io/v1beta1",
				Namespace:  "ns",
			},
		},
	}

	pb := ConvertAddressGroupToPB(model)

	require.Len(t, pb.Networks, 1, "networks must be populated in proto")
	require.Equal(t, "net-a", pb.Networks[0].Name)
	require.Equal(t, "10.0.0.0/32", pb.Networks[0].Cidr)
	require.Equal(t, "netguard.sgroups.io/v1beta1", pb.Networks[0].ApiVersion)
	require.Equal(t, "Network", pb.Networks[0].Kind)
	require.Equal(t, "ns/example", pb.AddressGroupName)
}

func TestConvertAddressGroup_CommentField(t *testing.T) {
	tests := []struct {
		name     string
		comment  string
		expected string
	}{
		{
			name:     "address group with comment",
			comment:  "Production servers group",
			expected: "Production servers group",
		},
		{
			name:     "address group with empty comment",
			comment:  "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pb := &netguardpb.AddressGroup{
				SelfRef: &netguardpb.ResourceIdentifier{
					Name:      "example",
					Namespace: "ns",
				},
				Comment: tt.comment,
			}

			result := ConvertAddressGroup(pb)

			require.Equal(t, tt.expected, result.Comment)
		})
	}
}

func TestConvertAddressGroupToPB_CommentField(t *testing.T) {
	tests := []struct {
		name     string
		comment  string
		expected string
	}{
		{
			name:     "domain address group with comment",
			comment:  "Domain comment",
			expected: "Domain comment",
		},
		{
			name:     "domain address group without comment",
			comment:  "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := models.AddressGroup{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier(
						"example",
						models.WithNamespace("ns"),
					),
				},
				Comment: tt.comment,
			}

			pb := ConvertAddressGroupToPB(model)

			require.Equal(t, tt.expected, pb.GetComment())
		})
	}
}

func TestConvertAddressGroup_BidirectionalComment(t *testing.T) {
	originalComment := "Bidirectional AG comment"
	pb := &netguardpb.AddressGroup{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      "example",
			Namespace: "ns",
		},
		Comment: originalComment,
	}

	domain := ConvertAddressGroup(pb)
	backToProto := ConvertAddressGroupToPB(domain)

	require.Equal(t, originalComment, backToProto.GetComment())
}
