package testutil

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

// TestFixtures provides consistent test data for all test suites
var TestFixtures = struct {
	Service             models.Service
	ServiceWithPorts    models.Service
	ServiceAlias        models.ServiceAlias
	AddressGroup        models.AddressGroup
	AddressGroupBinding models.AddressGroupBinding
	Network             models.Network
	NetworkBinding      models.NetworkBinding
	RuleS2S             models.RuleS2S
	IEAgAgRule          models.IEAgAgRule
	ResourceIdentifier  models.ResourceIdentifier
	ResourceIdentifier2 models.ResourceIdentifier
	Condition           metav1.Condition
	IngressPort         models.IngressPort
}{
	// Basic resource identifier
	ResourceIdentifier: models.ResourceIdentifier{
		Name:      "test-resource",
		Namespace: "test-namespace",
	},

	ResourceIdentifier2: models.ResourceIdentifier{
		Name:      "test-resource-2",
		Namespace: "test-namespace",
	},

	// Service without ports
	Service: models.Service{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      "test-service",
				Namespace: "test-namespace",
			},
		},
		Description:   "Test service for unit tests",
		IngressPorts:  []models.IngressPort{},
		AddressGroups: []models.AddressGroupRef{},
		Meta: models.Meta{
			CreationTS: metav1.NewTime(time.Now()),
			Generation: 1,
		},
	},

	// Service with ingress ports
	ServiceWithPorts: models.Service{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      "test-service-with-ports",
				Namespace: "test-namespace",
			},
		},
		Description: "Test service with ports",
		IngressPorts: []models.IngressPort{
			{
				Port:     "80",
				Protocol: models.TCP,
			},
			{
				Port:     "443",
				Protocol: models.TCP,
			},
		},
		AddressGroups: []models.AddressGroupRef{
			models.NewAddressGroupRef("test-address-group", models.WithNamespace("test-namespace")),
		},
		Meta: models.Meta{
			CreationTS: metav1.NewTime(time.Now()),
			Generation: 1,
		},
	},

	// Service alias
	ServiceAlias: models.ServiceAlias{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      "test-service-alias",
				Namespace: "test-namespace",
			},
		},
		ServiceRef: models.NewServiceRef("test-service", models.WithNamespace("test-namespace")),
		Meta: models.Meta{
			CreationTS: metav1.NewTime(time.Now()),
			Generation: 1,
		},
	},

	// Address group
	AddressGroup: models.AddressGroup{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      "test-address-group",
				Namespace: "test-namespace",
			},
		},
		DefaultAction: models.ActionAccept,
		Networks: []models.NetworkItem{
			{
				Name:      "test-network",
				Namespace: "test-namespace",
				CIDR:      "10.0.0.0/24",
			},
		},
		Trace: false,
		Logs:  false,
		Meta: models.Meta{
			CreationTS: metav1.NewTime(time.Now()),
			Generation: 1,
		},
	},

	// Network
	Network: models.Network{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      "test-network",
				Namespace: "test-namespace",
			},
		},
		CIDR: "10.0.0.0/24",
		Meta: models.Meta{
			CreationTS: metav1.NewTime(time.Now()),
			Generation: 1,
		},
	},

	// Network binding
	NetworkBinding: models.NetworkBinding{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      "test-network-binding",
				Namespace: "test-namespace",
			},
		},
		NetworkRef: v1beta1.ObjectReference{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "Network",
			Name:       "test-network",
		},
		AddressGroupRef: v1beta1.ObjectReference{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "AddressGroup",
			Name:       "test-address-group",
		},
		Meta: models.Meta{
			CreationTS: metav1.NewTime(time.Now()),
			Generation: 1,
		},
	},

	// RuleS2S
	RuleS2S: models.RuleS2S{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      "test-rule-s2s",
				Namespace: "test-namespace",
			},
		},
		Traffic:         models.EGRESS,
		ServiceLocalRef: models.NewServiceRef("test-local-service", models.WithNamespace("test-namespace")),
		ServiceRef:      models.NewServiceRef("test-service", models.WithNamespace("test-namespace")),
		Trace:           false,
		Meta: models.Meta{
			CreationTS: metav1.NewTime(time.Now()),
			Generation: 1,
		},
	},

	// IEAgAgRule
	IEAgAgRule: models.IEAgAgRule{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      "test-ieagag-rule",
				Namespace: "test-namespace",
			},
		},
		AddressGroupLocal: models.NewAddressGroupRef("test-local-address-group", models.WithNamespace("test-namespace")),
		AddressGroup:      models.NewAddressGroupRef("test-target-address-group", models.WithNamespace("test-namespace")),
		Traffic:           models.INGRESS,
		Ports: []models.PortSpec{
			{
				Source:      "443",
				Destination: "443",
			},
		},
		Action: models.ActionAccept,
		Meta: models.Meta{
			CreationTS: metav1.NewTime(time.Now()),
			Generation: 1,
		},
	},

	// Address group binding
	AddressGroupBinding: models.AddressGroupBinding{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      "test-address-group-binding",
				Namespace: "test-namespace",
			},
		},
		ServiceRef:      models.NewServiceRef("test-service", models.WithNamespace("test-namespace")),
		AddressGroupRef: models.NewAddressGroupRef("test-address-group", models.WithNamespace("test-namespace")),
		Meta: models.Meta{
			CreationTS: metav1.NewTime(time.Now()),
			Generation: 1,
		},
	},

	// Basic condition
	Condition: metav1.Condition{
		Type:               models.ConditionReady,
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.NewTime(time.Now()),
		Reason:             "TestReason",
		Message:            "Test condition message",
	},

	// Ingress port
	IngressPort: models.IngressPort{
		Port:     "8080",
		Protocol: models.TCP,
	},
}

// CreateTestService creates a test service with the given name and namespace
func CreateTestService(name, namespace string) models.Service {
	service := TestFixtures.Service
	service.SelfRef.ResourceIdentifier.Name = name
	service.SelfRef.ResourceIdentifier.Namespace = namespace
	return service
}

// CreateTestAddressGroup creates a test address group with the given name and namespace
func CreateTestAddressGroup(name, namespace string) models.AddressGroup {
	// Create a new address group from scratch to avoid shared references
	return models.AddressGroup{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      name,
				Namespace: namespace,
			},
		},
		DefaultAction: models.ActionAccept,
		Networks:      []models.NetworkItem{}, // Start with empty networks
		Trace:         false,
		Logs:          false,
		Meta: models.Meta{
			CreationTS: metav1.NewTime(time.Now()),
			Generation: 1,
		},
	}
}

// CreateTestNetwork creates a test network with the given name, namespace, and CIDR
func CreateTestNetwork(name, namespace, cidr string) models.Network {
	// Create a new network from scratch to avoid shared references
	return models.Network{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      name,
				Namespace: namespace,
			},
		},
		CIDR: cidr,
		Meta: models.Meta{
			CreationTS: metav1.NewTime(time.Now()),
			Generation: 1,
		},
	}
}

// CreateTestResourceIdentifier creates a test resource identifier
func CreateTestResourceIdentifier(name, namespace string) models.ResourceIdentifier {
	return models.ResourceIdentifier{
		Name:      name,
		Namespace: namespace,
	}
}

// CreateTestNetworkBinding creates a test network binding with the given parameters
func CreateTestNetworkBinding(name, namespace, networkName, addressGroupName string) models.NetworkBinding {
	// Create a new binding from scratch to avoid shared references
	return models.NetworkBinding{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      name,
				Namespace: namespace,
			},
		},
		NetworkRef: v1beta1.ObjectReference{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "Network",
			Name:       networkName,
		},
		AddressGroupRef: v1beta1.ObjectReference{
			APIVersion: "netguard.sgroups.io/v1beta1",
			Kind:       "AddressGroup",
			Name:       addressGroupName,
		},
		Meta: models.Meta{
			CreationTS: metav1.NewTime(time.Now()),
			Generation: 1,
		},
	}
}

// CreateTestRuleS2S creates a test RuleS2S with the given parameters
func CreateTestRuleS2S(name, namespace string) models.RuleS2S {
	// Create a new RuleS2S from scratch to avoid shared references
	return models.RuleS2S{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      name,
				Namespace: namespace,
			},
		},
		Traffic:         models.EGRESS,
		ServiceLocalRef: models.NewServiceRef("test-local-service", models.WithNamespace(namespace)),
		ServiceRef:      models.NewServiceRef("test-service", models.WithNamespace(namespace)),
		Trace:           false,
		Meta: models.Meta{
			CreationTS: metav1.NewTime(time.Now()),
			Generation: 1,
		},
	}
}

// CreateTestIEAgAgRule creates a test IEAgAgRule with the given parameters
func CreateTestIEAgAgRule(name, namespace string) models.IEAgAgRule {
	// Create a new IEAgAgRule from scratch to avoid shared references
	return models.IEAgAgRule{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      name,
				Namespace: namespace,
			},
		},
		AddressGroupLocal: models.NewAddressGroupRef("test-local-address-group", models.WithNamespace(namespace)),
		AddressGroup:      models.NewAddressGroupRef("test-target-address-group", models.WithNamespace(namespace)),
		Traffic:           models.INGRESS,
		Ports: []models.PortSpec{
			{
				Source:      "443",
				Destination: "443",
			},
		},
		Action: models.ActionAccept,
		Meta: models.Meta{
			CreationTS: metav1.NewTime(time.Now()),
			Generation: 1,
		},
	}
}

// CreateTestServiceAlias creates a test ServiceAlias with the given parameters
func CreateTestServiceAlias(name, namespace, serviceName string) models.ServiceAlias {
	// Create a new ServiceAlias from scratch to avoid shared references
	return models.ServiceAlias{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      name,
				Namespace: namespace,
			},
		},
		ServiceRef: models.NewServiceRef(serviceName, models.WithNamespace(namespace)),
		Meta: models.Meta{
			CreationTS: metav1.NewTime(time.Now()),
			Generation: 1,
		},
	}
}

// CreateTestCondition creates a test condition
func CreateTestCondition(conditionType string, status metav1.ConditionStatus, reason, message string) metav1.Condition {
	return metav1.Condition{
		Type:               conditionType,
		Status:             status,
		LastTransitionTime: metav1.NewTime(time.Now()),
		Reason:             reason,
		Message:            message,
	}
}
