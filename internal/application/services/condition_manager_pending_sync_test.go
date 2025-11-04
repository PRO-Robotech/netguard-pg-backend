package services

import (
	"context"
	"testing"

	"netguard-pg-backend/internal/application/services/resources/testutil"
	"netguard-pg-backend/internal/domain/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// =============================================================================
// TESTS: Network - ConditionManager должен сохранять Ready=False для новых ресурсов
// =============================================================================

func TestConditionManager_Network_KeepsPendingSyncStatus(t *testing.T) {
	t.Log("🧪 TEST: Network с PendingSGROUPSync НЕ должен получить Ready=True от ConditionManager")

	// SETUP: Создаем Network как Writer (с Ready=False, reason=PendingSGROUPSync)
	network := &models.Network{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      "test-network",
				Namespace: "default",
			},
		},
		CIDR: "10.0.0.0/24",
		Meta: models.Meta{
			Conditions: []metav1.Condition{
				{
					Type:    "Ready",
					Status:  metav1.ConditionFalse,
					Reason:  "PendingSGROUPSync",
					Message: "Awaiting synchronization with SGROUP before marking as ready",
				},
			},
		},
	}

	t.Logf("  ✅ Writer создал Network с Ready=False (reason=%s)", network.Meta.GetCondition("Ready").Reason)

	// SETUP: Создаем ConditionManager с mock registry
	mockReg := testutil.NewMockRegistry()
	cm := NewConditionManager(mockReg)

	// ACTION: Вызываем ProcessNetworkConditions (как это делает NetguardFacade после commit)
	ctx := context.Background()
	err := cm.ProcessNetworkConditions(ctx, network, nil)
	require.NoError(t, err, "ProcessNetworkConditions не должен возвращать ошибку")

	// ASSERTIONS: Проверяем, что Ready ВСЕ ЕЩЕ False
	readyCond := network.Meta.GetCondition("Ready")
	require.NotNil(t, readyCond, "Ready condition должен существовать")

	assert.Equal(t, metav1.ConditionFalse, readyCond.Status,
		"❌ КРИТИЧЕСКИЙ БАГ: ConditionManager изменил Ready=False → Ready=True! Это нарушает Sync-First принцип!")

	assert.Equal(t, "PendingSGROUPSync", readyCond.Reason,
		"❌ КРИТИЧЕСКИЙ БАГ: ConditionManager изменил reason! Ready должен оставаться PendingSGROUPSync до Worker sync!")

	// ASSERTIONS: Проверяем, что Validated установлен (ConditionManager делает валидацию)
	validatedCond := network.Meta.GetCondition("Validated")
	require.NotNil(t, validatedCond, "Validated condition должен быть установлен")
	assert.Equal(t, metav1.ConditionTrue, validatedCond.Status, "Validated должен быть True")

	// ASSERTIONS: Проверяем, что Synced НЕ установлен (т.к. ресурс новый, синхронизации еще не было)
	syncedCond := network.Meta.GetCondition("Synced")
	if syncedCond != nil {
		assert.NotEqual(t, metav1.ConditionTrue, syncedCond.Status,
			"Synced НЕ должен быть True для новых ресурсов (синхронизация еще не выполнена)")
	}

	t.Log("  ✅ PASSED: ConditionManager НЕ изменил Ready=False на Ready=True")
	t.Log("  ✅ PASSED: Ready.Reason остался PendingSGROUPSync")
	t.Log("  ✅ PASSED: Validated=True установлен")
	t.Log("  ✅ SUCCESS: Network остается в состоянии ожидания Worker sync")
}

// =============================================================================
// TESTS: AddressGroup - ConditionManager должен сохранять Ready=False для новых ресурсов
// =============================================================================

func TestConditionManager_AddressGroup_KeepsPendingSyncStatus(t *testing.T) {
	t.Log("🧪 TEST: AddressGroup с PendingSGROUPSync НЕ должен получить Ready=True от ConditionManager")

	// SETUP: Создаем AddressGroup как Writer (с Ready=False, reason=PendingSGROUPSync)
	ag := &models.AddressGroup{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      "test-ag",
				Namespace: "default",
			},
		},
		Meta: models.Meta{
			Conditions: []metav1.Condition{
				{
					Type:    "Ready",
					Status:  metav1.ConditionFalse,
					Reason:  "PendingSGROUPSync",
					Message: "Awaiting synchronization with SGROUP before marking as ready",
				},
			},
		},
	}

	t.Logf("  ✅ Writer создал AddressGroup с Ready=False (reason=%s)", ag.Meta.GetCondition("Ready").Reason)

	// SETUP: Создаем ConditionManager с mock registry
	mockReg := testutil.NewMockRegistry()
	cm := NewConditionManager(mockReg)

	// ACTION: Вызываем ProcessAddressGroupConditions
	ctx := context.Background()
	err := cm.ProcessAddressGroupConditions(ctx, ag)
	require.NoError(t, err, "ProcessAddressGroupConditions не должен возвращать ошибку")

	// ASSERTIONS: Проверяем, что Ready ВСЕ ЕЩЕ False
	readyCond := ag.Meta.GetCondition("Ready")
	require.NotNil(t, readyCond, "Ready condition должен существовать")

	assert.Equal(t, metav1.ConditionFalse, readyCond.Status,
		"❌ КРИТИЧЕСКИЙ БАГ: ConditionManager изменил Ready=False → Ready=True для AddressGroup!")

	assert.Equal(t, "PendingSGROUPSync", readyCond.Reason,
		"❌ КРИТИЧЕСКИЙ БАГ: ConditionManager изменил reason для AddressGroup!")

	// ASSERTIONS: Проверяем, что Validated установлен
	validatedCond := ag.Meta.GetCondition("Validated")
	require.NotNil(t, validatedCond, "Validated condition должен быть установлен")
	assert.Equal(t, metav1.ConditionTrue, validatedCond.Status, "Validated должен быть True")

	// ASSERTIONS: Проверяем, что Synced НЕ установлен для новых ресурсов
	syncedCond := ag.Meta.GetCondition("Synced")
	if syncedCond != nil {
		assert.NotEqual(t, metav1.ConditionTrue, syncedCond.Status,
			"Synced НЕ должен быть True для новых AddressGroup")
	}

	t.Log("  ✅ PASSED: ConditionManager НЕ изменил Ready=False на Ready=True")
	t.Log("  ✅ PASSED: Ready.Reason остался PendingSGROUPSync")
	t.Log("  ✅ PASSED: Validated=True установлен")
	t.Log("  ✅ SUCCESS: AddressGroup остается в состоянии ожидания Worker sync")
}

// =============================================================================
// TESTS: Host - ConditionManager должен сохранять Ready=False для новых ресурсов
// =============================================================================

func TestConditionManager_Host_KeepsPendingSyncStatus(t *testing.T) {
	t.Log("🧪 TEST: Host с PendingSGROUPSync НЕ должен получить Ready=True от ConditionManager")

	// SETUP: Создаем Host как Writer (с Ready=False, reason=PendingSGROUPSync)
	host := &models.Host{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      "test-host",
				Namespace: "default",
			},
		},
		UUID: "550e8400-e29b-41d4-a716-446655440000",
		Meta: models.Meta{
			Conditions: []metav1.Condition{
				{
					Type:    "Ready",
					Status:  metav1.ConditionFalse,
					Reason:  "PendingSGROUPSync",
					Message: "Awaiting synchronization with SGROUP before marking as ready",
				},
			},
		},
	}

	t.Logf("  ✅ Writer создал Host с Ready=False (reason=%s)", host.Meta.GetCondition("Ready").Reason)

	// SETUP: Создаем ConditionManager с mock registry
	mockReg := testutil.NewMockRegistry()
	cm := NewConditionManager(mockReg)

	// ACTION: Вызываем ProcessHostConditions
	ctx := context.Background()
	err := cm.ProcessHostConditions(ctx, host, nil)
	require.NoError(t, err, "ProcessHostConditions не должен возвращать ошибку")

	// ASSERTIONS: Проверяем, что Ready ВСЕ ЕЩЕ False
	readyCond := host.Meta.GetCondition("Ready")
	require.NotNil(t, readyCond, "Ready condition должен существовать")

	assert.Equal(t, metav1.ConditionFalse, readyCond.Status,
		"❌ КРИТИЧЕСКИЙ БАГ: ConditionManager изменил Ready=False → Ready=True для Host!")

	assert.Equal(t, "PendingSGROUPSync", readyCond.Reason,
		"❌ КРИТИЧЕСКИЙ БАГ: ConditionManager изменил reason для Host!")

	// ASSERTIONS: Проверяем, что Validated установлен
	validatedCond := host.Meta.GetCondition("Validated")
	require.NotNil(t, validatedCond, "Validated condition должен быть установлен")
	assert.Equal(t, metav1.ConditionTrue, validatedCond.Status, "Validated должен быть True")

	// ASSERTIONS: Проверяем, что Synced НЕ установлен для новых ресурсов
	syncedCond := host.Meta.GetCondition("Synced")
	if syncedCond != nil {
		assert.NotEqual(t, metav1.ConditionTrue, syncedCond.Status,
			"Synced НЕ должен быть True для новых Host")
	}

	t.Log("  ✅ PASSED: ConditionManager НЕ изменил Ready=False на Ready=True")
	t.Log("  ✅ PASSED: Ready.Reason остался PendingSGROUPSync")
	t.Log("  ✅ PASSED: Validated=True установлен")
	t.Log("  ✅ SUCCESS: Host остается в состоянии ожидания Worker sync")
}

// =============================================================================
// TESTS: OLD BEHAVIOR - Существующие ресурсы (уже синхронизированные) получают Ready=True
// =============================================================================

func TestConditionManager_Network_ExistingResource_SetsReadyTrue(t *testing.T) {
	t.Log("🧪 TEST: Существующий Network (БЕЗ PendingSGROUPSync) должен получить Ready=True")

	// SETUP: Создаем Network как существующий ресурс (уже был синхронизирован)
	network := &models.Network{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      "existing-network",
				Namespace: "default",
			},
		},
		CIDR: "10.0.0.0/24",
		Meta: models.Meta{
			Conditions: []metav1.Condition{
				// Нет PendingSGROUPSync - это существующий ресурс
				{
					Type:    "Ready",
					Status:  metav1.ConditionTrue,
					Reason:  "Ready",
					Message: "Network is ready for use",
				},
			},
		},
	}

	t.Log("  ✅ Existing Network (без PendingSGROUPSync)")

	// SETUP: Создаем ConditionManager
	mockReg := testutil.NewMockRegistry()
	cm := NewConditionManager(mockReg)

	// ACTION: Вызываем ProcessNetworkConditions
	ctx := context.Background()
	err := cm.ProcessNetworkConditions(ctx, network, nil)
	require.NoError(t, err)

	// ASSERTIONS: Проверяем, что Ready=True (старое поведение для существующих)
	readyCond := network.Meta.GetCondition("Ready")
	require.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionTrue, readyCond.Status,
		"Существующий Network должен получить Ready=True (OLD BEHAVIOR)")

	t.Log("  ✅ PASSED: Существующий Network получил Ready=True")
}

func TestConditionManager_AddressGroup_ExistingResource_SetsReadyTrue(t *testing.T) {
	t.Log("🧪 TEST: Существующая AddressGroup (БЕЗ PendingSGROUPSync) должна получить Ready=True")

	// SETUP: Существующая AddressGroup
	ag := &models.AddressGroup{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      "existing-ag",
				Namespace: "default",
			},
		},
		Meta: models.Meta{
			Conditions: []metav1.Condition{
				{
					Type:    "Ready",
					Status:  metav1.ConditionTrue,
					Reason:  "Ready",
					Message: "AddressGroup is ready",
				},
			},
		},
	}

	t.Log("  ✅ Existing AddressGroup (без PendingSGROUPSync)")

	mockReg := testutil.NewMockRegistry()
	cm := NewConditionManager(mockReg)

	ctx := context.Background()
	err := cm.ProcessAddressGroupConditions(ctx, ag)
	require.NoError(t, err)

	readyCond := ag.Meta.GetCondition("Ready")
	require.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionTrue, readyCond.Status,
		"Существующая AddressGroup должна получить Ready=True (OLD BEHAVIOR)")

	t.Log("  ✅ PASSED: Существующая AddressGroup получила Ready=True")
}

func TestConditionManager_Host_ExistingResource_SetsReadyTrue(t *testing.T) {
	t.Log("🧪 TEST: Существующий Host (БЕЗ PendingSGROUPSync) должен получить Ready=True")

	// SETUP: Существующий Host
	host := &models.Host{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      "existing-host",
				Namespace: "default",
			},
		},
		UUID: "550e8400-e29b-41d4-a716-446655440000",
		Meta: models.Meta{
			Conditions: []metav1.Condition{
				{
					Type:    "Ready",
					Status:  metav1.ConditionTrue,
					Reason:  "Ready",
					Message: "Host is ready",
				},
			},
		},
	}

	t.Log("  ✅ Existing Host (без PendingSGROUPSync)")

	mockReg := testutil.NewMockRegistry()
	cm := NewConditionManager(mockReg)

	ctx := context.Background()
	err := cm.ProcessHostConditions(ctx, host, nil)
	require.NoError(t, err)

	readyCond := host.Meta.GetCondition("Ready")
	require.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionTrue, readyCond.Status,
		"Существующий Host должен получить Ready=True (OLD BEHAVIOR)")

	t.Log("  ✅ PASSED: Существующий Host получил Ready=True")
}
