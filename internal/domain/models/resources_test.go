package models

import (
	"testing"
	"time"
)

func TestTransportProtocolConstants(t *testing.T) {
	tests := []struct {
		name     string
		protocol TransportProtocol
		expected string
	}{
		{"TCP", TCP, "TCP"},
		{"UDP", UDP, "UDP"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.protocol) != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, tt.protocol)
			}
		})
	}
}

func TestTrafficConstants(t *testing.T) {
	tests := []struct {
		name     string
		traffic  Traffic
		expected string
	}{
		{"INGRESS", INGRESS, "ingress"},
		{"EGRESS", EGRESS, "egress"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.traffic) != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, tt.traffic)
			}
		})
	}
}

func TestPortRange(t *testing.T) {
	tests := []struct {
		name      string
		portRange PortRange
		valid     bool
	}{
		{"Valid range", PortRange{Start: 80, End: 8080}, true},
		{"Single port", PortRange{Start: 80, End: 80}, true},
		{"Invalid range", PortRange{Start: 8080, End: 80}, false},
		{"Negative start", PortRange{Start: -1, End: 80}, false},
		{"Out of range end", PortRange{Start: 1, End: 65536}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := tt.portRange.Start <= tt.portRange.End &&
				tt.portRange.Start >= 0 &&
				tt.portRange.End <= 65535
			if valid != tt.valid {
				t.Errorf("Expected validity %v, got %v", tt.valid, valid)
			}
		})
	}
}

func TestProtocolPorts(t *testing.T) {
	pp := make(ProtocolPorts)
	pp[TCP] = []PortRange{
		{Start: 80, End: 80},
		{Start: 443, End: 443},
	}
	pp[UDP] = []PortRange{
		{Start: 53, End: 53},
	}

	if len(pp) != 2 {
		t.Errorf("Expected 2 protocols, got %d", len(pp))
	}

	if len(pp[TCP]) != 2 {
		t.Errorf("Expected 2 TCP port ranges, got %d", len(pp[TCP]))
	}

	if len(pp[UDP]) != 1 {
		t.Errorf("Expected 1 UDP port range, got %d", len(pp[UDP]))
	}
}

var (
	serviceRefWebDef        = NewServiceRef("web", WithNamespace("default"))
	addressGroupInternalDef = NewAddressGroupRef("internal", WithNamespace("default"))
)

func TestService(t *testing.T) {
	service := Service{
		SelfRef:     NewSelfRef(NewResourceIdentifier("web", WithNamespace("default"))),
		Description: "Web service",
		IngressPorts: []IngressPort{
			{Protocol: TCP, Port: "80", Description: "HTTP"},
			{Protocol: TCP, Port: "443", Description: "HTTPS"},
		},
		AddressGroups: []AddressGroupRef{addressGroupInternalDef},
	}

	if service.Name != "web" {
		t.Errorf("Expected name 'web', got '%s'", service.Name)
	}

	if len(service.IngressPorts) != 2 {
		t.Errorf("Expected 2 ingress ports, got %d", len(service.IngressPorts))
	}

	if len(service.AddressGroups) != 1 {
		t.Errorf("Expected 1 address group, got %d", len(service.AddressGroups))
	}
}

func TestAddressGroup(t *testing.T) {
	addressGroup := AddressGroup{
		SelfRef:       NewSelfRef(NewResourceIdentifier("internal", WithNamespace("default"))),
		DefaultAction: ActionAccept,
		Logs:          true,
		Trace:         false,
	}

	if addressGroup.Name != "internal" {
		t.Errorf("Expected name 'internal', got '%s'", addressGroup.Name)
	}

	if addressGroup.DefaultAction != ActionAccept {
		t.Errorf("Expected DefaultAction ACCEPT, got %s", addressGroup.DefaultAction)
	}

	if !addressGroup.Logs {
		t.Errorf("Expected Logs to be true")
	}
}

func TestAddressGroupBinding(t *testing.T) {
	binding := AddressGroupBinding{
		SelfRef:         NewSelfRef(NewResourceIdentifier("web-internal", WithNamespace("default"))),
		ServiceRef:      serviceRefWebDef,
		AddressGroupRef: addressGroupInternalDef,
	}

	if binding.Name != "web-internal" {
		t.Errorf("Expected name 'web-internal', got '%s'", binding.Name)
	}

	if binding.ServiceRef.Name != "web" {
		t.Errorf("Expected service name 'web', got '%s'", binding.ServiceRef.Name)
	}

	if binding.AddressGroupRef.Name != "internal" {
		t.Errorf("Expected address group name 'internal', got '%s'", binding.AddressGroupRef.Name)
	}
}

func TestNewServiceAlias(t *testing.T) {
	srvAlias := ServiceAlias{
		SelfRef:    NewSelfRef(NewResourceIdentifier("alias-to-web", WithNamespace("default"))),
		ServiceRef: serviceRefWebDef,
	}

	if srvAlias.Key() != "default/alias-to-web" {
		t.Errorf("Expected name 'default/alias-to-web', got '%s'", srvAlias.Key())

	}

	if srvAlias.ServiceRef.Key() != "default/web" {
		t.Errorf("Expected name 'default/web', got '%s'", srvAlias.ServiceRef.Key())

	}
}

func TestAddressGroupPortMapping(t *testing.T) {
	mapping := AddressGroupPortMapping{
		SelfRef: NewSelfRef(NewResourceIdentifier("internal-ports", WithNamespace("default"))),
		AccessPorts: map[ServiceRef]ServicePorts{
			serviceRefWebDef: {
				ProtocolPorts{
					TCP: []PortRange{
						{Start: 80, End: 80},
						{Start: 443, End: 443},
					},
				},
			},
		},
	}

	if mapping.Key() != "default/internal-ports" {
		t.Errorf("Expected name 'default/internal-ports', got '%s'", mapping.Key())
	}

	if len(mapping.AccessPorts) != 1 {
		t.Errorf("Expected 1 access port, got %d", len(mapping.AccessPorts))
	}

	svcRef := NewServiceRef("web", WithNamespace("default"))
	svcPorts, ok := mapping.AccessPorts[svcRef]
	if !ok {
		t.Errorf("Expected AccessPorts to contain key %v", svcRef)
	}

	// 5) Проверяем, что у этого сервиса два TCP-диапазона портов
	tcpRanges := svcPorts.Ports[TCP]
	if len(tcpRanges) != 2 {
		t.Errorf("Expected 2 TCP port ranges, got %d", len(tcpRanges))
	}
}

func TestRuleS2S(t *testing.T) {
	rule := RuleS2S{
		SelfRef:         NewSelfRef(NewResourceIdentifier("web-to-db", WithNamespace("default"))),
		Traffic:         EGRESS,
		ServiceLocalRef: NewServiceAliasRef("alias-web", WithNamespace("default")),
		ServiceRef:      NewServiceAliasRef("alias-db", WithNamespace("default")),
	}

	if rule.Key() != "default/web-to-db" {
		t.Errorf("Expected name 'default/web-to-db', got '%s'", rule.Key())
	}

	if rule.Traffic != EGRESS {
		t.Errorf("Expected traffic EGRESS, got %s", rule.Traffic)
	}

	if rule.ServiceLocalRef.Key() != "default/alias-web" {
		t.Errorf("Expected local service name 'default/alias-web', got '%s'", rule.ServiceLocalRef.Key())
	}

	if rule.ServiceRef.Key() != "default/alias-db" {
		t.Errorf("Expected service name 'default/alias-db', got '%s'", rule.ServiceRef.Key())
	}
}

func TestSyncStatus(t *testing.T) {
	now := time.Now()
	status := SyncStatus{
		UpdatedAt: now,
	}

	if !status.UpdatedAt.Equal(now) {
		t.Errorf("Expected updated at %v, got %v", now, status.UpdatedAt)
	}
}
