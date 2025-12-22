package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHost_MetaInfo(t *testing.T) {
	t.Run("SetMetaInfo sets the meta info", func(t *testing.T) {
		host := NewHost("test-host", "test-ns", "test-uuid")

		metaInfo := &HostMetaInfo{
			HostName:        "server1",
			Os:              "linux",
			Platform:        "ubuntu",
			PlatformFamily:  "debian",
			PlatformVersion: "22.04",
			KernelVersion:   "5.15.0",
		}

		host.SetMetaInfo(metaInfo)

		assert.NotNil(t, host.MetaInfo)
		assert.Equal(t, "server1", host.MetaInfo.HostName)
		assert.Equal(t, "linux", host.MetaInfo.Os)
		assert.Equal(t, "ubuntu", host.MetaInfo.Platform)
		assert.Equal(t, "debian", host.MetaInfo.PlatformFamily)
		assert.Equal(t, "22.04", host.MetaInfo.PlatformVersion)
		assert.Equal(t, "5.15.0", host.MetaInfo.KernelVersion)
	})

	t.Run("GetMetaInfo returns the meta info", func(t *testing.T) {
		host := NewHost("test-host", "test-ns", "test-uuid")

		metaInfo := &HostMetaInfo{
			HostName: "server1",
			Os:       "linux",
		}
		host.MetaInfo = metaInfo

		result := host.GetMetaInfo()
		assert.Equal(t, metaInfo, result)
	})

	t.Run("GetMetaInfo returns nil when not set", func(t *testing.T) {
		host := NewHost("test-host", "test-ns", "test-uuid")

		result := host.GetMetaInfo()
		assert.Nil(t, result)
	})

	t.Run("HasMetaInfo returns true when set", func(t *testing.T) {
		host := NewHost("test-host", "test-ns", "test-uuid")
		host.MetaInfo = &HostMetaInfo{HostName: "server1"}

		assert.True(t, host.HasMetaInfo())
	})

	t.Run("HasMetaInfo returns false when not set", func(t *testing.T) {
		host := NewHost("test-host", "test-ns", "test-uuid")

		assert.False(t, host.HasMetaInfo())
	})

	t.Run("ClearMetaInfo removes the meta info", func(t *testing.T) {
		host := NewHost("test-host", "test-ns", "test-uuid")
		host.MetaInfo = &HostMetaInfo{HostName: "server1"}

		host.ClearMetaInfo()

		assert.Nil(t, host.MetaInfo)
		assert.False(t, host.HasMetaInfo())
	})
}

func TestHost_DeepCopyWithMetaInfo(t *testing.T) {
	t.Run("DeepCopy copies MetaInfo correctly", func(t *testing.T) {
		host := NewHost("test-host", "test-ns", "test-uuid")
		host.MetaInfo = &HostMetaInfo{
			HostName:        "server1",
			Os:              "linux",
			Platform:        "ubuntu",
			PlatformFamily:  "debian",
			PlatformVersion: "22.04",
			KernelVersion:   "5.15.0",
		}

		copied := host.DeepCopy()
		require.NotNil(t, copied)

		copiedHost, ok := copied.(*Host)
		require.True(t, ok)

		// Verify MetaInfo is copied
		assert.NotNil(t, copiedHost.MetaInfo)
		assert.Equal(t, host.MetaInfo.HostName, copiedHost.MetaInfo.HostName)
		assert.Equal(t, host.MetaInfo.Os, copiedHost.MetaInfo.Os)
		assert.Equal(t, host.MetaInfo.Platform, copiedHost.MetaInfo.Platform)
		assert.Equal(t, host.MetaInfo.PlatformFamily, copiedHost.MetaInfo.PlatformFamily)
		assert.Equal(t, host.MetaInfo.PlatformVersion, copiedHost.MetaInfo.PlatformVersion)
		assert.Equal(t, host.MetaInfo.KernelVersion, copiedHost.MetaInfo.KernelVersion)

		// Verify it's a deep copy (modifying copy doesn't affect original)
		copiedHost.MetaInfo.HostName = "modified"
		assert.NotEqual(t, host.MetaInfo.HostName, copiedHost.MetaInfo.HostName)
	})

	t.Run("DeepCopy handles nil MetaInfo", func(t *testing.T) {
		host := NewHost("test-host", "test-ns", "test-uuid")
		// MetaInfo is nil

		copied := host.DeepCopy()
		require.NotNil(t, copied)

		copiedHost, ok := copied.(*Host)
		require.True(t, ok)

		assert.Nil(t, copiedHost.MetaInfo)
	})
}

func TestHost_IpListMethods(t *testing.T) {
	t.Run("SetIpList sets IPs from string slice", func(t *testing.T) {
		host := NewHost("test-host", "test-ns", "test-uuid")

		host.SetIpList([]string{"192.168.1.1", "10.0.0.1"})

		assert.Len(t, host.IpList, 2)
		assert.Equal(t, "192.168.1.1", host.IpList[0].IP)
		assert.Equal(t, "10.0.0.1", host.IpList[1].IP)
	})

	t.Run("GetIpList returns IPs as string slice", func(t *testing.T) {
		host := NewHost("test-host", "test-ns", "test-uuid")
		host.IpList = []IPItem{{IP: "192.168.1.1"}, {IP: "10.0.0.1"}}

		result := host.GetIpList()

		assert.Len(t, result, 2)
		assert.Contains(t, result, "192.168.1.1")
		assert.Contains(t, result, "10.0.0.1")
	})

	t.Run("HasIpList returns true when IPs exist", func(t *testing.T) {
		host := NewHost("test-host", "test-ns", "test-uuid")
		host.IpList = []IPItem{{IP: "192.168.1.1"}}

		assert.True(t, host.HasIpList())
	})

	t.Run("HasIpList returns false when empty", func(t *testing.T) {
		host := NewHost("test-host", "test-ns", "test-uuid")

		assert.False(t, host.HasIpList())
	})

	t.Run("AddIP adds an IP to the list", func(t *testing.T) {
		host := NewHost("test-host", "test-ns", "test-uuid")

		host.AddIP("192.168.1.1")
		host.AddIP("10.0.0.1")

		assert.Len(t, host.IpList, 2)
		assert.Equal(t, "192.168.1.1", host.IpList[0].IP)
		assert.Equal(t, "10.0.0.1", host.IpList[1].IP)
	})

	t.Run("ClearIpList removes all IPs", func(t *testing.T) {
		host := NewHost("test-host", "test-ns", "test-uuid")
		host.IpList = []IPItem{{IP: "192.168.1.1"}}

		host.ClearIpList()

		assert.Nil(t, host.IpList)
		assert.False(t, host.HasIpList())
	})
}

func TestHost_Key(t *testing.T) {
	t.Run("Key returns namespace/name format", func(t *testing.T) {
		host := NewHost("test-host", "test-ns", "test-uuid")

		assert.Equal(t, "test-ns/test-host", host.Key())
	})

	t.Run("Key returns just name when namespace is empty", func(t *testing.T) {
		host := NewHost("test-host", "", "test-uuid")

		assert.Equal(t, "test-host", host.Key())
	})
}

func TestHost_GetSyncKey(t *testing.T) {
	t.Run("GetSyncKey returns prefixed key with namespace", func(t *testing.T) {
		host := NewHost("test-host", "test-ns", "test-uuid")

		assert.Equal(t, "host-test-ns/test-host", host.GetSyncKey())
	})

	t.Run("GetSyncKey returns prefixed key without namespace", func(t *testing.T) {
		host := NewHost("test-host", "", "test-uuid")

		assert.Equal(t, "host-test-host", host.GetSyncKey())
	})
}
