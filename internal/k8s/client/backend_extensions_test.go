package client

import (
	"context"
	"testing"
	"time"

	"netguard-pg-backend/internal/domain/models"
)

// TestPhase4_1_BackendClientExtensions тестирует новые методы, добавленные в Phase 4.1
func TestPhase4_1_BackendClientExtensions(t *testing.T) {
	client := NewMockBackendClient()
	ctx := context.Background()

	t.Run("Ping method", func(t *testing.T) {
		err := client.Ping(ctx)
		if err != nil {
			t.Errorf("Ping() failed: %v", err)
		}
	})

	t.Run("UpdateMeta methods", func(t *testing.T) {
		// Создаем тестовый сервис
		serviceID := models.NewResourceIdentifier("test-service", models.WithNamespace("test-ns"))
		testService := &models.Service{
			SelfRef:     models.SelfRef{ResourceIdentifier: serviceID},
			Description: "Test service for meta update",
		}

		err := client.CreateService(ctx, testService)
		if err != nil {
			t.Fatalf("Failed to create test service: %v", err)
		}

		// Тестируем UpdateServiceMeta
		meta := models.Meta{
			Labels: map[string]string{
				"phase": "4.1",
				"test":  "meta-update",
			},
			Annotations: map[string]string{
				"netguard.io/updated": "true",
			},
		}

		err = client.UpdateServiceMeta(ctx, serviceID, meta)
		if err != nil {
			t.Errorf("UpdateServiceMeta() failed: %v", err)
		}

		// Проверяем, что meta обновилась
		updatedService, err := client.GetService(ctx, serviceID)
		if err != nil {
			t.Fatalf("Failed to get updated service: %v", err)
		}

		if updatedService.Meta.Labels["phase"] != "4.1" {
			t.Errorf("Expected label 'phase'='4.1', got '%s'", updatedService.Meta.Labels["phase"])
		}
	})

	t.Run("Helper methods for subresources", func(t *testing.T) {
		serviceID := models.NewResourceIdentifier("test-service-1", models.WithNamespace("netguard-test"))

		// Тестируем ListAddressGroupsForService
		addressGroups, err := client.ListAddressGroupsForService(ctx, serviceID)
		if err != nil {
			t.Errorf("ListAddressGroupsForService() failed: %v", err)
		}

		if len(addressGroups) == 0 {
			t.Log("No address groups found for test-service-1 (expected for mock)")
		}

		// Тестируем ListRuleS2SDstOwnRef
		rules, err := client.ListRuleS2SDstOwnRef(ctx, serviceID)
		if err != nil {
			t.Errorf("ListRuleS2SDstOwnRef() failed: %v", err)
		}

		if len(rules) > 0 {
			rule := rules[0]
			if rule.ServiceRef.Name != serviceID.Name {
				t.Errorf("Expected ServiceRef.Name '%s', got '%s'", serviceID.Name, rule.ServiceRef.Name)
			}
			if rule.ServiceRef.Namespace != serviceID.Namespace {
				t.Errorf("Expected ServiceRef.Namespace '%s', got '%s'", serviceID.Namespace, rule.ServiceRef.Namespace)
			}
		}

		// Тестируем ListAccessPorts
		mappingID := models.NewResourceIdentifier("test-mapping", models.WithNamespace("netguard-test"))
		servicePortsRefs, err := client.ListAccessPorts(ctx, mappingID)
		if err != nil {
			t.Errorf("ListAccessPorts() failed: %v", err)
		}

		if len(servicePortsRefs) > 0 {
			portRef := servicePortsRefs[0]
			if portRef.ServiceRef.Name != "test-service-1" {
				t.Errorf("Expected ServiceRef.Name 'test-service-1', got '%s'", portRef.ServiceRef.Name)
			}

			if len(portRef.Ports.Ports[models.TCP]) == 0 {
				t.Error("Expected TCP ports in ServicePortsRef")
			}
		}
	})

	t.Run("All BackendClient methods compile", func(t *testing.T) {
		// Проверяем, что все методы интерфейса BackendClient реализованы
		var _ BackendClient = client
		t.Log("✅ All BackendClient interface methods are implemented")
	})
}

// TestPhase4_1_PerformanceComparison тестирует различие в производительности между HealthCheck и Ping
func TestPhase4_1_PerformanceComparison(t *testing.T) {
	client := NewMockBackendClient()
	ctx := context.Background()

	// Тестируем Ping
	start := time.Now()
	err := client.Ping(ctx)
	pingDuration := time.Since(start)
	if err != nil {
		t.Errorf("Ping() failed: %v", err)
	}

	// Тестируем HealthCheck
	start = time.Now()
	err = client.HealthCheck(ctx)
	healthCheckDuration := time.Since(start)
	if err != nil {
		t.Errorf("HealthCheck() failed: %v", err)
	}

	t.Logf("Ping duration: %v", pingDuration)
	t.Logf("HealthCheck duration: %v", healthCheckDuration)

	// В mock оба должны быть быстрыми, но это демонстрирует различие в API
	if pingDuration > time.Second {
		t.Errorf("Ping took too long: %v", pingDuration)
	}
	if healthCheckDuration > time.Second {
		t.Errorf("HealthCheck took too long: %v", healthCheckDuration)
	}
}
