package converters

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"
)

func TestConvertHost(t *testing.T) {
	t.Run("minimal host", func(t *testing.T) {
		pb := &netguardpb.Host{
			SelfRef: &netguardpb.ResourceIdentifier{
				Name:      "test-host",
				Namespace: "default",
			},
			Uuid: "test-uuid-123",
		}

		result := ConvertHost(pb)

		assert.Equal(t, "test-host", result.Name)
		assert.Equal(t, "default", result.Namespace)
		assert.Equal(t, "test-uuid-123", result.UUID)
		assert.Nil(t, result.MetaInfo)
		assert.Nil(t, result.IpList)
	})

	t.Run("host with status fields", func(t *testing.T) {
		pb := &netguardpb.Host{
			SelfRef: &netguardpb.ResourceIdentifier{
				Name:      "test-host",
				Namespace: "default",
			},
			Uuid:             "test-uuid",
			HostNameSync:     "server.example.com",
			AddressGroupName: "ag-test",
			IsBound:          true,
		}

		result := ConvertHost(pb)

		assert.Equal(t, "server.example.com", result.HostName)
		assert.Equal(t, "ag-test", result.AddressGroupName)
		assert.True(t, result.IsBound)
	})

	t.Run("host with binding references", func(t *testing.T) {
		pb := &netguardpb.Host{
			SelfRef: &netguardpb.ResourceIdentifier{
				Name:      "test-host",
				Namespace: "default",
			},
			Uuid: "test-uuid",
			BindingRef: &netguardpb.NamespacedObjectReference{
				ApiVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "HostBinding",
				Name:       "test-binding",
				Namespace:  "default",
			},
			AddressGroupRef: &netguardpb.NamespacedObjectReference{
				ApiVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "AddressGroup",
				Name:       "ag-test",
				Namespace:  "default",
			},
		}

		result := ConvertHost(pb)

		require.NotNil(t, result.BindingRef)
		assert.Equal(t, "netguard.sgroups.io/v1beta1", result.BindingRef.APIVersion)
		assert.Equal(t, "HostBinding", result.BindingRef.Kind)
		assert.Equal(t, "test-binding", result.BindingRef.Name)
		assert.Equal(t, "default", result.BindingRef.Namespace)

		require.NotNil(t, result.AddressGroupRef)
		assert.Equal(t, "netguard.sgroups.io/v1beta1", result.AddressGroupRef.APIVersion)
		assert.Equal(t, "AddressGroup", result.AddressGroupRef.Kind)
		assert.Equal(t, "ag-test", result.AddressGroupRef.Name)
		assert.Equal(t, "default", result.AddressGroupRef.Namespace)
	})

	t.Run("host with IP list", func(t *testing.T) {
		pb := &netguardpb.Host{
			SelfRef: &netguardpb.ResourceIdentifier{
				Name:      "test-host",
				Namespace: "default",
			},
			Uuid: "test-uuid",
			IpList: []*netguardpb.IPItem{
				{Ip: "192.168.1.1"},
				{Ip: "10.0.0.1"},
				{Ip: "172.16.0.1"},
			},
		}

		result := ConvertHost(pb)

		require.Len(t, result.IpList, 3)
		assert.Equal(t, "192.168.1.1", result.IpList[0].IP)
		assert.Equal(t, "10.0.0.1", result.IpList[1].IP)
		assert.Equal(t, "172.16.0.1", result.IpList[2].IP)
	})

	t.Run("host with MetaInfo", func(t *testing.T) {
		pb := &netguardpb.Host{
			SelfRef: &netguardpb.ResourceIdentifier{
				Name:      "test-host",
				Namespace: "default",
			},
			Uuid: "test-uuid",
			MetaInfo: &netguardpb.HostMetaInfo{
				HostName:        "prod-server-01",
				Os:              "linux",
				Platform:        "ubuntu",
				PlatformFamily:  "debian",
				PlatformVersion: "22.04",
				KernelVersion:   "5.15.0-generic",
			},
		}

		result := ConvertHost(pb)

		require.NotNil(t, result.MetaInfo)
		assert.Equal(t, "prod-server-01", result.MetaInfo.HostName)
		assert.Equal(t, "linux", result.MetaInfo.Os)
		assert.Equal(t, "ubuntu", result.MetaInfo.Platform)
		assert.Equal(t, "debian", result.MetaInfo.PlatformFamily)
		assert.Equal(t, "22.04", result.MetaInfo.PlatformVersion)
		assert.Equal(t, "5.15.0-generic", result.MetaInfo.KernelVersion)
	})

	t.Run("host with IP list and MetaInfo", func(t *testing.T) {
		pb := &netguardpb.Host{
			SelfRef: &netguardpb.ResourceIdentifier{
				Name:      "test-host",
				Namespace: "default",
			},
			Uuid: "test-uuid",
			IpList: []*netguardpb.IPItem{
				{Ip: "10.10.10.10"},
			},
			MetaInfo: &netguardpb.HostMetaInfo{
				HostName: "combined-server",
				Os:       "windows",
			},
		}

		result := ConvertHost(pb)

		require.Len(t, result.IpList, 1)
		assert.Equal(t, "10.10.10.10", result.IpList[0].IP)

		require.NotNil(t, result.MetaInfo)
		assert.Equal(t, "combined-server", result.MetaInfo.HostName)
		assert.Equal(t, "windows", result.MetaInfo.Os)
	})

	t.Run("host with partial MetaInfo", func(t *testing.T) {
		pb := &netguardpb.Host{
			SelfRef: &netguardpb.ResourceIdentifier{
				Name:      "test-host",
				Namespace: "default",
			},
			Uuid: "test-uuid",
			MetaInfo: &netguardpb.HostMetaInfo{
				HostName: "only-hostname",
				// Other fields are empty
			},
		}

		result := ConvertHost(pb)

		require.NotNil(t, result.MetaInfo)
		assert.Equal(t, "only-hostname", result.MetaInfo.HostName)
		assert.Empty(t, result.MetaInfo.Os)
		assert.Empty(t, result.MetaInfo.Platform)
		assert.Empty(t, result.MetaInfo.PlatformFamily)
		assert.Empty(t, result.MetaInfo.PlatformVersion)
		assert.Empty(t, result.MetaInfo.KernelVersion)
	})
}

func TestConvertHostToPB(t *testing.T) {
	t.Run("minimal host", func(t *testing.T) {
		model := models.Host{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{
					Name:      "test-host",
					Namespace: "default",
				},
			},
			UUID: "test-uuid-123",
		}

		result := ConvertHostToPB(model)

		require.NotNil(t, result.SelfRef)
		assert.Equal(t, "test-host", result.SelfRef.Name)
		assert.Equal(t, "default", result.SelfRef.Namespace)
		assert.Equal(t, "test-uuid-123", result.Uuid)
		assert.Nil(t, result.MetaInfo)
		assert.Nil(t, result.IpList)
	})

	t.Run("host with status fields", func(t *testing.T) {
		model := models.Host{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{
					Name:      "test-host",
					Namespace: "default",
				},
			},
			UUID:             "test-uuid",
			HostName:         "server.example.com",
			AddressGroupName: "ag-test",
			IsBound:          true,
		}

		result := ConvertHostToPB(model)

		assert.Equal(t, "server.example.com", result.HostNameSync)
		assert.Equal(t, "ag-test", result.AddressGroupName)
		assert.True(t, result.IsBound)
	})

	t.Run("host with binding references", func(t *testing.T) {
		model := models.Host{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{
					Name:      "test-host",
					Namespace: "default",
				},
			},
			UUID: "test-uuid",
			BindingRef: &v1beta1.NamespacedObjectReference{
				ObjectReference: v1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "HostBinding",
					Name:       "test-binding",
				},
				Namespace: "default",
			},
			AddressGroupRef: &v1beta1.NamespacedObjectReference{
				ObjectReference: v1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "AddressGroup",
					Name:       "ag-test",
				},
				Namespace: "default",
			},
		}

		result := ConvertHostToPB(model)

		require.NotNil(t, result.BindingRef)
		assert.Equal(t, "netguard.sgroups.io/v1beta1", result.BindingRef.ApiVersion)
		assert.Equal(t, "HostBinding", result.BindingRef.Kind)
		assert.Equal(t, "test-binding", result.BindingRef.Name)
		assert.Equal(t, "default", result.BindingRef.Namespace)

		require.NotNil(t, result.AddressGroupRef)
		assert.Equal(t, "netguard.sgroups.io/v1beta1", result.AddressGroupRef.ApiVersion)
		assert.Equal(t, "AddressGroup", result.AddressGroupRef.Kind)
		assert.Equal(t, "ag-test", result.AddressGroupRef.Name)
		assert.Equal(t, "default", result.AddressGroupRef.Namespace)
	})

	t.Run("host with IP list", func(t *testing.T) {
		model := models.Host{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{
					Name:      "test-host",
					Namespace: "default",
				},
			},
			UUID: "test-uuid",
			IpList: []models.IPItem{
				{IP: "192.168.1.1"},
				{IP: "10.0.0.1"},
				{IP: "172.16.0.1"},
			},
		}

		result := ConvertHostToPB(model)

		require.Len(t, result.IpList, 3)
		assert.Equal(t, "192.168.1.1", result.IpList[0].Ip)
		assert.Equal(t, "10.0.0.1", result.IpList[1].Ip)
		assert.Equal(t, "172.16.0.1", result.IpList[2].Ip)
	})

	t.Run("host with MetaInfo", func(t *testing.T) {
		model := models.Host{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{
					Name:      "test-host",
					Namespace: "default",
				},
			},
			UUID: "test-uuid",
			MetaInfo: &models.HostMetaInfo{
				HostName:        "prod-server-01",
				Os:              "linux",
				Platform:        "ubuntu",
				PlatformFamily:  "debian",
				PlatformVersion: "22.04",
				KernelVersion:   "5.15.0-generic",
			},
		}

		result := ConvertHostToPB(model)

		require.NotNil(t, result.MetaInfo)
		assert.Equal(t, "prod-server-01", result.MetaInfo.HostName)
		assert.Equal(t, "linux", result.MetaInfo.Os)
		assert.Equal(t, "ubuntu", result.MetaInfo.Platform)
		assert.Equal(t, "debian", result.MetaInfo.PlatformFamily)
		assert.Equal(t, "22.04", result.MetaInfo.PlatformVersion)
		assert.Equal(t, "5.15.0-generic", result.MetaInfo.KernelVersion)
	})

	t.Run("host with IP list and MetaInfo", func(t *testing.T) {
		model := models.Host{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{
					Name:      "test-host",
					Namespace: "default",
				},
			},
			UUID: "test-uuid",
			IpList: []models.IPItem{
				{IP: "10.10.10.10"},
			},
			MetaInfo: &models.HostMetaInfo{
				HostName: "combined-server",
				Os:       "windows",
			},
		}

		result := ConvertHostToPB(model)

		require.Len(t, result.IpList, 1)
		assert.Equal(t, "10.10.10.10", result.IpList[0].Ip)

		require.NotNil(t, result.MetaInfo)
		assert.Equal(t, "combined-server", result.MetaInfo.HostName)
		assert.Equal(t, "windows", result.MetaInfo.Os)
	})

	t.Run("host with nil MetaInfo", func(t *testing.T) {
		model := models.Host{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{
					Name:      "test-host",
					Namespace: "default",
				},
			},
			UUID:     "test-uuid",
			MetaInfo: nil,
		}

		result := ConvertHostToPB(model)

		assert.Nil(t, result.MetaInfo)
	})
}

func TestConvertHostRoundTrip(t *testing.T) {
	t.Run("round trip preserves all fields", func(t *testing.T) {
		original := &netguardpb.Host{
			SelfRef: &netguardpb.ResourceIdentifier{
				Name:      "test-host",
				Namespace: "production",
			},
			Uuid:             "round-trip-uuid",
			HostNameSync:     "prod-server.example.com",
			AddressGroupName: "ag-prod",
			IsBound:          true,
			BindingRef: &netguardpb.NamespacedObjectReference{
				ApiVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "HostBinding",
				Name:       "binding-prod",
				Namespace:  "production",
			},
			AddressGroupRef: &netguardpb.NamespacedObjectReference{
				ApiVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "AddressGroup",
				Name:       "ag-prod",
				Namespace:  "production",
			},
			IpList: []*netguardpb.IPItem{
				{Ip: "10.0.0.1"},
				{Ip: "10.0.0.2"},
			},
			MetaInfo: &netguardpb.HostMetaInfo{
				HostName:        "server-hostname",
				Os:              "linux",
				Platform:        "centos",
				PlatformFamily:  "rhel",
				PlatformVersion: "8.5",
				KernelVersion:   "4.18.0",
			},
		}

		// Proto -> Domain
		domain := ConvertHost(original)

		// Domain -> Proto
		result := ConvertHostToPB(domain)

		// Compare
		assert.Equal(t, original.SelfRef.Name, result.SelfRef.Name)
		assert.Equal(t, original.SelfRef.Namespace, result.SelfRef.Namespace)
		assert.Equal(t, original.Uuid, result.Uuid)
		assert.Equal(t, original.HostNameSync, result.HostNameSync)
		assert.Equal(t, original.AddressGroupName, result.AddressGroupName)
		assert.Equal(t, original.IsBound, result.IsBound)

		require.NotNil(t, result.BindingRef)
		assert.Equal(t, original.BindingRef.ApiVersion, result.BindingRef.ApiVersion)
		assert.Equal(t, original.BindingRef.Kind, result.BindingRef.Kind)
		assert.Equal(t, original.BindingRef.Name, result.BindingRef.Name)
		assert.Equal(t, original.BindingRef.Namespace, result.BindingRef.Namespace)

		require.NotNil(t, result.AddressGroupRef)
		assert.Equal(t, original.AddressGroupRef.ApiVersion, result.AddressGroupRef.ApiVersion)
		assert.Equal(t, original.AddressGroupRef.Kind, result.AddressGroupRef.Kind)
		assert.Equal(t, original.AddressGroupRef.Name, result.AddressGroupRef.Name)
		assert.Equal(t, original.AddressGroupRef.Namespace, result.AddressGroupRef.Namespace)

		require.Len(t, result.IpList, 2)
		assert.Equal(t, original.IpList[0].Ip, result.IpList[0].Ip)
		assert.Equal(t, original.IpList[1].Ip, result.IpList[1].Ip)

		require.NotNil(t, result.MetaInfo)
		assert.Equal(t, original.MetaInfo.HostName, result.MetaInfo.HostName)
		assert.Equal(t, original.MetaInfo.Os, result.MetaInfo.Os)
		assert.Equal(t, original.MetaInfo.Platform, result.MetaInfo.Platform)
		assert.Equal(t, original.MetaInfo.PlatformFamily, result.MetaInfo.PlatformFamily)
		assert.Equal(t, original.MetaInfo.PlatformVersion, result.MetaInfo.PlatformVersion)
		assert.Equal(t, original.MetaInfo.KernelVersion, result.MetaInfo.KernelVersion)
	})

	t.Run("round trip with nil MetaInfo", func(t *testing.T) {
		original := &netguardpb.Host{
			SelfRef: &netguardpb.ResourceIdentifier{
				Name:      "test-host",
				Namespace: "default",
			},
			Uuid:     "no-meta-uuid",
			MetaInfo: nil,
		}

		domain := ConvertHost(original)
		result := ConvertHostToPB(domain)

		assert.Nil(t, result.MetaInfo)
	})
}

func TestConvertHostBinding(t *testing.T) {
	t.Run("basic binding", func(t *testing.T) {
		pb := &netguardpb.HostBinding{
			SelfRef: &netguardpb.ResourceIdentifier{
				Name:      "test-binding",
				Namespace: "default",
			},
			HostRef: &netguardpb.NamespacedObjectReference{
				ApiVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "Host",
				Name:       "test-host",
				Namespace:  "default",
			},
			AddressGroupRef: &netguardpb.NamespacedObjectReference{
				ApiVersion: "netguard.sgroups.io/v1beta1",
				Kind:       "AddressGroup",
				Name:       "ag-test",
				Namespace:  "default",
			},
		}

		result := ConvertHostBinding(pb)

		assert.Equal(t, "test-binding", result.Name)
		assert.Equal(t, "default", result.Namespace)

		assert.Equal(t, "netguard.sgroups.io/v1beta1", result.HostRef.APIVersion)
		assert.Equal(t, "Host", result.HostRef.Kind)
		assert.Equal(t, "test-host", result.HostRef.Name)
		assert.Equal(t, "default", result.HostRef.Namespace)

		assert.Equal(t, "netguard.sgroups.io/v1beta1", result.AddressGroupRef.APIVersion)
		assert.Equal(t, "AddressGroup", result.AddressGroupRef.Kind)
		assert.Equal(t, "ag-test", result.AddressGroupRef.Name)
		assert.Equal(t, "default", result.AddressGroupRef.Namespace)
	})
}

func TestConvertHostBindingToPB(t *testing.T) {
	t.Run("basic binding", func(t *testing.T) {
		model := models.HostBinding{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{
					Name:      "test-binding",
					Namespace: "default",
				},
			},
			HostRef: v1beta1.NamespacedObjectReference{
				ObjectReference: v1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "Host",
					Name:       "test-host",
				},
				Namespace: "default",
			},
			AddressGroupRef: v1beta1.NamespacedObjectReference{
				ObjectReference: v1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "AddressGroup",
					Name:       "ag-test",
				},
				Namespace: "default",
			},
		}

		result := ConvertHostBindingToPB(model)

		require.NotNil(t, result.SelfRef)
		assert.Equal(t, "test-binding", result.SelfRef.Name)
		assert.Equal(t, "default", result.SelfRef.Namespace)

		require.NotNil(t, result.HostRef)
		assert.Equal(t, "netguard.sgroups.io/v1beta1", result.HostRef.ApiVersion)
		assert.Equal(t, "Host", result.HostRef.Kind)
		assert.Equal(t, "test-host", result.HostRef.Name)
		assert.Equal(t, "default", result.HostRef.Namespace)

		require.NotNil(t, result.AddressGroupRef)
		assert.Equal(t, "netguard.sgroups.io/v1beta1", result.AddressGroupRef.ApiVersion)
		assert.Equal(t, "AddressGroup", result.AddressGroupRef.Kind)
		assert.Equal(t, "ag-test", result.AddressGroupRef.Name)
		assert.Equal(t, "default", result.AddressGroupRef.Namespace)
	})
}
