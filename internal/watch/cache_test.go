package watch

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"

	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

func TestWatchCacheAddUpdateDelete(t *testing.T) {
	cache := NewWatchCache("services", 16)

	makeService := func(name, rv string) *netguardv1beta1.Service {
		return &netguardv1beta1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:            name,
				Namespace:       "default",
				ResourceVersion: rv,
			},
		}
	}

	added := makeService("svc-1", "1")
	require.NoError(t, cache.Add(added, 1))

	events, _, err := cache.GetEventsSince(context.Background(), 0, nil)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, watch.Added, events[0].Type)
	require.Equal(t, int64(1), events[0].ResourceVersion)

	modified := makeService("svc-1", "2")
	require.NoError(t, cache.Update(modified, 2))

	events, _, err = cache.GetEventsSince(context.Background(), 1, nil)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, watch.Modified, events[0].Type)
	require.Equal(t, int64(2), events[0].ResourceVersion)

	deleted := makeService("svc-1", "3")
	require.NoError(t, cache.Delete(deleted, 3))

	events, _, err = cache.GetEventsSince(context.Background(), 2, nil)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, watch.Deleted, events[0].Type)
	require.Equal(t, int64(3), events[0].ResourceVersion)

	require.Empty(t, cache.GetCurrentObjects())

	bookmark, err := cache.CreateBookmark()
	require.NoError(t, err)
	require.Equal(t, int64(3), bookmark.ResourceVersion)
	require.True(t, bookmark.IsBookmark)
	require.WithinDuration(t, time.Now(), bookmark.Timestamp, time.Second)
}
