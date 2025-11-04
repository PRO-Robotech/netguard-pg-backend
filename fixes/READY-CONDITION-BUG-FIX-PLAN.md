# Fix Plan: Host Ready=True Before SGROUP Sync Bug

**Дата создания**: 2025-10-16
**Приоритет**: P0 CRITICAL
**Статус**: PLAN CREATED - READY FOR EXECUTION

---

## Описание Проблемы

**Симптом**: При создании нового Host ресурса через kubectl, условие `Ready=True` устанавливается НЕМЕДЛЕННО, ДО того как происходит синхронизация с SGROUP.

**Ожидаемое поведение**:
- После создания: `Ready=False`, `PendingSync=True`
- После успешной синхронизации с SGROUP: `Ready=True`, `PendingSync=False`

**Фактическое поведение**:
- Сразу после создания: `Ready=True` (НЕПРАВИЛЬНО!)

**Почему это критично**:
- Нарушает семантику Ready condition
- Ready=True означает "синхронизирован с внешней системой"
- Клиенты полагаются на Ready condition для определения готовности ресурса
- Может привести к race conditions в dependent системах

**Обнаружено**: Во время тестирования SGROUP availability scenarios
**Дата**: 2025-10-15

---

## Root Cause Hypothesis

**Подозреваемый код**: `internal/infrastructure/repositories/pg/writers/host.go`, lines 63-72

```go
// Check if host exists to determine if this is INSERT or UPDATE
var existingResourceVersion sql.NullInt64
checkQuery := `SELECT resource_version FROM hosts WHERE namespace = $1 AND name = $2`
err = w.tx.QueryRow(ctx, checkQuery, host.Namespace, host.Name).Scan(&existingResourceVersion)

// CRITICAL FIX: Force Ready=False for new resources (INSERT path)
conditions := host.Meta.Conditions
if !existingResourceVersion.Valid {
	// This is a new resource - force Pending status
	conditions = forcePendingSyncCondition(conditions)  // ← ПОДОЗРЕВАЕМАЯ ФУНКЦИЯ
	klog.V(4).InfoS("Forcing PendingSGROUPSync status for new Host", ...)
}
```

**Гипотеза**: Функция `forcePendingSyncCondition()` может:
1. НЕ вызываться для новых Host (логика условия неверна)
2. Вызываться, но НЕ устанавливать Ready=False корректно
3. Вызываться, но её результат затирается позже в коде

---

## Стратегия Тестирования

**КЛЮЧЕВОЕ ЗАМЕЧАНИЕ**: Синхронизация с SGROUP происходит очень быстро (< 1 секунды), поэтому для проверки начального состояния необходимо тестировать с **ВЫКЛЮЧЕННЫМ SGROUP**.

**Методы**:
1. **Minikube с SGROUP disabled** - "заморозить" процесс синка
2. **Local integration test** - проверять conditions сразу после создания, ДО OutboxWorker

---

## План Исправления

### Part 1: Подтверждение Бага в Minikube 🔍

**Цель**: Подтвердить что Ready=True устанавливается ДО синка с SGROUP

**Шаги**:

#### 1.1 Выключить SGROUP (заморозить sync)
```bash
kubectl scale deployment sgroups-api --replicas=0 -n incloud-sgroups
kubectl get pods -n incloud-sgroups | grep sgroups-api
# Ожидаем: 0 pods (SGROUP выключен)
```

#### 1.2 Создать новый Host
```bash
cat <<EOF | kubectl apply -f -
apiVersion: netguard.sgroups.io/v1beta1
kind: Host
metadata:
  name: ready-condition-test-host
  namespace: incloud-sgroups
spec:
  uuid: "11111111-2222-3333-4444-555555555999"
EOF
```

#### 1.3 НЕМЕДЛЕННО проверить conditions (через 2-3 секунды)
```bash
sleep 3
kubectl get host ready-condition-test-host -n incloud-sgroups -o jsonpath='{.status.conditions}' | jq '.[] | select(.type=="Ready" or .type=="PendingSync")'
```

**Ожидаемый результат (ПРАВИЛЬНЫЙ)**:
```json
{
  "type": "Ready",
  "status": "False",
  "reason": "PendingSGROUPSync",
  "message": "Waiting for SGROUP synchronization"
}
{
  "type": "PendingSync",
  "status": "True",
  "reason": "WaitingForSync"
}
```

**Если баг существует (НЕПРАВИЛЬНЫЙ)**:
```json
{
  "type": "Ready",
  "status": "True",  // ❌ НЕПРАВИЛЬНО! Должно быть False!
  "reason": "Ready",
  "message": "Synced to SGROUP"
}
```

#### 1.4 Проверить БД (убедиться что sync НЕ произошёл)
```bash
kubectl exec -n incloud-sgroups netguard-postgresql-0 -- \
  env PGPASSWORD=1mNym19CYv psql -U postgres -d netguard \
  -c "SELECT operation, status, error FROM sync_outbox WHERE entity_type='Host' AND entity_name='ready-condition-test-host';"
```

**Ожидаем**: status = 'pending' (ещё не обработан OutboxWorker)

#### 1.5 Cleanup
```bash
kubectl delete host ready-condition-test-host -n incloud-sgroups
kubectl scale deployment sgroups-api --replicas=1 -n incloud-sgroups
```

**Критерий успеха**: Если Ready=True при выключенном SGROUP → баг подтверждён

---

### Part 2: Создание Local Integration Test 🧪

**Цель**: Создать автоматический тест который ловит этот баг БЕЗ minikube deployment

**Файл**: `internal/sync/integration/host_initial_conditions_test.go`

**Структура теста**:

```go
// TestHost_InitialConditions_ReadyFalseBeforeSync validates that a newly created Host
// has Ready=False and PendingSync=True BEFORE OutboxWorker processes it
func TestHost_InitialConditions_ReadyFalseBeforeSync(t *testing.T) {
	// Setup testcontainer with PostgreSQL + apply all migrations
	ctx := context.Background()
	env := setupTestEnvironment(t, ctx)
	defer env.Cleanup()

	// Create Host through Writer (simulates kubectl create)
	host := models.Host{
		Namespace: "test-ns",
		Name:      "test-host-initial",
		UUID:      "aaaaaaaa-bbbb-cccc-dddd-000000000001",
		// No initial conditions set
	}

	writer := writers.NewWriter(env.DB)
	err := writer.Transaction(ctx, func(ctx context.Context, tx ports.Writer) error {
		scope := ports.NewNamespaceScope("test-ns")
		return tx.SyncHosts(ctx, []models.Host{host}, scope)
	})
	require.NoError(t, err, "SyncHosts should succeed")

	// CRITICAL CHECK: Read Host conditions IMMEDIATELY (before OutboxWorker)
	reader := readers.NewReader(env.DB)
	hosts, err := reader.GetHostsByNamespace(ctx, "test-ns")
	require.NoError(t, err)
	require.Len(t, hosts, 1, "Host should be created")

	createdHost := hosts[0]

	// Validate initial conditions
	t.Run("Ready condition should be False", func(t *testing.T) {
		readyCond := findCondition(createdHost.Meta.Conditions, "Ready")
		require.NotNil(t, readyCond, "Ready condition should exist")
		assert.Equal(t, metav1.ConditionFalse, readyCond.Status,
			"Ready should be False before SGROUP sync")
		assert.Equal(t, models.ReasonPendingSGROUPSync, readyCond.Reason,
			"Reason should be PendingSGROUPSync")
	})

	t.Run("PendingSync condition should be True", func(t *testing.T) {
		pendingCond := findCondition(createdHost.Meta.Conditions, models.ConditionPendingSync)
		require.NotNil(t, pendingCond, "PendingSync condition should exist")
		assert.Equal(t, metav1.ConditionTrue, pendingCond.Status,
			"PendingSync should be True before OutboxWorker processes")
	})

	t.Run("Synced condition should be False", func(t *testing.T) {
		syncedCond := findCondition(createdHost.Meta.Conditions, models.ConditionSynced)
		require.NotNil(t, syncedCond, "Synced condition should exist")
		assert.Equal(t, metav1.ConditionFalse, syncedCond.Status,
			"Synced should be False before sync happens")
	})
}

func findCondition(conditions []metav1.Condition, condType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == condType {
			return &conditions[i]
		}
	}
	return nil
}
```

**Запуск теста**:
```bash
cd /Users/zhd/Projects/newPro/netguard-pg-backend
go test -v -run TestHost_InitialConditions_ReadyFalseBeforeSync \
  ./internal/sync/integration/
```

**Критерий успеха**: Тест должен FAIL если баг существует (Ready=True вместо False)

---

### Part 3: Root Cause Investigation 🔬

**Цель**: Найти ТОЧНУЮ причину почему Ready=True устанавливается преждевременно

#### 3.1 Проверить вызов `forcePendingSyncCondition()`

**Файл**: `internal/infrastructure/repositories/pg/writers/host.go`

**Вопросы для проверки**:

1. **Вызывается ли функция для новых Host?**
   - Добавить логирование в строку 69:
   ```go
   if !existingResourceVersion.Valid {
       klog.InfoS("🔍 DEBUG: New Host detected, calling forcePendingSyncCondition()",
           "namespace", host.Namespace, "name", host.Name)
       conditions = forcePendingSyncCondition(conditions)
   }
   ```

2. **Что возвращает функция?**
   - Логировать conditions ДО и ПОСЛЕ:
   ```go
   klog.InfoS("🔍 DEBUG: Conditions BEFORE forcePendingSyncCondition()",
       "conditions", conditions)
   conditions = forcePendingSyncCondition(conditions)
   klog.InfoS("🔍 DEBUG: Conditions AFTER forcePendingSyncCondition()",
       "conditions", conditions)
   ```

3. **Затираются ли условия позже?**
   - Найти все места где conditions изменяются после вызова функции
   - Проверить что conditions сохраняются в БД корректно

#### 3.2 Проверить саму функцию `forcePendingSyncCondition()`

**Найти определение функции** (вероятно в том же файле или в `helpers.go`):
```bash
cd /Users/zhd/Projects/newPro/netguard-pg-backend
grep -rn "func forcePendingSyncCondition" internal/
```

**Проверить логику**:
- Устанавливается ли Ready=False?
- Устанавливается ли PendingSync=True?
- Правильный ли Reason используется?

**Ожидаемая реализация**:
```go
func forcePendingSyncCondition(conditions []metav1.Condition) []metav1.Condition {
	// Remove any existing Ready condition
	filtered := []metav1.Condition{}
	for _, c := range conditions {
		if c.Type != models.ConditionReady {
			filtered = append(filtered, c)
		}
	}

	// Add Ready=False
	filtered = append(filtered, metav1.Condition{
		Type:               models.ConditionReady,
		Status:             metav1.ConditionFalse,
		Reason:             models.ReasonPendingSGROUPSync,
		Message:            "Waiting for SGROUP synchronization",
		LastTransitionTime: metav1.Now(),
	})

	// Add PendingSync=True
	filtered = append(filtered, metav1.Condition{
		Type:               models.ConditionPendingSync,
		Status:             metav1.ConditionTrue,
		Reason:             models.ReasonWaitingForSync,
		Message:            "Resource created, waiting for sync",
		LastTransitionTime: metav1.Now(),
	})

	return filtered
}
```

#### 3.3 Проверить другие ресурсы

**Вопрос**: Эта проблема только для Host, или для всех ресурсов?

**Проверить**:
- Network (internal/infrastructure/repositories/pg/writers/network.go)
- AddressGroup (internal/infrastructure/repositories/pg/writers/address_group.go)
- Service (internal/infrastructure/repositories/pg/writers/service.go)

**Искать**: Аналогичные вызовы `forcePendingSyncCondition()` или подобной логики

---

### Part 4: Реализация Фикса 🔧

**Цель**: Исправить код так чтобы Ready=False устанавливался корректно для новых ресурсов

#### Вариант A: Функция не вызывается

**Если**: Условие `!existingResourceVersion.Valid` не срабатывает

**Фикс**: Проверить логику определения "новый ресурс"
```go
// BEFORE
if !existingResourceVersion.Valid {
    conditions = forcePendingSyncCondition(conditions)
}

// AFTER (если нужно)
err = w.tx.QueryRow(ctx, checkQuery, host.Namespace, host.Name).Scan(&existingResourceVersion)
if err == sql.ErrNoRows || !existingResourceVersion.Valid {
    // Definitively a new resource
    conditions = forcePendingSyncCondition(conditions)
    klog.V(4).InfoS("Forcing PendingSGROUPSync status for new Host", ...)
}
```

#### Вариант B: Функция возвращает неправильные conditions

**Если**: `forcePendingSyncCondition()` не устанавливает Ready=False

**Фикс**: Исправить реализацию функции (см. ожидаемую реализацию в Part 3.2)

#### Вариант C: Conditions затираются позже

**Если**: Conditions изменяются ПОСЛЕ вызова функции

**Фикс**: Найти место где происходит затирание и убрать его для новых ресурсов

#### Вариант D: Проблема в INSERT query

**Если**: SQL INSERT не сохраняет правильные conditions

**Фикс**: Проверить SQL query который вставляет k8s_metadata:
```go
// Убедиться что используются ПРАВИЛЬНЫЕ conditions (после forcePendingSyncCondition)
conditionsJSON, err := json.Marshal(conditions)  // НЕ host.Meta.Conditions!
// ...
INSERT INTO k8s_metadata (..., conditions) VALUES (..., $X)
```

#### Применить фикс ко всем ресурсам

**Если баг найден и исправлен для Host**: Применить аналогичный фикс к:
- Network
- AddressGroup
- Service
- HostBinding (если применимо)
- NetworkBinding (если применимо)

---

### Part 5: Тестирование и Валидация ✅

**Цель**: Подтвердить что фикс работает локально и в minikube

#### 5.1 Local Integration Test
```bash
# Запустить тест созданный в Part 2
cd /Users/zhd/Projects/newPro/netguard-pg-backend
go test -v -run TestHost_InitialConditions_ReadyFalseBeforeSync \
  ./internal/sync/integration/

# Ожидаем: PASS ✅
```

#### 5.2 Regression Tests
```bash
# Убедиться что ничего не сломалось
go test -v ./internal/infrastructure/repositories/pg/writers/... -run "Host"
go test -v ./internal/sync/integration/... -run "Host"

# Ожидаем: All tests PASS ✅
```

#### 5.3 Собрать новый образ
```bash
cd /Users/zhd/Projects/newPro/netguard-pg-backend
TIMESTAMP=$(date +%s)
docker build -t netguard/pg-backend:ready-fix-$TIMESTAMP -f Dockerfile.backend .

# Ожидаем: Build SUCCESS ✅
echo "Image: netguard/pg-backend:ready-fix-$TIMESTAMP"
```

#### 5.4 Deploy в Minikube
```bash
# Load image
minikube image load netguard/pg-backend:ready-fix-$TIMESTAMP

# Update deployment
kubectl set image deployment/netguard-backend \
  backend=netguard/pg-backend:ready-fix-$TIMESTAMP \
  -n incloud-sgroups

# Wait for rollout
kubectl rollout status deployment/netguard-backend -n incloud-sgroups

# Verify new image
kubectl get pod -n incloud-sgroups -l app=netguard-backend -o jsonpath='{.items[0].spec.containers[0].image}'
# Ожидаем: netguard/pg-backend:ready-fix-$TIMESTAMP ✅
```

#### 5.5 E2E Validation в Minikube (SGROUP DISABLED)

**Scenario 1: SGROUP выключен (initial state test)**

```bash
# 1. Выключить SGROUP
kubectl scale deployment sgroups-api --replicas=0 -n incloud-sgroups
sleep 5

# 2. Создать Host
cat <<EOF | kubectl apply -f -
apiVersion: netguard.sgroups.io/v1beta1
kind: Host
metadata:
  name: ready-fix-validation-host
  namespace: incloud-sgroups
spec:
  uuid: "ffffffff-eeee-dddd-cccc-bbbbbbbbbbbb"
EOF

# 3. Проверить conditions НЕМЕДЛЕННО (2-3 секунды)
sleep 3
kubectl get host ready-fix-validation-host -n incloud-sgroups \
  -o jsonpath='{.status.conditions}' | jq '.[] | select(.type=="Ready")'

# ✅ ОЖИДАЕМ:
# {
#   "type": "Ready",
#   "status": "False",  // ✅ НЕ True!
#   "reason": "PendingSGROUPSync",
#   "message": "Waiting for SGROUP synchronization"
# }

# 4. Проверить PendingSync
kubectl get host ready-fix-validation-host -n incloud-sgroups \
  -o jsonpath='{.status.conditions}' | jq '.[] | select(.type=="PendingSync")'

# ✅ ОЖИДАЕМ:
# {
#   "type": "PendingSync",
#   "status": "True"
# }
```

**Scenario 2: SGROUP включён (automatic recovery test)**

```bash
# 5. Включить SGROUP обратно
kubectl scale deployment sgroups-api --replicas=1 -n incloud-sgroups

# 6. Подождать OutboxWorker (20 секунд)
sleep 20

# 7. Проверить что Ready ТЕПЕРЬ True (после синка)
kubectl get host ready-fix-validation-host -n incloud-sgroups \
  -o jsonpath='{.status.conditions}' | jq '.[] | select(.type=="Ready")'

# ✅ ОЖИДАЕМ:
# {
#   "type": "Ready",
#   "status": "True",  // ✅ Теперь True после синка!
#   "reason": "Ready",
#   "message": "Synced to SGROUP"
# }

# 8. Проверить что PendingSync ТЕПЕРЬ False
kubectl get host ready-fix-validation-host -n incloud-sgroups \
  -o jsonpath='{.status.conditions}' | jq '.[] | select(.type=="PendingSync")'

# ✅ ОЖИДАЕМ:
# {
#   "type": "PendingSync",
#   "status": "False"
# }
```

**Scenario 3: Новый Host при работающем SGROUP (fast sync test)**

```bash
# 9. Создать второй Host (SGROUP уже работает)
cat <<EOF | kubectl apply -f -
apiVersion: netguard.sgroups.io/v1beta1
kind: Host
metadata:
  name: ready-fix-validation-host-2
  namespace: incloud-sgroups
spec:
  uuid: "ffffffff-eeee-dddd-cccc-bbbbbbbbbbb2"
EOF

# 10. Проверить ОЧЕНЬ БЫСТРО (1 секунда)
sleep 1
kubectl get host ready-fix-validation-host-2 -n incloud-sgroups \
  -o jsonpath='{.status.conditions}' | jq '.[] | select(.type=="Ready")'

# 📊 ВОЗМОЖНЫЕ РЕЗУЛЬТАТЫ:
# - Ready=False (caught before sync) ✅ ИДЕАЛЬНО
# - Ready=True (sync already happened) ✅ ДОПУСТИМО (< 1s - очень быстро!)

# 11. Подождать 10 секунд и проверить финальное состояние
sleep 10
kubectl get host ready-fix-validation-host-2 -n incloud-sgroups \
  -o jsonpath='{.status.conditions}' | jq '.[] | select(.type=="Ready")'

# ✅ ОЖИДАЕМ: Ready=True (синк завершён)
```

#### 5.6 Проверить Backend Logs

```bash
# Проверить что НЕТ ошибок
kubectl logs -n incloud-sgroups deployment/netguard-backend --tail=100 --since=5m | \
  grep -E "(error|warn)" | grep -i "ready-fix-validation"

# ✅ ОЖИДАЕМ: Нет ошибок

# Проверить что forcePendingSyncCondition вызывалась
kubectl logs -n incloud-sgroups deployment/netguard-backend --tail=200 | \
  grep "Forcing PendingSGROUPSync status for new Host"

# ✅ ОЖИДАЕМ: Сообщения о принудительной установке Pending status
```

#### 5.7 Cleanup

```bash
kubectl delete host ready-fix-validation-host -n incloud-sgroups
kubectl delete host ready-fix-validation-host-2 -n incloud-sgroups
```

**Критерий успеха всех тестов**:
- ✅ Local integration test PASSED
- ✅ Regression tests PASSED (no breakage)
- ✅ Minikube Scenario 1: Ready=False при создании (SGROUP disabled)
- ✅ Minikube Scenario 2: Ready=True после включения SGROUP (automatic recovery)
- ✅ Minikube Scenario 3: Работает корректно при быстром синке
- ✅ Backend logs без ошибок

---

### Part 6: Final QA Approval 📋

**Цель**: Получить финальное подтверждение что фикс готов к production

#### 6.1 Создать Summary Report

**Файл**: `READY_CONDITION_FIX_VALIDATION_REPORT.md`

**Содержимое**:
```markdown
# ✅ Ready Condition Fix - Validation Report

## Bug Fixed
Host resources no longer have Ready=True before SGROUP synchronization

## Evidence

### Local Tests
- Integration test: PASSED ✅
- Regression tests: PASSED (0 failures) ✅

### Minikube E2E Tests
- Scenario 1 (SGROUP disabled): Ready=False ✅
- Scenario 2 (SGROUP enabled): Ready=True after sync ✅
- Scenario 3 (Fast sync): Correct behavior ✅

### Backend Logs
- No errors ✅
- forcePendingSyncCondition() called correctly ✅

## Risk Assessment
- Risk Level: LOW ✅
- Isolated change in condition initialization logic
- No impact on existing resources
- Backward compatible

## Production Readiness
✅ APPROVED FOR PRODUCTION
```

#### 6.2 QA Sign-Off

**QA Engineer проверяет**:
1. ✅ Все тесты прошли
2. ✅ E2E validation в minikube подтверждена
3. ✅ Backend logs чистые
4. ✅ Нет регрессий
5. ✅ Ready condition semantics исправлена

**QA Decision**: ✅ APPROVE / ❌ REJECT

#### 6.3 Deployment Approval

**DevOps/SRE проверяет**:
1. ✅ Image собран корректно
2. ✅ Minikube deployment успешен
3. ✅ Rollback plan есть
4. ✅ Monitoring в порядке

**DevOps Decision**: ✅ APPROVE / ❌ REJECT

---

## Success Metrics

### Before Fix ❌
- Ready=True IMMEDIATELY after creation (< 1 second)
- No indication that sync is pending
- Misleading status for clients

### After Fix ✅
- Ready=False initially (correct!)
- PendingSync=True initially (correct!)
- Ready=True ONLY after successful SGROUP sync
- Clear indication of sync status

---

## Rollback Plan

If issues occur after deployment:

```bash
# Get previous image tag
kubectl rollout history deployment/netguard-backend -n incloud-sgroups

# Rollback
kubectl rollout undo deployment/netguard-backend -n incloud-sgroups

# Verify
kubectl rollout status deployment/netguard-backend -n incloud-sgroups
```

**Rollback Safety**: ✅ Easy (1 command, ~30 seconds)

---

## Related Bugs

**Previous P0 Fix**: Host deletion bug (fixed in `p0-fix-1760557500`)
- Issue: Hosts deleted during condition-only updates
- Fix: Added ConditionOnlyOperation check in SyncHosts()
- Status: ✅ FIXED and deployed

**Current Fix**: Ready condition premature True
- Issue: Ready=True before SGROUP sync
- Fix: Ensure forcePendingSyncCondition() works correctly
- Status: 🔄 IN PROGRESS (following this plan)

---

## Files To Be Modified

### Primary Files
1. `internal/infrastructure/repositories/pg/writers/host.go`
   - Lines 63-72: Verify forcePendingSyncCondition() is called and works
   - Potentially: Fix the function implementation

### Test Files To Be Created
1. `internal/sync/integration/host_initial_conditions_test.go`
   - Test that validates Ready=False before sync

### Potentially Affected Files
1. `internal/infrastructure/repositories/pg/writers/network.go`
2. `internal/infrastructure/repositories/pg/writers/address_group.go`
3. `internal/infrastructure/repositories/pg/writers/service.go`
   - Apply same fix if they have the same bug

---

## Timeline Estimate

| Phase | Duration | Description |
|-------|----------|-------------|
| Part 1: Bug Confirmation | 15 min | Minikube testing with SGROUP disabled |
| Part 2: Local Test Creation | 30 min | Write integration test |
| Part 3: Root Cause Investigation | 30 min | Find exact cause |
| Part 4: Fix Implementation | 30 min | Apply fix to code |
| Part 5: Testing & Validation | 45 min | Local + minikube validation |
| Part 6: QA Approval | 15 min | Final sign-off |
| **TOTAL** | **~3 hours** | Full cycle from bug to production-ready |

---

## Next Steps

1. ✅ Plan created and saved
2. ⏳ Execute Part 1: Confirm bug in minikube
3. ⏳ Execute Part 2: Create local integration test
4. ⏳ Execute Part 3: Root cause investigation
5. ⏳ Execute Part 4: Implement fix
6. ⏳ Execute Part 5: Testing and validation
7. ⏳ Execute Part 6: QA approval
8. ⏳ Deploy to production

---

**План создан**: 2025-10-16
**Приоритет**: P0 CRITICAL
**Готов к выполнению**: ✅ YES

**Следующее действие**: Выполнить Part 1 - подтверждение бага в minikube с выключенным SGROUP
