package converters

import (
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"
)

// ConvertHost converts protobuf Host to domain model
func ConvertHost(protoHost *netguardpb.Host) models.Host {
	host := models.Host{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      protoHost.SelfRef.Name,
				Namespace: protoHost.SelfRef.Namespace,
			},
		},
		UUID: protoHost.Uuid,

		// Status fields
		HostName:         protoHost.HostNameSync,
		AddressGroupName: protoHost.AddressGroupName,
		IsBound:          protoHost.IsBound,
		Meta:             ConvertMeta(protoHost.Meta),
	}

	// Set binding reference if present
	if protoHost.BindingRef != nil {
		host.BindingRef = &v1beta1.ObjectReference{
			APIVersion: protoHost.BindingRef.ApiVersion,
			Kind:       protoHost.BindingRef.Kind,
			Name:       protoHost.BindingRef.Name,
		}
	}

	// Set address group reference if present
	if protoHost.AddressGroupRef != nil {
		host.AddressGroupRef = &v1beta1.ObjectReference{
			APIVersion: protoHost.AddressGroupRef.ApiVersion,
			Kind:       protoHost.AddressGroupRef.Kind,
			Name:       protoHost.AddressGroupRef.Name,
		}
	}

	// Convert IP list if present
	if len(protoHost.IpList) > 0 {
		host.IpList = make([]models.IPItem, len(protoHost.IpList))
		for i, ipItem := range protoHost.IpList {
			host.IpList[i] = models.IPItem{
				IP: ipItem.Ip,
			}
		}
	}

	return host
}

// ConvertHostToPB converts domain Host to protobuf
func ConvertHostToPB(host models.Host) *netguardpb.Host {
	pbHost := &netguardpb.Host{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      host.Name,
			Namespace: host.Namespace,
		},
		Uuid: host.UUID,

		// Status fields
		HostNameSync:     host.HostName,
		AddressGroupName: host.AddressGroupName,
		IsBound:          host.IsBound,
		Meta:             ConvertMetaToPB(host.Meta),
	}

	// Convert binding reference if present
	if host.BindingRef != nil {
		pbHost.BindingRef = &netguardpb.ObjectReference{
			ApiVersion: host.BindingRef.APIVersion,
			Kind:       host.BindingRef.Kind,
			Name:       host.BindingRef.Name,
		}
	}

	// Convert address group reference if present
	if host.AddressGroupRef != nil {
		pbHost.AddressGroupRef = &netguardpb.ObjectReference{
			ApiVersion: host.AddressGroupRef.APIVersion,
			Kind:       host.AddressGroupRef.Kind,
			Name:       host.AddressGroupRef.Name,
		}
	}

	// Convert IP list if present
	if len(host.IpList) > 0 {
		pbHost.IpList = make([]*netguardpb.IPItem, len(host.IpList))
		for i, ipItem := range host.IpList {
			pbHost.IpList[i] = &netguardpb.IPItem{
				Ip: ipItem.IP,
			}
		}
	}

	return pbHost
}

// ConvertHostBinding converts protobuf HostBinding to domain model
func ConvertHostBinding(protoBinding *netguardpb.HostBinding) models.HostBinding {
	binding := models.HostBinding{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      protoBinding.SelfRef.Name,
				Namespace: protoBinding.SelfRef.Namespace,
			},
		},
		Meta: ConvertMeta(protoBinding.Meta),
	}

	// Set host reference
	if protoBinding.HostRef != nil {
		binding.HostRef = v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: protoBinding.HostRef.ApiVersion,
				Kind:       protoBinding.HostRef.Kind,
				Name:       protoBinding.HostRef.Name,
			},
			Namespace: protoBinding.HostRef.Namespace,
		}
	}

	// Set address group reference
	if protoBinding.AddressGroupRef != nil {
		binding.AddressGroupRef = v1beta1.NamespacedObjectReference{
			ObjectReference: v1beta1.ObjectReference{
				APIVersion: protoBinding.AddressGroupRef.ApiVersion,
				Kind:       protoBinding.AddressGroupRef.Kind,
				Name:       protoBinding.AddressGroupRef.Name,
			},
			Namespace: protoBinding.AddressGroupRef.Namespace,
		}
	}

	return binding
}

// ConvertHostBindingToPB converts domain HostBinding to protobuf
func ConvertHostBindingToPB(binding models.HostBinding) *netguardpb.HostBinding {
	return &netguardpb.HostBinding{
		SelfRef: &netguardpb.ResourceIdentifier{
			Name:      binding.Name,
			Namespace: binding.Namespace,
		},

		HostRef: &netguardpb.NamespacedObjectReference{
			ApiVersion: binding.HostRef.APIVersion,
			Kind:       binding.HostRef.Kind,
			Name:       binding.HostRef.Name,
			Namespace:  binding.HostRef.Namespace,
		},

		AddressGroupRef: &netguardpb.NamespacedObjectReference{
			ApiVersion: binding.AddressGroupRef.APIVersion,
			Kind:       binding.AddressGroupRef.Kind,
			Name:       binding.AddressGroupRef.Name,
			Namespace:  binding.AddressGroupRef.Namespace,
		},

		Meta: ConvertMetaToPB(binding.Meta),
	}
}
