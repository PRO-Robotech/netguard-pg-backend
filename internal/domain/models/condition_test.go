package models

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestMeta_SetCondition проверяет установку и обновление условий
func TestMeta_SetCondition(t *testing.T) {
	meta := &Meta{}

	// Устанавливаем первое условие
	condition1 := metav1.Condition{
		Type:               ConditionReady,
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             ReasonReady,
		Message:            "Resource is ready",
	}

	meta.SetCondition(condition1)

	if len(meta.Conditions) != 1 {
		t.Errorf("Expected 1 condition, got %d", len(meta.Conditions))
	}

	if meta.Conditions[0].Type != ConditionReady {
		t.Errorf("Expected condition type %s, got %s", ConditionReady, meta.Conditions[0].Type)
	}

	// Обновляем то же условие
	condition1Updated := metav1.Condition{
		Type:               ConditionReady,
		Status:             metav1.ConditionFalse,
		LastTransitionTime: metav1.Now(),
		Reason:             ReasonNotReady,
		Message:            "Resource is not ready",
	}

	meta.SetCondition(condition1Updated)

	if len(meta.Conditions) != 1 {
		t.Errorf("Expected 1 condition after update, got %d", len(meta.Conditions))
	}

	if meta.Conditions[0].Status != metav1.ConditionFalse {
		t.Errorf("Expected condition status %s, got %s", metav1.ConditionFalse, meta.Conditions[0].Status)
	}

	// Добавляем второе условие
	condition2 := metav1.Condition{
		Type:               ConditionSynced,
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             ReasonSynced,
		Message:            "Resource is synced",
	}

	meta.SetCondition(condition2)

	if len(meta.Conditions) != 2 {
		t.Errorf("Expected 2 conditions, got %d", len(meta.Conditions))
	}
}

// TestMeta_GetCondition проверяет получение условий
func TestMeta_GetCondition(t *testing.T) {
	meta := &Meta{}

	// Проверяем получение несуществующего условия
	condition := meta.GetCondition(ConditionReady)
	if condition != nil {
		t.Errorf("Expected nil condition, got %v", condition)
	}

	// Добавляем условие и проверяем его получение
	testCondition := metav1.Condition{
		Type:               ConditionReady,
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             ReasonReady,
		Message:            "Resource is ready",
	}

	meta.SetCondition(testCondition)

	retrievedCondition := meta.GetCondition(ConditionReady)
	if retrievedCondition == nil {
		t.Fatal("Expected condition, got nil")
	}

	if retrievedCondition.Type != ConditionReady {
		t.Errorf("Expected condition type %s, got %s", ConditionReady, retrievedCondition.Type)
	}

	if retrievedCondition.Status != metav1.ConditionTrue {
		t.Errorf("Expected condition status %s, got %s", metav1.ConditionTrue, retrievedCondition.Status)
	}
}

// TestMeta_IsConditionTrue проверяет проверку истинности условий
func TestMeta_IsConditionTrue(t *testing.T) {
	meta := &Meta{}

	// Проверяем несуществующее условие
	if meta.IsConditionTrue(ConditionReady) {
		t.Error("Expected false for non-existent condition")
	}

	// Добавляем условие со статусом True
	meta.SetReadyCondition(metav1.ConditionTrue, ReasonReady, "Ready")

	if !meta.IsConditionTrue(ConditionReady) {
		t.Error("Expected true for Ready condition")
	}

	// Добавляем условие со статусом False
	meta.SetSyncedCondition(metav1.ConditionFalse, ReasonSyncFailed, "Not synced")

	if meta.IsConditionTrue(ConditionSynced) {
		t.Error("Expected false for Synced condition")
	}
}

// TestMeta_HelperMethods проверяет helper методы для условий
func TestMeta_HelperMethods(t *testing.T) {
	meta := &Meta{}

	// Проверяем начальное состояние
	if meta.IsReady() {
		t.Error("Expected not ready initially")
	}

	if meta.IsSynced() {
		t.Error("Expected not synced initially")
	}

	if meta.IsValidated() {
		t.Error("Expected not validated initially")
	}

	if meta.HasError() {
		t.Error("Expected no error initially")
	}

	// Устанавливаем условия
	meta.SetReadyCondition(metav1.ConditionTrue, ReasonReady, "Ready")
	meta.SetSyncedCondition(metav1.ConditionTrue, ReasonSynced, "Synced")
	meta.SetValidatedCondition(metav1.ConditionTrue, ReasonValidated, "Validated")

	// Проверяем
	if !meta.IsReady() {
		t.Error("Expected ready")
	}

	if !meta.IsSynced() {
		t.Error("Expected synced")
	}

	if !meta.IsValidated() {
		t.Error("Expected validated")
	}

	// Устанавливаем ошибку
	meta.SetErrorCondition(ReasonError, "Test error")

	if !meta.HasError() {
		t.Error("Expected error")
	}

	// Очищаем ошибку
	meta.ClearErrorCondition()

	if meta.HasError() {
		t.Error("Expected no error after clearing")
	}
}

// TestMeta_TouchMethods проверяет методы обновления метаданных
func TestMeta_TouchMethods(t *testing.T) {
	meta := &Meta{}

	// Проверяем TouchOnCreate
	meta.TouchOnCreate()

	if meta.UID == "" {
		t.Error("Expected UID to be set")
	}

	if meta.Generation != 1 {
		t.Errorf("Expected generation 1, got %d", meta.Generation)
	}

	if meta.ObservedGeneration != 1 {
		t.Errorf("Expected observed generation 1, got %d", meta.ObservedGeneration)
	}

	if meta.CreationTS.IsZero() {
		t.Error("Expected creation timestamp to be set")
	}

	// Проверяем TouchOnWrite
	meta.Generation = 2
	meta.TouchOnWrite("v2")

	if meta.ResourceVersion != "v2" {
		t.Errorf("Expected resource version v2, got %s", meta.ResourceVersion)
	}

	if meta.ObservedGeneration != 2 {
		t.Errorf("Expected observed generation 2, got %d", meta.ObservedGeneration)
	}
}

// TestConditionHelpers проверяет helper функции для создания условий
func TestConditionHelpers(t *testing.T) {
	// Проверяем NewReadyCondition
	condition := NewReadyCondition(metav1.ConditionTrue, ReasonReady, "Test message")

	if condition.Type != ConditionReady {
		t.Errorf("Expected type %s, got %s", ConditionReady, condition.Type)
	}

	if condition.Status != metav1.ConditionTrue {
		t.Errorf("Expected status %s, got %s", metav1.ConditionTrue, condition.Status)
	}

	if condition.Reason != ReasonReady {
		t.Errorf("Expected reason %s, got %s", ReasonReady, condition.Reason)
	}

	if condition.Message != "Test message" {
		t.Errorf("Expected message 'Test message', got %s", condition.Message)
	}

	if condition.LastTransitionTime.IsZero() {
		t.Error("Expected last transition time to be set")
	}

	// Проверяем NewErrorCondition
	errorCondition := NewErrorCondition(ReasonError, "Test error")

	if errorCondition.Type != ConditionError {
		t.Errorf("Expected type %s, got %s", ConditionError, errorCondition.Type)
	}

	if errorCondition.Status != metav1.ConditionTrue {
		t.Errorf("Expected status %s, got %s", metav1.ConditionTrue, errorCondition.Status)
	}
}
