package converters

import (
	"netguard-pg-backend/internal/domain/models"
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
	var networkName string
	if netRef := binding.GetNetworkRef(); netRef != nil {
		networkName = netRef.GetName()
	}
	if networkName == "" {
		return result // Skip conversion if NetworkRef is incomplete
	}
	result.NetworkRef = NewObjectReference(KindNetwork, networkName)

	// Convert AddressGroupRef with nil-safe access
	var agName string
	if agRef := binding.GetAddressGroupRef(); agRef != nil {
		agName = agRef.GetName()
	}
	if agName == "" {
		return result // Skip conversion if AddressGroupRef is incomplete
	}
	result.AddressGroupRef = NewObjectReference(KindAddressGroup, agName)

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
		NetworkRef: &netguardpb.ObjectReference{
			ApiVersion: binding.NetworkRef.APIVersion,
			Kind:       binding.NetworkRef.Kind,
			Name:       binding.NetworkRef.Name,
		},
		AddressGroupRef: &netguardpb.ObjectReference{
			ApiVersion: binding.AddressGroupRef.APIVersion,
			Kind:       binding.AddressGroupRef.Kind,
			Name:       binding.AddressGroupRef.Name,
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
