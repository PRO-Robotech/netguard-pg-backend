package watch

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"

	apiconverters "netguard-pg-backend/internal/api/netguard/converters"
	convertk8s "netguard-pg-backend/internal/k8s/registry/convert"
	watchpb "netguard-pg-backend/protos/pkg/api/netguard"
)

type Stream interface {
	Recv() (*watchpb.WatchEvent, error)
	Context() context.Context
}

func NewGRPCWatcher(resourceType string, stream Stream) (watch.Interface, error) {
	decoder, err := decoderForResource(resourceType)
	if err != nil {
		return nil, err
	}
	return newGRPCStreamWatcher(stream, decoder), nil
}

type runtimeDecoder func(ctx context.Context, evt *watchpb.WatchEvent) (runtime.Object, error)

type grpcStreamWatcher struct {
	resultChan chan watch.Event
	stopChan   chan struct{}
	stopOnce   sync.Once
}

func newGRPCStreamWatcher(stream Stream, decoder runtimeDecoder) watch.Interface {
	w := &grpcStreamWatcher{
		resultChan: make(chan watch.Event),
		stopChan:   make(chan struct{}),
	}

	go w.run(stream, decoder)
	return w
}

func (w *grpcStreamWatcher) run(stream Stream, decoder runtimeDecoder) {
	defer close(w.resultChan)

	for {
		select {
		case <-w.stopChan:
			return
		default:
		}

		evt, err := stream.Recv()
		if err != nil {
			return
		}

		eventType := fromProtoEventType(evt.GetType())
		var obj runtime.Object

		switch eventType {
		case watch.Error:
			obj = &metav1.Status{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "meta.k8s.io/v1",
					Kind:       "Status",
				},
				Status:  metav1.StatusFailure,
				Message: evt.GetErrorMessage(),
				Reason:  metav1.StatusReasonUnknown,
				Code:    int32(http.StatusInternalServerError),
			}
		case watch.Bookmark:
			// Bookmark events lead to "<unknown>" rows in kubectl watch output,
			// and we already keep track of resource versions elsewhere.
			// Skip emitting them to clients entirely.
			continue
		default:
			if evt.Object == nil {
				continue
			}
			obj, err = decoder(stream.Context(), evt)
			if err != nil {
				continue
			}
		}

		w.resultChan <- watch.Event{
			Type:   eventType,
			Object: obj,
		}
	}
}

func (w *grpcStreamWatcher) ResultChan() <-chan watch.Event {
	return w.resultChan
}

func (w *grpcStreamWatcher) Stop() {
	w.stopOnce.Do(func() {
		close(w.stopChan)
	})
}

func buildBookmark(rv string) runtime.Object {
	return &metav1.PartialObjectMetadata{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PartialObjectMetadata",
			APIVersion: "meta.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			ResourceVersion: rv,
		},
	}
}

func fromProtoEventType(t watchpb.WatchEventType) watch.EventType {
	switch t {
	case watchpb.WatchEventType_WATCH_EVENT_TYPE_ADDED:
		return watch.Added
	case watchpb.WatchEventType_WATCH_EVENT_TYPE_MODIFIED:
		return watch.Modified
	case watchpb.WatchEventType_WATCH_EVENT_TYPE_DELETED:
		return watch.Deleted
	case watchpb.WatchEventType_WATCH_EVENT_TYPE_BOOKMARK:
		return watch.Bookmark
	default:
		return watch.Error
	}
}

func decoderForResource(resourceType string) (runtimeDecoder, error) {
	switch resourceType {
	case "services":
		return buildServiceDecoder(), nil
	case "addressgroups":
		return buildAddressGroupDecoder(), nil
	case "addressgroupbindings":
		return buildAddressGroupBindingDecoder(), nil
	case "addressgroupportmappings":
		return buildAddressGroupPortMappingDecoder(), nil
	case "addressgroupbindingpolicies":
		return buildAddressGroupBindingPolicyDecoder(), nil
	case "hosts":
		return buildHostDecoder(), nil
	case "hostbindings":
		return buildHostBindingDecoder(), nil
	case "networks":
		return buildNetworkDecoder(), nil
	case "networkbindings":
		return buildNetworkBindingDecoder(), nil
	case "svcsvcrules":
		return buildSvcSvcRuleDecoder(), nil
	case "svcfqdnrules":
		return buildSvcFqdnRuleDecoder(), nil
	case "iecidrsvrules":
		return buildIECidrSvcRuleDecoder(), nil
	default:
		return nil, fmt.Errorf("unsupported watch resource %q", resourceType)
	}
}

func buildServiceDecoder() runtimeDecoder {
	k8sConv := &convertk8s.ServiceConverter{}
	return func(ctx context.Context, evt *watchpb.WatchEvent) (runtime.Object, error) {
		pb := evt.GetService()
		if pb == nil {
			return nil, fmt.Errorf("service payload missing")
		}
		domain := apiconverters.ConvertService(pb)
		return k8sConv.FromDomain(ctx, &domain)
	}
}

func buildAddressGroupDecoder() runtimeDecoder {
	k8sConv := &convertk8s.AddressGroupConverter{}
	return func(ctx context.Context, evt *watchpb.WatchEvent) (runtime.Object, error) {
		pb := evt.GetAddressGroup()
		if pb == nil {
			return nil, fmt.Errorf("address group payload missing")
		}
		domain := apiconverters.ConvertAddressGroup(pb)
		return k8sConv.FromDomain(ctx, &domain)
	}
}

func buildAddressGroupBindingDecoder() runtimeDecoder {
	k8sConv := &convertk8s.AddressGroupBindingConverter{}
	return func(ctx context.Context, evt *watchpb.WatchEvent) (runtime.Object, error) {
		pb := evt.GetAddressGroupBinding()
		if pb == nil {
			return nil, fmt.Errorf("address group binding payload missing")
		}
		domain := apiconverters.ConvertAddressGroupBinding(pb)
		return k8sConv.FromDomain(ctx, &domain)
	}
}

func buildAddressGroupPortMappingDecoder() runtimeDecoder {
	k8sConv := &convertk8s.AddressGroupPortMappingConverter{}
	return func(ctx context.Context, evt *watchpb.WatchEvent) (runtime.Object, error) {
		pb := evt.GetAddressGroupPortMapping()
		if pb == nil {
			return nil, fmt.Errorf("address group port mapping payload missing")
		}
		domain := apiconverters.ConvertAddressGroupPortMapping(pb)
		return k8sConv.FromDomain(ctx, &domain)
	}
}

func buildAddressGroupBindingPolicyDecoder() runtimeDecoder {
	k8sConv := &convertk8s.AddressGroupBindingPolicyConverter{}
	return func(ctx context.Context, evt *watchpb.WatchEvent) (runtime.Object, error) {
		pb := evt.GetAddressGroupBindingPolicy()
		if pb == nil {
			return nil, fmt.Errorf("address group binding policy payload missing")
		}
		domain := apiconverters.ConvertAddressGroupBindingPolicy(pb)
		return k8sConv.FromDomain(ctx, &domain)
	}
}

func buildHostDecoder() runtimeDecoder {
	k8sConv := &convertk8s.HostConverter{}
	return func(ctx context.Context, evt *watchpb.WatchEvent) (runtime.Object, error) {
		pb := evt.GetHost()
		if pb == nil {
			return nil, fmt.Errorf("host payload missing")
		}
		domain := apiconverters.ConvertHost(pb)
		return k8sConv.FromDomain(ctx, &domain)
	}
}

func buildHostBindingDecoder() runtimeDecoder {
	k8sConv := &convertk8s.HostBindingConverter{}
	return func(ctx context.Context, evt *watchpb.WatchEvent) (runtime.Object, error) {
		pb := evt.GetHostBinding()
		if pb == nil {
			return nil, fmt.Errorf("host binding payload missing")
		}
		domain := apiconverters.ConvertHostBinding(pb)
		return k8sConv.FromDomain(ctx, &domain)
	}
}

func buildNetworkDecoder() runtimeDecoder {
	k8sConv := &convertk8s.NetworkConverter{}
	return func(ctx context.Context, evt *watchpb.WatchEvent) (runtime.Object, error) {
		pb := evt.GetNetwork()
		if pb == nil {
			return nil, fmt.Errorf("network payload missing")
		}
		domain := apiconverters.ConvertNetwork(pb)
		return k8sConv.FromDomain(ctx, &domain)
	}
}

func buildNetworkBindingDecoder() runtimeDecoder {
	k8sConv := &convertk8s.NetworkBindingConverter{}
	return func(ctx context.Context, evt *watchpb.WatchEvent) (runtime.Object, error) {
		pb := evt.GetNetworkBinding()
		if pb == nil {
			return nil, fmt.Errorf("network binding payload missing")
		}
		domain := apiconverters.ConvertNetworkBinding(pb)
		return k8sConv.FromDomain(ctx, &domain)
	}
}

func buildSvcSvcRuleDecoder() runtimeDecoder {
	k8sConv := &convertk8s.SvcSvcRuleConverter{}
	return func(ctx context.Context, evt *watchpb.WatchEvent) (runtime.Object, error) {
		pb := evt.GetSvcSvcRule()
		if pb == nil {
			return nil, fmt.Errorf("svc-svc rule payload missing")
		}
		domain := apiconverters.ConvertSvcSvcRule(pb)
		return k8sConv.FromDomain(ctx, &domain)
	}
}

func buildSvcFqdnRuleDecoder() runtimeDecoder {
	k8sConv := &convertk8s.SvcFqdnRuleConverter{}
	return func(ctx context.Context, evt *watchpb.WatchEvent) (runtime.Object, error) {
		pb := evt.GetSvcFqdnRule()
		if pb == nil {
			return nil, fmt.Errorf("svc-fqdn rule payload missing")
		}
		domain := apiconverters.ConvertSvcFqdnRule(pb)
		return k8sConv.FromDomain(ctx, &domain)
	}
}

func buildIECidrSvcRuleDecoder() runtimeDecoder {
	k8sConv := &convertk8s.IECidrSvcRuleConverter{}
	return func(ctx context.Context, evt *watchpb.WatchEvent) (runtime.Object, error) {
		pb := evt.GetIecidrsvcRule()
		if pb == nil {
			return nil, fmt.Errorf("ie-cidr-svc rule payload missing")
		}
		domain := apiconverters.ConvertIECidrSvcRule(pb)
		return k8sConv.FromDomain(ctx, &domain)
	}
}
