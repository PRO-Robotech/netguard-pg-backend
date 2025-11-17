package netguard

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"

	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	watchpkg "netguard-pg-backend/internal/watch"
	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"
)

type fakeCacheProvider struct {
	caches map[string]*watchpkg.WatchCache
}

func (f *fakeCacheProvider) Cache(resourceType string) (*watchpkg.WatchCache, bool) {
	cache, ok := f.caches[resourceType]
	return cache, ok
}

func TestBuildListOptions(t *testing.T) {
	req := &netguardpb.WatchRequest{
		LabelSelector: "env=prod",
		FieldSelector: "metadata.name=demo",
		Namespace:     "default",
	}
	opts, err := buildListOptions(req)
	require.NoError(t, err)
	require.NotNil(t, opts)

	require.Equal(t, labels.Set{"env": "prod"}.AsSelector().String(), opts.LabelSelector.String())
	require.True(t, opts.FieldSelector.Matches(fields.Set{
		"metadata.namespace": "default",
		"metadata.name":      "demo",
	}))
}

func TestParseResourceVersion(t *testing.T) {
	rv, err := parseResourceVersion("")
	require.NoError(t, err)
	require.Zero(t, rv)

	rv, err = parseResourceVersion("42")
	require.NoError(t, err)
	require.Equal(t, int64(42), rv)

	_, err = parseResourceVersion("abc")
	require.Error(t, err)
}

func TestWatchHandlerStreamInitialEvents(t *testing.T) {
	cache := watchpkg.NewWatchCache("services", 16)
	svc := makeService("default", "demo", "1")
	require.NoError(t, cache.Add(svc, 1))

	provider := &fakeCacheProvider{
		caches: map[string]*watchpkg.WatchCache{
			"services": cache,
		},
	}

	handler := &WatchHandler{
		cacheProvider:    provider,
		encoders:         buildEncoders(),
		pollInterval:     time.Millisecond,
		bookmarkInterval: time.Hour,
		allowBookmarks:   false,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := &netguardpb.WatchRequest{
		Namespace:       "default",
		ResourceVersion: "0",
	}

	eventCh := make(chan *netguardpb.WatchEvent, 1)
	errCh := make(chan error, 1)

	go func() {
		errCh <- handler.Stream(ctx, "services", req, func(event *netguardpb.WatchEvent) error {
			eventCh <- event
			cancel()
			return nil
		})
	}()

	select {
	case event := <-eventCh:
		require.NotNil(t, event)
		require.Equal(t, netguardpb.WatchEventType_WATCH_EVENT_TYPE_ADDED, event.GetType())
		require.Equal(t, "demo", event.GetService().GetSelfRef().GetName())
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for watch event")
	}

	err := <-errCh
	require.ErrorIs(t, err, context.Canceled)
}

func makeService(namespace, name, rv string) *netguardv1beta1.Service {
	return &netguardv1beta1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			ResourceVersion: rv,
		},
	}
}
