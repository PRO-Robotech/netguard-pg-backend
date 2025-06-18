package services_test

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"netguard-pg-backend/internal/application/services"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/patterns"
)

// MockRegistryForIEAgAgRules extends the MockRegistry to support IEAgAgRules
type MockRegistryForIEAgAgRules struct {
	reader  ports.Reader
	writer  ports.Writer
	subject patterns.Subject
}

func NewMockRegistryForIEAgAgRules(reader ports.Reader, writer ports.Writer) *MockRegistryForIEAgAgRules {
	return &MockRegistryForIEAgAgRules{
		reader:  reader,
		writer:  writer,
		subject: &MockSubject{},
	}
}

func (m *MockRegistryForIEAgAgRules) Reader(ctx context.Context) (ports.Reader, error) {
	return m.reader, nil
}

func (m *MockRegistryForIEAgAgRules) Writer(ctx context.Context) (ports.Writer, error) {
	return m.writer, nil
}

func (m *MockRegistryForIEAgAgRules) Subject() patterns.Subject {
	return m.subject
}

func (m *MockRegistryForIEAgAgRules) Close() error {
	return nil
}

// MockReaderForIEAgAgRules implements a custom reader for IEAgAgRules tests
type MockReaderForIEAgAgRules struct {
	service             models.Service
	serviceAlias        models.ServiceAlias
	addressGroupLocal   models.AddressGroup
	addressGroup        models.AddressGroup
	ruleS2S             models.RuleS2S
	existingIEAgAgRules []models.IEAgAgRule
}

func NewMockReaderForIEAgAgRules() *MockReaderForIEAgAgRules {
	return &MockReaderForIEAgAgRules{}
}

func (m *MockReaderForIEAgAgRules) Close() error {
	return nil
}

func (m *MockReaderForIEAgAgRules) ListServices(ctx context.Context, consume func(models.Service) error, scope ports.Scope) error {
	return consume(m.service)
}

func (m *MockReaderForIEAgAgRules) ListAddressGroups(ctx context.Context, consume func(models.AddressGroup) error, scope ports.Scope) error {
	if err := consume(m.addressGroupLocal); err != nil {
		return err
	}
	return consume(m.addressGroup)
}

func (m *MockReaderForIEAgAgRules) ListAddressGroupBindings(ctx context.Context, consume func(models.AddressGroupBinding) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForIEAgAgRules) ListAddressGroupPortMappings(ctx context.Context, consume func(models.AddressGroupPortMapping) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForIEAgAgRules) ListRuleS2S(ctx context.Context, consume func(models.RuleS2S) error, scope ports.Scope) error {
	return consume(m.ruleS2S)
}

func (m *MockReaderForIEAgAgRules) ListServiceAliases(ctx context.Context, consume func(models.ServiceAlias) error, scope ports.Scope) error {
	return consume(m.serviceAlias)
}

func (m *MockReaderForIEAgAgRules) GetSyncStatus(ctx context.Context) (*models.SyncStatus, error) {
	return nil, nil
}

func (m *MockReaderForIEAgAgRules) GetAddressGroupBindingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBinding, error) {
	return nil, errors.New("address group binding not found")
}

func (m *MockReaderForIEAgAgRules) GetAddressGroupPortMappingByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupPortMapping, error) {
	return nil, errors.New("address group port mapping not found")
}

func (m *MockReaderForIEAgAgRules) GetServiceByID(ctx context.Context, id models.ResourceIdentifier) (*models.Service, error) {
	if id.Key() == m.service.Key() {
		return &m.service, nil
	}
	return nil, errors.New("service not found")
}

func (m *MockReaderForIEAgAgRules) GetServiceAliasByID(ctx context.Context, id models.ResourceIdentifier) (*models.ServiceAlias, error) {
	if id.Key() == m.serviceAlias.Key() {
		return &m.serviceAlias, nil
	}
	return nil, errors.New("service alias not found")
}

func (m *MockReaderForIEAgAgRules) GetAddressGroupByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroup, error) {
	if id.Key() == m.addressGroupLocal.Key() {
		return &m.addressGroupLocal, nil
	}
	if id.Key() == m.addressGroup.Key() {
		return &m.addressGroup, nil
	}
	return nil, errors.New("address group not found")
}

func (m *MockReaderForIEAgAgRules) GetRuleS2SByID(ctx context.Context, id models.ResourceIdentifier) (*models.RuleS2S, error) {
	if id.Key() == m.ruleS2S.Key() {
		return &m.ruleS2S, nil
	}
	return nil, errors.New("rule s2s not found")
}

func (m *MockReaderForIEAgAgRules) ListIEAgAgRules(ctx context.Context, consume func(models.IEAgAgRule) error, scope ports.Scope) error {
	for _, rule := range m.existingIEAgAgRules {
		if err := consume(rule); err != nil {
			return err
		}
	}
	return nil
}

func (m *MockReaderForIEAgAgRules) GetIEAgAgRuleByID(ctx context.Context, id models.ResourceIdentifier) (*models.IEAgAgRule, error) {
	for _, rule := range m.existingIEAgAgRules {
		if rule.Key() == id.Key() {
			return &rule, nil
		}
	}
	return nil, errors.New("IEAgAgRule not found")
}

func (m *MockReaderForIEAgAgRules) ListAddressGroupBindingPolicies(ctx context.Context, consume func(models.AddressGroupBindingPolicy) error, scope ports.Scope) error {
	return nil
}

func (m *MockReaderForIEAgAgRules) GetAddressGroupBindingPolicyByID(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroupBindingPolicy, error) {
	return nil, errors.New("address group binding policy not found")
}

// MockWriterForIEAgAgRules extends the MockWriter to support IEAgAgRules
type MockWriterForIEAgAgRules struct {
	*MockWriter
	syncedIEAgAgRules    []models.IEAgAgRule
	deletedIEAgAgRuleIDs []models.ResourceIdentifier
}

func NewMockWriterForIEAgAgRules() *MockWriterForIEAgAgRules {
	return &MockWriterForIEAgAgRules{
		MockWriter: NewMockWriter(),
	}
}

func (m *MockWriterForIEAgAgRules) SyncIEAgAgRules(ctx context.Context, rules []models.IEAgAgRule, scope ports.Scope, opts ...ports.Option) error {
	m.syncedIEAgAgRules = rules
	return nil
}

func (m *MockWriterForIEAgAgRules) DeleteIEAgAgRulesByIDs(ctx context.Context, ids []models.ResourceIdentifier, opts ...ports.Option) error {
	m.deletedIEAgAgRuleIDs = ids
	return nil
}

// TestGenerateIEAgAgRulesFromRuleS2S tests the GenerateIEAgAgRulesFromRuleS2S method
func TestGenerateIEAgAgRulesFromRuleS2S(t *testing.T) {
	// Create test data
	serviceID := models.NewResourceIdentifier("test-service")

	addressGroupLocalID := models.NewResourceIdentifier("test-ag-local")
	addressGroupLocal := models.AddressGroup{
		SelfRef:   models.SelfRef{ResourceIdentifier: addressGroupLocalID},
		Addresses: []string{"192.168.1.1/32"},
	}

	addressGroupID := models.NewResourceIdentifier("test-ag")
	addressGroup := models.AddressGroup{
		SelfRef:   models.SelfRef{ResourceIdentifier: addressGroupID},
		Addresses: []string{"10.0.0.1/32"},
	}

	service := models.Service{
		SelfRef: models.SelfRef{ResourceIdentifier: serviceID},
		IngressPorts: []models.IngressPort{
			{
				Protocol:    models.TCP,
				Port:        "80",
				Description: "HTTP",
			},
			{
				Protocol:    models.TCP,
				Port:        "443",
				Description: "HTTPS",
			},
		},
		AddressGroups: []models.AddressGroupRef{
			{ResourceIdentifier: addressGroupLocalID},
			{ResourceIdentifier: addressGroupID},
		},
	}

	aliasID := models.NewResourceIdentifier("test-alias")
	alias := models.ServiceAlias{
		SelfRef:    models.SelfRef{ResourceIdentifier: aliasID},
		ServiceRef: models.ServiceRef{ResourceIdentifier: serviceID},
	}

	// Test cases
	testCases := []struct {
		name           string
		ruleS2S        models.RuleS2S
		expectedCount  int
		expectedPorts  []string
		expectedAction models.RuleAction
	}{
		{
			name: "INGRESS rule",
			ruleS2S: models.RuleS2S{
				SelfRef:         models.SelfRef{ResourceIdentifier: models.NewResourceIdentifier("test-rule-ingress")},
				Traffic:         models.INGRESS,
				ServiceLocalRef: models.ServiceAliasRef{ResourceIdentifier: aliasID},
				ServiceRef:      models.ServiceAliasRef{ResourceIdentifier: aliasID},
			},
			expectedCount:  4,                  // One rule for each combination of address groups and protocol (2 address groups * 1 protocol (TCP) * 2 ports = 4)
			expectedPorts:  []string{"80,443"}, // Ports are combined in a single rule
			expectedAction: models.ActionAccept,
		},
		{
			name: "EGRESS rule",
			ruleS2S: models.RuleS2S{
				SelfRef:         models.SelfRef{ResourceIdentifier: models.NewResourceIdentifier("test-rule-egress")},
				Traffic:         models.EGRESS,
				ServiceLocalRef: models.ServiceAliasRef{ResourceIdentifier: aliasID},
				ServiceRef:      models.ServiceAliasRef{ResourceIdentifier: aliasID},
			},
			expectedCount:  4,                  // One rule for each combination of address groups and protocol (2 address groups * 1 protocol (TCP) * 2 ports = 4)
			expectedPorts:  []string{"80,443"}, // Ports are combined in a single rule
			expectedAction: models.ActionAccept,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup mock reader
			mockReader := NewMockReaderForIEAgAgRules()
			mockReader.service = service
			mockReader.serviceAlias = alias
			mockReader.addressGroupLocal = addressGroupLocal
			mockReader.addressGroup = addressGroup
			mockReader.ruleS2S = tc.ruleS2S

			// Setup mock registry
			mockRegistry := NewMockRegistryForIEAgAgRules(mockReader, &MockWriter{})

			// Create service
			netguardService := services.NewNetguardService(mockRegistry)

			// Call the method
			rules, err := netguardService.GenerateIEAgAgRulesFromRuleS2S(context.Background(), tc.ruleS2S)
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}

			// Check the results
			if len(rules) != tc.expectedCount {
				t.Errorf("Expected %d rules, got %d", tc.expectedCount, len(rules))
			}

			// Check that each rule has the expected properties
			portFound := make(map[string]bool)
			localAddressGroupsFound := make(map[string]bool)
			targetAddressGroupsFound := make(map[string]bool)

			for _, rule := range rules {
				// Check basic properties
				if rule.Traffic != tc.ruleS2S.Traffic {
					t.Errorf("Expected traffic %s, got %s", tc.ruleS2S.Traffic, rule.Traffic)
				}
				if rule.Action != tc.expectedAction {
					t.Errorf("Expected action %s, got %s", tc.expectedAction, rule.Action)
				}

				// Track address group references
				localAddressGroupsFound[rule.AddressGroupLocal.Key()] = true
				targetAddressGroupsFound[rule.AddressGroup.Key()] = true

				// Check ports
				if len(rule.Ports) != 1 {
					t.Errorf("Expected 1 port spec, got %d", len(rule.Ports))
				} else {
					portFound[rule.Ports[0].Destination] = true
				}
			}

			// Check that both address groups are found
			if !localAddressGroupsFound[addressGroupLocalID.Key()] {
				t.Errorf("Local address group %s not found in any rule", addressGroupLocalID.Key())
			}
			if !targetAddressGroupsFound[addressGroupID.Key()] {
				t.Errorf("Target address group %s not found in any rule", addressGroupID.Key())
			}

			// Check that all expected ports are found
			for _, port := range tc.expectedPorts {
				if !portFound[port] {
					t.Errorf("Expected port %s not found in generated rules", port)
				}
			}
		})
	}
}

// TestSyncRuleS2S_WithIEAgAgRules tests the SyncRuleS2S method with IEAgAgRules
func TestSyncRuleS2S_WithIEAgAgRules(t *testing.T) {
	// Create test data
	serviceID := models.NewResourceIdentifier("test-service")

	addressGroupLocalID := models.NewResourceIdentifier("test-ag-local")
	addressGroupLocal := models.AddressGroup{
		SelfRef:   models.SelfRef{ResourceIdentifier: addressGroupLocalID},
		Addresses: []string{"192.168.1.1/32"},
	}

	addressGroupID := models.NewResourceIdentifier("test-ag")
	addressGroup := models.AddressGroup{
		SelfRef:   models.SelfRef{ResourceIdentifier: addressGroupID},
		Addresses: []string{"10.0.0.1/32"},
	}

	service := models.Service{
		SelfRef: models.SelfRef{ResourceIdentifier: serviceID},
		IngressPorts: []models.IngressPort{
			{
				Protocol:    models.TCP,
				Port:        "80",
				Description: "HTTP",
			},
		},
		AddressGroups: []models.AddressGroupRef{
			{ResourceIdentifier: addressGroupLocalID},
			{ResourceIdentifier: addressGroupID},
		},
	}

	aliasID := models.NewResourceIdentifier("test-alias")
	alias := models.ServiceAlias{
		SelfRef:    models.SelfRef{ResourceIdentifier: aliasID},
		ServiceRef: models.ServiceRef{ResourceIdentifier: serviceID},
	}

	ruleID := models.NewResourceIdentifier("test-rule")
	rule := models.RuleS2S{
		SelfRef:         models.SelfRef{ResourceIdentifier: ruleID},
		Traffic:         models.INGRESS,
		ServiceLocalRef: models.ServiceAliasRef{ResourceIdentifier: aliasID},
		ServiceRef:      models.ServiceAliasRef{ResourceIdentifier: aliasID},
	}

	// Create existing IEAgAgRule (will be obsolete after sync)
	existingRuleID := models.NewResourceIdentifier("existing-rule")
	existingRule := models.IEAgAgRule{
		SelfRef:           models.SelfRef{ResourceIdentifier: existingRuleID},
		Transport:         models.TCP,
		Traffic:           models.INGRESS,
		AddressGroupLocal: models.AddressGroupRef{ResourceIdentifier: addressGroupLocalID},
		AddressGroup:      models.AddressGroupRef{ResourceIdentifier: addressGroupID},
		Ports: []models.PortSpec{
			{
				Destination: "8080", // Different port, will be obsolete
			},
		},
		Action:   models.ActionAccept,
		Logs:     true,
		Priority: 100,
	}

	// Setup mock reader
	mockReader := NewMockReaderForIEAgAgRules()
	mockReader.service = service
	mockReader.serviceAlias = alias
	mockReader.addressGroupLocal = addressGroupLocal
	mockReader.addressGroup = addressGroup
	mockReader.ruleS2S = rule
	mockReader.existingIEAgAgRules = []models.IEAgAgRule{existingRule}

	// Setup mock writer
	mockWriter := NewMockWriterForIEAgAgRules()

	// Setup mock registry
	mockRegistry := NewMockRegistryForIEAgAgRules(mockReader, mockWriter)

	// Create service
	netguardService := services.NewNetguardService(mockRegistry)

	// Call the method
	err := netguardService.SyncRuleS2S(context.Background(), []models.RuleS2S{rule}, nil)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Check that new IEAgAgRules were synced
	if len(mockWriter.syncedIEAgAgRules) != 4 {
		t.Errorf("Expected 4 synced IEAgAgRules, got %d", len(mockWriter.syncedIEAgAgRules))
	} else {
		// Check that at least one rule has the expected port
		portFound := false
		for _, syncedRule := range mockWriter.syncedIEAgAgRules {
			if syncedRule.Ports[0].Destination == "80" {
				portFound = true
				break
			}
		}
		if !portFound {
			t.Errorf("Expected at least one rule with port 80, but none found")
		}
	}

	// Check that obsolete IEAgAgRules were deleted
	if len(mockWriter.deletedIEAgAgRuleIDs) != 1 {
		t.Errorf("Expected 1 deleted IEAgAgRule, got %d", len(mockWriter.deletedIEAgAgRuleIDs))
	} else {
		deletedID := mockWriter.deletedIEAgAgRuleIDs[0]
		if deletedID.Key() != existingRuleID.Key() {
			t.Errorf("Expected deleted rule ID %s, got %s", existingRuleID.Key(), deletedID.Key())
		}
	}
}

// TestRuleS2SAndIEAgAgRuleIntegration tests the integration between RuleS2S and IEAgAgRule
func TestRuleS2SAndIEAgAgRuleIntegration(t *testing.T) {
	// Create test data
	serviceID := models.NewResourceIdentifier("test-service")

	addressGroupLocalID := models.NewResourceIdentifier("test-ag-local")
	addressGroupLocal := models.AddressGroup{
		SelfRef:   models.SelfRef{ResourceIdentifier: addressGroupLocalID},
		Addresses: []string{"192.168.1.1/32"},
	}

	addressGroupID := models.NewResourceIdentifier("test-ag")
	addressGroup := models.AddressGroup{
		SelfRef:   models.SelfRef{ResourceIdentifier: addressGroupID},
		Addresses: []string{"10.0.0.1/32"},
	}

	service := models.Service{
		SelfRef: models.SelfRef{ResourceIdentifier: serviceID},
		IngressPorts: []models.IngressPort{
			{
				Protocol:    models.TCP,
				Port:        "80",
				Description: "HTTP",
			},
		},
		AddressGroups: []models.AddressGroupRef{
			{ResourceIdentifier: addressGroupLocalID},
			{ResourceIdentifier: addressGroupID},
		},
	}

	aliasID := models.NewResourceIdentifier("test-alias")
	alias := models.ServiceAlias{
		SelfRef:    models.SelfRef{ResourceIdentifier: aliasID},
		ServiceRef: models.ServiceRef{ResourceIdentifier: serviceID},
	}

	ruleID := models.NewResourceIdentifier("test-rule")
	rule := models.RuleS2S{
		SelfRef:         models.SelfRef{ResourceIdentifier: ruleID},
		Traffic:         models.INGRESS,
		ServiceLocalRef: models.ServiceAliasRef{ResourceIdentifier: aliasID},
		ServiceRef:      models.ServiceAliasRef{ResourceIdentifier: aliasID},
	}

	// Test cases for related entity changes
	testCases := []struct {
		name                string
		updateEntity        string
		updateFunc          func(*services.NetguardService) error
		expectedRuleUpdated bool
	}{
		{
			name:         "Update Service",
			updateEntity: "Service",
			updateFunc: func(s *services.NetguardService) error {
				// Update service with new port
				updatedService := service
				updatedService.IngressPorts = []models.IngressPort{
					{
						Protocol:    models.TCP,
						Port:        "8080", // Changed port
						Description: "New HTTP",
					},
				}
				return s.SyncServices(context.Background(), []models.Service{updatedService}, nil)
			},
			expectedRuleUpdated: true,
		},
		{
			name:         "Update AddressGroup",
			updateEntity: "AddressGroup",
			updateFunc: func(s *services.NetguardService) error {
				// Update address group with new address
				updatedAddressGroup := addressGroup
				updatedAddressGroup.Addresses = []string{"10.0.0.2/32"} // Changed address
				return s.SyncAddressGroups(context.Background(), []models.AddressGroup{updatedAddressGroup}, nil)
			},
			expectedRuleUpdated: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup mock reader
			mockReader := NewMockReaderForIEAgAgRules()
			mockReader.service = service
			mockReader.serviceAlias = alias
			mockReader.addressGroupLocal = addressGroupLocal
			mockReader.addressGroup = addressGroup
			mockReader.ruleS2S = rule

			// Setup mock writer
			mockWriter := NewMockWriterForIEAgAgRules()

			// Setup mock registry
			mockRegistry := NewMockRegistryForIEAgAgRules(mockReader, mockWriter)

			// Create service
			netguardService := services.NewNetguardService(mockRegistry)

			// First, sync the rule to create initial IEAgAgRules
			err := netguardService.SyncRuleS2S(context.Background(), []models.RuleS2S{rule}, nil)
			if err != nil {
				t.Fatalf("Expected no error syncing rule, got %v", err)
			}

			// Clear the writer's state
			mockWriter.syncedIEAgAgRules = nil
			mockWriter.deletedIEAgAgRuleIDs = nil

			// Now update the related entity
			err = tc.updateFunc(netguardService)
			if err != nil {
				t.Fatalf("Expected no error updating %s, got %v", tc.updateEntity, err)
			}

			// Check if IEAgAgRules were updated
			if tc.expectedRuleUpdated {
				if len(mockWriter.syncedIEAgAgRules) == 0 {
					t.Errorf("Expected IEAgAgRules to be updated after %s change, but none were synced", tc.updateEntity)
				}
			} else {
				if len(mockWriter.syncedIEAgAgRules) > 0 {
					t.Errorf("Expected no IEAgAgRules updates after %s change, but %d were synced", tc.updateEntity, len(mockWriter.syncedIEAgAgRules))
				}
			}
		})
	}
}
