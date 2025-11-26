package netguard

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"

	"netguard-pg-backend/internal/api/netguard/converters"
	"netguard-pg-backend/internal/config"
	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
	convertk8s "netguard-pg-backend/internal/k8s/registry/convert"
	watchpkg "netguard-pg-backend/internal/watch"
	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"
)

type eventEncoder func(ctx context.Context, obj runtime.Object, event *netguardpb.WatchEvent) error

type WatchHandler struct {
	cacheProvider    watchCacheProvider
	encoders         map[string]eventEncoder
	pollInterval     time.Duration
	bookmarkInterval time.Duration
	allowBookmarks   bool
}

func NewWatchHandler(provider watchCacheProvider, cfg config.WatchConfig) *WatchHandler {
	return &WatchHandler{
		cacheProvider:    provider,
		encoders:         buildEncoders(),
		pollInterval:     cfg.PollInterval,
		bookmarkInterval: cfg.BookmarkInterval,
		allowBookmarks:   cfg.AllowBookmarks,
	}
}

func (h *WatchHandler) Stream(ctx context.Context, resourceType string, req *netguardpb.WatchRequest, send func(*netguardpb.WatchEvent) error) error {
	cache, ok := h.cacheProvider.Cache(resourceType)
	if !ok {
		return status.Errorf(codes.NotFound, "watch cache for %s not found", resourceType)
	}
	encoder, ok := h.encoders[resourceType]
	if !ok {
		return status.Errorf(codes.Unimplemented, "watch encoder for %s not found", resourceType)
	}

	listOpts, err := buildListOptions(req)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid selectors: %v", err)
	}

	resourceVersion, err := parseResourceVersion(req.GetResourceVersion())
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid resourceVersion: %v", err)
	}

	streamCtx := ctx
	cancel := func() {}
	if req.GetTimeoutSeconds() > 0 {
		streamCtx, cancel = context.WithTimeout(ctx, time.Duration(req.TimeoutSeconds)*time.Second)
	}
	defer cancel()

	pollInterval := h.pollInterval
	if pollInterval <= 0 {
		pollInterval = 500 * time.Millisecond
	}
	pollTicker := time.NewTicker(pollInterval)
	defer pollTicker.Stop()
	var (
		bookmarkTicker *time.Ticker
		bookmarkCh     <-chan time.Time
	)
	if h.allowBookmarks {
		bookmarkInterval := h.bookmarkInterval
		if bookmarkInterval <= 0 {
			bookmarkInterval = 30 * time.Second
		}
		bookmarkTicker = time.NewTicker(bookmarkInterval)
		defer bookmarkTicker.Stop()
		bookmarkCh = bookmarkTicker.C
	}

	watchpkg.IncrementActiveWatchers(resourceType)
	defer watchpkg.DecrementActiveWatchers(resourceType)

	for {
		select {
		case <-streamCtx.Done():
			return streamCtx.Err()
		default:
		}

		events, _, err := cache.GetEventsSince(streamCtx, resourceVersion, listOpts)
		if err != nil {
			return err
		}

		for _, ce := range events {
			event, err := buildWatchEvent(streamCtx, resourceType, ce, encoder)
			if err != nil {
				return err
			}
			if err := send(event); err != nil {
				return err
			}
			if ce.ResourceVersion > resourceVersion {
				resourceVersion = ce.ResourceVersion
			}
		}

		select {
		case <-streamCtx.Done():
			return streamCtx.Err()
		case <-bookmarkCh:
			if bookmarkTicker == nil {
				continue
			}
			bookmark, err := cache.CreateBookmark()
			if err != nil {
				return err
			}
			event := &netguardpb.WatchEvent{
				Type:            netguardpb.WatchEventType_WATCH_EVENT_TYPE_BOOKMARK,
				ResourceVersion: strconv.FormatInt(bookmark.ResourceVersion, 10),
				Timestamp:       timestamppb.New(bookmark.Timestamp),
			}
			if err := send(event); err != nil {
				return err
			}
		case <-pollTicker.C:
		}
	}
}

func buildWatchEvent(ctx context.Context, resourceType string, ce watchpkg.CachedEvent, encoder eventEncoder) (*netguardpb.WatchEvent, error) {
	event := &netguardpb.WatchEvent{
		Type:            toProtoEventType(ce.Type),
		ResourceVersion: strconv.FormatInt(ce.ResourceVersion, 10),
		Timestamp:       timestamppb.New(ce.Timestamp),
	}

	if ce.Type == watch.Bookmark || ce.Object == nil {
		return event, nil
	}

	if err := encoder(ctx, ce.Object, event); err != nil {
		return nil, fmt.Errorf("encode %s event: %w", resourceType, err)
	}

	return event, nil
}

func buildListOptions(req *netguardpb.WatchRequest) (*metainternalversion.ListOptions, error) {
	opts := &metainternalversion.ListOptions{
		LabelSelector: labels.Everything(),
		FieldSelector: fields.Everything(),
	}

	if ls := req.GetLabelSelector(); ls != "" {
		selector, err := labels.Parse(ls)
		if err != nil {
			return nil, err
		}
		opts.LabelSelector = selector
	}

	var selectors []fields.Selector
	if fs := req.GetFieldSelector(); fs != "" {
		selector, err := fields.ParseSelector(fs)
		if err != nil {
			return nil, err
		}
		selectors = append(selectors, selector)
	}
	if ns := req.GetNamespace(); ns != "" {
		selectors = append(selectors, fields.OneTermEqualSelector("metadata.namespace", ns))
	}
	if len(selectors) > 0 {
		composite := selectors[0]
		for _, sel := range selectors[1:] {
			composite = fields.AndSelectors(composite, sel)
		}
		opts.FieldSelector = composite
	}

	return opts, nil
}

func parseResourceVersion(rv string) (int64, error) {
	if rv == "" || rv == "0" {
		return 0, nil
	}
	return strconv.ParseInt(rv, 10, 64)
}

func toProtoEventType(t watch.EventType) netguardpb.WatchEventType {
	switch t {
	case watch.Added:
		return netguardpb.WatchEventType_WATCH_EVENT_TYPE_ADDED
	case watch.Modified:
		return netguardpb.WatchEventType_WATCH_EVENT_TYPE_MODIFIED
	case watch.Deleted:
		return netguardpb.WatchEventType_WATCH_EVENT_TYPE_DELETED
	case watch.Bookmark:
		return netguardpb.WatchEventType_WATCH_EVENT_TYPE_BOOKMARK
	default:
		return netguardpb.WatchEventType_WATCH_EVENT_TYPE_UNSPECIFIED
	}
}

func buildEncoders() map[string]eventEncoder {
	return map[string]eventEncoder{
		"services":                       encodeServiceEvent(),
		"address_groups":                 encodeAddressGroupEvent(),
		"address_group_bindings":         encodeAddressGroupBindingEvent(),
		"address_group_port_mappings":    encodeAddressGroupPortMappingEvent(),
		"address_group_binding_policies": encodeAddressGroupBindingPolicyEvent(),
		"rule_s2s":                       encodeRuleS2SEvent(),
		"service_aliases":                encodeServiceAliasEvent(),
		"hosts":                          encodeHostEvent(),
		"host_bindings":                  encodeHostBindingEvent(),
		"networks":                       encodeNetworkEvent(),
		"network_bindings":               encodeNetworkBindingEvent(),
		"ie_ag_ag_rules":                 encodeIEAgAgRuleEvent(),
		"svc_svc_rules":                  encodeSvcSvcRuleEvent(),
		"svc_fqdn_rules":                 encodeSvcFqdnRuleEvent(),
	}
}

func encodeServiceEvent() eventEncoder {
	k8sConv := &convertk8s.ServiceConverter{}
	return func(ctx context.Context, obj runtime.Object, event *netguardpb.WatchEvent) error {
		k8sObj, ok := obj.(*v1beta1.Service)
		if !ok {
			return fmt.Errorf("expected *Service, got %T", obj)
		}
		domainObj, err := k8sConv.ToDomain(ctx, k8sObj)
		if err != nil {
			return err
		}
		event.Object = &netguardpb.WatchEvent_Service{
			Service: converters.ConvertServiceToPB(*domainObj),
		}
		return nil
	}
}

func encodeAddressGroupEvent() eventEncoder {
	k8sConv := &convertk8s.AddressGroupConverter{}
	return func(ctx context.Context, obj runtime.Object, event *netguardpb.WatchEvent) error {
		k8sObj, ok := obj.(*v1beta1.AddressGroup)
		if !ok {
			return fmt.Errorf("expected *AddressGroup, got %T", obj)
		}
		domainObj, err := k8sConv.ToDomain(ctx, k8sObj)
		if err != nil {
			return err
		}
		event.Object = &netguardpb.WatchEvent_AddressGroup{
			AddressGroup: converters.ConvertAddressGroupToPB(*domainObj),
		}
		return nil
	}
}

func encodeAddressGroupBindingEvent() eventEncoder {
	k8sConv := &convertk8s.AddressGroupBindingConverter{}
	return func(ctx context.Context, obj runtime.Object, event *netguardpb.WatchEvent) error {
		k8sObj, ok := obj.(*v1beta1.AddressGroupBinding)
		if !ok {
			return fmt.Errorf("expected *AddressGroupBinding, got %T", obj)
		}
		domainObj, err := k8sConv.ToDomain(ctx, k8sObj)
		if err != nil {
			return err
		}
		event.Object = &netguardpb.WatchEvent_AddressGroupBinding{
			AddressGroupBinding: converters.ConvertAddressGroupBindingToPB(*domainObj),
		}
		return nil
	}
}

func encodeAddressGroupPortMappingEvent() eventEncoder {
	k8sConv := &convertk8s.AddressGroupPortMappingConverter{}
	return func(ctx context.Context, obj runtime.Object, event *netguardpb.WatchEvent) error {
		k8sObj, ok := obj.(*v1beta1.AddressGroupPortMapping)
		if !ok {
			return fmt.Errorf("expected *AddressGroupPortMapping, got %T", obj)
		}
		domainObj, err := k8sConv.ToDomain(ctx, k8sObj)
		if err != nil {
			return err
		}
		event.Object = &netguardpb.WatchEvent_AddressGroupPortMapping{
			AddressGroupPortMapping: converters.ConvertAddressGroupPortMappingToPB(*domainObj),
		}
		return nil
	}
}

func encodeAddressGroupBindingPolicyEvent() eventEncoder {
	k8sConv := &convertk8s.AddressGroupBindingPolicyConverter{}
	return func(ctx context.Context, obj runtime.Object, event *netguardpb.WatchEvent) error {
		k8sObj, ok := obj.(*v1beta1.AddressGroupBindingPolicy)
		if !ok {
			return fmt.Errorf("expected *AddressGroupBindingPolicy, got %T", obj)
		}
		domainObj, err := k8sConv.ToDomain(ctx, k8sObj)
		if err != nil {
			return err
		}
		event.Object = &netguardpb.WatchEvent_AddressGroupBindingPolicy{
			AddressGroupBindingPolicy: converters.ConvertAddressGroupBindingPolicyToPB(*domainObj),
		}
		return nil
	}
}

func encodeRuleS2SEvent() eventEncoder {
	k8sConv := &convertk8s.RuleS2SConverter{}
	return func(ctx context.Context, obj runtime.Object, event *netguardpb.WatchEvent) error {
		k8sObj, ok := obj.(*v1beta1.RuleS2S)
		if !ok {
			return fmt.Errorf("expected *RuleS2S, got %T", obj)
		}
		domainObj, err := k8sConv.ToDomain(ctx, k8sObj)
		if err != nil {
			return err
		}
		event.Object = &netguardpb.WatchEvent_RuleS2S{
			RuleS2S: converters.ConvertRuleS2SToPB(*domainObj),
		}
		return nil
	}
}

func encodeServiceAliasEvent() eventEncoder {
	k8sConv := &convertk8s.ServiceAliasConverter{}
	return func(ctx context.Context, obj runtime.Object, event *netguardpb.WatchEvent) error {
		k8sObj, ok := obj.(*v1beta1.ServiceAlias)
		if !ok {
			return fmt.Errorf("expected *ServiceAlias, got %T", obj)
		}
		domainObj, err := k8sConv.ToDomain(ctx, k8sObj)
		if err != nil {
			return err
		}
		event.Object = &netguardpb.WatchEvent_ServiceAlias{
			ServiceAlias: converters.ConvertServiceAliasToPB(*domainObj),
		}
		return nil
	}
}

func encodeHostEvent() eventEncoder {
	k8sConv := &convertk8s.HostConverter{}
	return func(ctx context.Context, obj runtime.Object, event *netguardpb.WatchEvent) error {
		k8sObj, ok := obj.(*v1beta1.Host)
		if !ok {
			return fmt.Errorf("expected *Host, got %T", obj)
		}
		domainObj, err := k8sConv.ToDomain(ctx, k8sObj)
		if err != nil {
			return err
		}
		event.Object = &netguardpb.WatchEvent_Host{
			Host: converters.ConvertHostToPB(*domainObj),
		}
		return nil
	}
}

func encodeHostBindingEvent() eventEncoder {
	k8sConv := &convertk8s.HostBindingConverter{}
	return func(ctx context.Context, obj runtime.Object, event *netguardpb.WatchEvent) error {
		k8sObj, ok := obj.(*v1beta1.HostBinding)
		if !ok {
			return fmt.Errorf("expected *HostBinding, got %T", obj)
		}
		domainObj, err := k8sConv.ToDomain(ctx, k8sObj)
		if err != nil {
			return err
		}
		event.Object = &netguardpb.WatchEvent_HostBinding{
			HostBinding: converters.ConvertHostBindingToPB(*domainObj),
		}
		return nil
	}
}

func encodeNetworkEvent() eventEncoder {
	k8sConv := &convertk8s.NetworkConverter{}
	return func(ctx context.Context, obj runtime.Object, event *netguardpb.WatchEvent) error {
		k8sObj, ok := obj.(*v1beta1.Network)
		if !ok {
			return fmt.Errorf("expected *Network, got %T", obj)
		}
		domainObj, err := k8sConv.ToDomain(ctx, k8sObj)
		if err != nil {
			return err
		}
		event.Object = &netguardpb.WatchEvent_Network{
			Network: converters.ConvertNetworkToPB(*domainObj),
		}
		return nil
	}
}

func encodeNetworkBindingEvent() eventEncoder {
	k8sConv := &convertk8s.NetworkBindingConverter{}
	return func(ctx context.Context, obj runtime.Object, event *netguardpb.WatchEvent) error {
		k8sObj, ok := obj.(*v1beta1.NetworkBinding)
		if !ok {
			return fmt.Errorf("expected *NetworkBinding, got %T", obj)
		}
		domainObj, err := k8sConv.ToDomain(ctx, k8sObj)
		if err != nil {
			return err
		}
		event.Object = &netguardpb.WatchEvent_NetworkBinding{
			NetworkBinding: converters.ConvertNetworkBindingToPB(*domainObj),
		}
		return nil
	}
}

func encodeIEAgAgRuleEvent() eventEncoder {
	k8sConv := &convertk8s.IEAgAgRuleConverter{}
	return func(ctx context.Context, obj runtime.Object, event *netguardpb.WatchEvent) error {
		k8sObj, ok := obj.(*v1beta1.IEAgAgRule)
		if !ok {
			return fmt.Errorf("expected *IEAgAgRule, got %T", obj)
		}
		domainObj, err := k8sConv.ToDomain(ctx, k8sObj)
		if err != nil {
			return err
		}
		event.Object = &netguardpb.WatchEvent_IeAgAgRule{
			IeAgAgRule: converters.ConvertIEAgAgRuleToPB(*domainObj),
		}
		return nil
	}
}

func encodeSvcSvcRuleEvent() eventEncoder {
	k8sConv := &convertk8s.SvcSvcRuleConverter{}
	return func(ctx context.Context, obj runtime.Object, event *netguardpb.WatchEvent) error {
		k8sObj, ok := obj.(*v1beta1.SvcSvcRule)
		if !ok {
			return fmt.Errorf("expected *SvcSvcRule, got %T", obj)
		}
		domainObj, err := k8sConv.ToDomain(ctx, k8sObj)
		if err != nil {
			return err
		}
		event.Object = &netguardpb.WatchEvent_SvcSvcRule{
			SvcSvcRule: converters.ConvertSvcSvcRuleToPB(*domainObj),
		}
		return nil
	}
}

func encodeSvcFqdnRuleEvent() eventEncoder {
	k8sConv := &convertk8s.SvcFqdnRuleConverter{}
	return func(ctx context.Context, obj runtime.Object, event *netguardpb.WatchEvent) error {
		k8sObj, ok := obj.(*v1beta1.SvcFqdnRule)
		if !ok {
			return fmt.Errorf("expected *SvcFqdnRule, got %T", obj)
		}
		domainObj, err := k8sConv.ToDomain(ctx, k8sObj)
		if err != nil {
			return err
		}
		event.Object = &netguardpb.WatchEvent_SvcFqdnRule{
			SvcFqdnRule: converters.ConvertSvcFqdnRuleToPB(*domainObj),
		}
		return nil
	}
}
