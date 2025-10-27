package converters

import (
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"
)

// ConvertNetwork converts protobuf Network to domain model
func ConvertNetwork(network *netguardpb.Network) models.Network {
	result := models.Network{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      network.GetSelfRef().GetName(),
				Namespace: network.GetSelfRef().GetNamespace(),
			},
		},
		CIDR: network.Cidr,
		Meta: ConvertMeta(network.Meta),
	}

	return result
}

// ConvertNetworkToPB converts domain Network to protobuf
func ConvertNetworkToPB(network models.Network) *netguardpb.Network {
	pbNetwork := &netguardpb.Network{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      network.Name,
			Namespace: network.Namespace,
		},
		Cidr: network.CIDR,
		Meta: ConvertMetaToPB(network.Meta),
	}

	// Add status fields
	pbNetwork.IsBound = network.IsBound

	if network.BindingRef != nil {
		pbNetwork.BindingRef = &netguardpb.ObjectReference{
			ApiVersion: network.BindingRef.APIVersion,
			Kind:       network.BindingRef.Kind,
			Name:       network.BindingRef.Name,
		}
	}

	if network.AddressGroupRef != nil {
		pbNetwork.AddressGroupRef = &netguardpb.NamespacedObjectReference{
			ApiVersion: network.AddressGroupRef.APIVersion,
			Kind:       network.AddressGroupRef.Kind,
			Name:       network.AddressGroupRef.Name,
			Namespace:  network.Namespace,
		}
	}

	return pbNetwork
}

// ConvertNetworkBinding converts protobuf NetworkBinding to domain model
func ConvertNetworkBinding(binding *netguardpb.NetworkBinding) models.NetworkBinding {
	result := models.NetworkBinding{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      binding.GetSelfRef().GetName(),
				Namespace: binding.GetSelfRef().GetNamespace(),
			},
		},
		Meta: ConvertMeta(binding.Meta),
	}

	// Convert NetworkRef with nil-safe access
	if netRef := binding.GetNetworkRef(); netRef != nil && netRef.GetName() != "" {
		result.NetworkRef = v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: netRef.GetApiVersion(),
				Kind:       netRef.GetKind(),
				Name:       netRef.GetName(),
			},
			Namespace: netRef.GetNamespace(),
		}
	}

	// Convert AddressGroupRef with nil-safe access
	if agRef := binding.GetAddressGroupRef(); agRef != nil && agRef.GetName() != "" {
		result.AddressGroupRef = v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: agRef.GetApiVersion(),
				Kind:       agRef.GetKind(),
				Name:       agRef.GetName(),
			},
			Namespace: agRef.GetNamespace(),
		}
	}

	if binding.NetworkItem != nil {
		result.NetworkItem = models.NetworkItem{
			Name: binding.NetworkItem.Name,
			CIDR: binding.NetworkItem.Cidr,
		}
	}

	return result
}

// ConvertNetworkBindingToPB converts domain NetworkBinding to protobuf
func ConvertNetworkBindingToPB(binding models.NetworkBinding) *netguardpb.NetworkBinding {
	pbBinding := &netguardpb.NetworkBinding{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      binding.Name,
			Namespace: binding.Namespace,
		},
		NetworkRef: &netguardpb.NamespacedObjectReference{
			ApiVersion: binding.NetworkRef.APIVersion,
			Kind:       binding.NetworkRef.Kind,
			Name:       binding.NetworkRef.Name,
			Namespace:  binding.NetworkRef.Namespace,
		},
		AddressGroupRef: &netguardpb.NamespacedObjectReference{
			ApiVersion: binding.AddressGroupRef.APIVersion,
			Kind:       binding.AddressGroupRef.Kind,
			Name:       binding.AddressGroupRef.Name,
			Namespace:  binding.AddressGroupRef.Namespace,
		},
		Meta: ConvertMetaToPB(binding.Meta),
	}

	// Convert NetworkItem
	pbBinding.NetworkItem = &netguardpb.NetworkItem{
		Name: binding.NetworkItem.Name,
		Cidr: binding.NetworkItem.CIDR,
	}

	return pbBinding
}
