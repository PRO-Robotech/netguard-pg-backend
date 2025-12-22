package converters

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"
)

func TestConvertNetworkFromProtoCarriesBindingRefs(t *testing.T) {
	pb := &netguardpb.Network{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      "net-a",
			Namespace: "ns",
		},
		Cidr:    "1.1.1.1/32",
		Meta:    &netguardpb.Meta{},
		IsBound: true,
		BindingRef: &netguardpb.NamespacedObjectReference{
			ApiVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "NetworkBinding",
			Name:       "binding-a",
			Namespace:  "ns",
		},
		AddressGroupRef: &netguardpb.NamespacedObjectReference{
			ApiVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "AddressGroup",
			Name:       "example",
			Namespace:  "ns",
		},
	}

	domain := ConvertNetwork(pb)

	require.True(t, domain.IsBound)
	require.NotNil(t, domain.BindingRef)
	require.Equal(t, "binding-a", domain.BindingRef.Name)
	require.NotNil(t, domain.AddressGroupRef)
	require.Equal(t, "example", domain.AddressGroupRef.Name)
}

func TestConvertNetworkToProtoCarriesBindingRefs(t *testing.T) {
	bindingRef := &netguardv1beta1.NamespacedObjectReference{
		ObjectReference: netguardv1beta1.ObjectReference{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "NetworkBinding",
			Name:       "binding-a",
		},
		Namespace: "ns",
	}
	agRef := &netguardv1beta1.NamespacedObjectReference{
		ObjectReference: netguardv1beta1.ObjectReference{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "AddressGroup",
			Name:       "example",
		},
		Namespace: "ns",
	}

	domain := models.Network{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.NewResourceIdentifier("net-a", models.WithNamespace("ns")),
		},
		CIDR: "1.1.1.1/32",
		Meta: models.Meta{
			CreationTS: metav1.Now(),
		},
		IsBound:         true,
		BindingRef:      bindingRef,
		AddressGroupRef: agRef,
	}

	pb := ConvertNetworkToPB(domain)

	require.True(t, pb.GetIsBound())
	require.NotNil(t, pb.GetBindingRef())
	require.Equal(t, "binding-a", pb.GetBindingRef().GetName())
	require.NotNil(t, pb.GetAddressGroupRef())
	require.Equal(t, "example", pb.GetAddressGroupRef().GetName())
}

func TestConvertNetwork_CommentField(t *testing.T) {
	tests := []struct {
		name     string
		comment  string
		expected string
	}{
		{
			name:     "network with comment",
			comment:  "Production network segment",
			expected: "Production network segment",
		},
		{
			name:     "network with empty comment",
			comment:  "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pb := &netguardpb.Network{
				SelfRef: &netguardpb.ResourceIdentifier{
					Name:      "net-a",
					Namespace: "ns",
				},
				Cidr:    "10.0.0.0/24",
				Comment: tt.comment,
			}

			result := ConvertNetwork(pb)

			require.Equal(t, tt.expected, result.Comment)
		})
	}
}

func TestConvertNetworkToPB_CommentField(t *testing.T) {
	tests := []struct {
		name     string
		comment  string
		expected string
	}{
		{
			name:     "domain network with comment",
			comment:  "Domain network comment",
			expected: "Domain network comment",
		},
		{
			name:     "domain network without comment",
			comment:  "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			domain := models.Network{
				SelfRef: models.SelfRef{
					ResourceIdentifier: models.NewResourceIdentifier("net-a", models.WithNamespace("ns")),
				},
				CIDR:    "10.0.0.0/24",
				Comment: tt.comment,
			}

			pb := ConvertNetworkToPB(domain)

			require.Equal(t, tt.expected, pb.GetComment())
		})
	}
}

func TestConvertNetwork_BidirectionalComment(t *testing.T) {
	originalComment := "Bidirectional network comment"
	pb := &netguardpb.Network{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      "net-a",
			Namespace: "ns",
		},
		Cidr:    "10.0.0.0/24",
		Comment: originalComment,
	}

	domain := ConvertNetwork(pb)
	backToProto := ConvertNetworkToPB(domain)

	require.Equal(t, originalComment, backToProto.GetComment())
}
