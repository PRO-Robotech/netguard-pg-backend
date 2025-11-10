package models

import (
	"testing"
	"time"

	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSortConditions(t *testing.T) {
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	conds := []metav1.Condition{
		{Type: "Ready", Status: metav1.ConditionTrue, LastTransitionTime: metav1.NewTime(base.Add(10 * time.Minute))},
		{Type: "Synced", Status: metav1.ConditionFalse, LastTransitionTime: metav1.NewTime(base.Add(5 * time.Minute))},
		{Type: "Ready", Status: metav1.ConditionFalse, LastTransitionTime: metav1.NewTime(base.Add(2 * time.Minute))},
		{Type: "Validated", Status: metav1.ConditionTrue, LastTransitionTime: metav1.NewTime(base.Add(5 * time.Minute))},
	}

	SortConditions(conds)

	want := []string{"Ready", "Synced", "Validated", "Ready"}
	for i, cond := range conds {
		if cond.Type != want[i] {
			t.Fatalf("expected type %q at position %d, got %q", want[i], i, cond.Type)
		}
	}
}

func TestSortIngressPorts(t *testing.T) {
	ports := []IngressPort{
		{Protocol: UDP, Port: "53", Description: "dns"},
		{Protocol: TCP, Port: "443", Description: "https"},
		{Protocol: TCP, Port: "80", Description: "http"},
		{Protocol: TCP, Port: "443", Description: "alt"},
	}

	SortIngressPorts(ports)

	want := []IngressPort{
		{Protocol: TCP, Port: "80", Description: "http"},
		{Protocol: TCP, Port: "443", Description: "alt"},
		{Protocol: TCP, Port: "443", Description: "https"},
		{Protocol: UDP, Port: "53", Description: "dns"},
	}

	if len(ports) != len(want) {
		t.Fatalf("expected %d ports, got %d", len(want), len(ports))
	}
	for i := range ports {
		if ports[i] != want[i] {
			t.Fatalf("unexpected order at %d: got %+v want %+v", i, ports[i], want[i])
		}
	}
}

func TestSortAggregatedAddressGroupRefs(t *testing.T) {
	refs := []AddressGroupReference{
		{Ref: netguardv1beta1.NamespacedObjectReference{ObjectReference: netguardv1beta1.ObjectReference{Name: "svc"}, Namespace: "b"}, Source: AddressGroupSourceBinding},
		{Ref: netguardv1beta1.NamespacedObjectReference{ObjectReference: netguardv1beta1.ObjectReference{Name: "svc"}, Namespace: "a"}, Source: AddressGroupSourceSpec},
		{Ref: netguardv1beta1.NamespacedObjectReference{ObjectReference: netguardv1beta1.ObjectReference{Name: "svc"}, Namespace: "a"}, Source: AddressGroupSourceBinding},
	}

	SortAggregatedAddressGroupRefs(refs)

	if refs[0].Ref.Namespace != "a" || refs[0].Source != AddressGroupSourceBinding {
		terr := refs[0]
		t.Fatalf("unexpected first ref: %+v", terr)
	}
	if refs[1].Source != AddressGroupSourceSpec {
		t.Fatalf("expected spec source second, got %+v", refs[1])
	}
	if refs[2].Ref.Namespace != "b" {
		t.Fatalf("expected namespace b last, got %+v", refs[2])
	}
}

func TestSortHostReferences(t *testing.T) {
	refs := []HostReference{
		{Ref: netguardv1beta1.NamespacedObjectReference{ObjectReference: netguardv1beta1.ObjectReference{Name: "b"}, Namespace: "ns"}, Source: HostSourceBinding},
		{Ref: netguardv1beta1.NamespacedObjectReference{ObjectReference: netguardv1beta1.ObjectReference{Name: "a"}, Namespace: "ns"}, Source: HostSourceBinding},
		{Ref: netguardv1beta1.NamespacedObjectReference{ObjectReference: netguardv1beta1.ObjectReference{Name: "a"}, Namespace: "ns"}, Source: HostSourceSpec},
	}

	SortHostReferences(refs)

	if refs[0].Ref.Name != "a" || refs[0].Source != HostSourceSpec {
		t.Fatalf("expected ns/a spec first, got %+v", refs[0])
	}
	if refs[1].Ref.Name != "a" || refs[1].Source != HostSourceBinding {
		t.Fatalf("expected ns/a binding second, got %+v", refs[1])
	}
	if refs[2].Ref.Name != "b" {
		t.Fatalf("expected ns/b last, got %+v", refs[2])
	}
}

func TestSortNetworkItems(t *testing.T) {
	items := []NetworkItem{
		{Name: "net-b", Namespace: "ns", CIDR: "10.0.2.0/24"},
		{Name: "net-a", Namespace: "ns", CIDR: "10.0.1.0/24"},
		{Name: "net-a", Namespace: "ns", CIDR: "10.0.0.0/24"},
	}

	SortNetworkItems(items)

	if items[0].CIDR != "10.0.0.0/24" || items[1].CIDR != "10.0.1.0/24" {
		t.Fatalf("expected CIDR ordering for same namespace/name, got %+v", items[:2])
	}
	if items[2].Name != "net-b" {
		t.Fatalf("expected net-b last, got %+v", items[2])
	}
}

func TestSortIPItems(t *testing.T) {
	ips := []IPItem{{IP: "10.0.0.2"}, {IP: "10.0.0.1"}, {IP: "2001:db8::1"}}

	SortIPItems(ips)

	got := []string{ips[0].IP, ips[1].IP, ips[2].IP}
	want := []string{"10.0.0.1", "10.0.0.2", "2001:db8::1"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected IP order: got %v want %v", got, want)
		}
	}
}

func TestNormalizeProtocolPorts(t *testing.T) {
	pp := ProtocolPorts{
		TCP: {{Start: 8080, End: 8080}, {Start: 80, End: 80}},
		UDP: {{Start: 53, End: 53}, {Start: 5000, End: 5002}},
	}

	NormalizeProtocolPorts(pp)

	if pp[TCP][0].Start != 80 || pp[TCP][1].Start != 8080 {
		t.Fatalf("expected TCP ports sorted ascending, got %+v", pp[TCP])
	}
	if pp[UDP][0].Start != 53 {
		t.Fatalf("expected UDP ports sorted ascending, got %+v", pp[UDP])
	}
}
