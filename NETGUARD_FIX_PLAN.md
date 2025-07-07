# 🛠️ ПЛАН УСТРАНЕНИЯ ПРОБЛЕМ Netguard v1beta1 Aggregation Layer

**Дата создания:** 29 декабря 2024  
**Версия:** v1beta1  
**Проект:** netguard-pg-backend  
**Цель:** Исправить watch операции и CRUD функциональность

---

## 🎯 КРАТКОЕ РЕЗЮМЕ ПРОБЛЕМ

Анализ кодовой базы выявил следующие критические проблемы:

1. **🚨 WATCH OPERATIONS** - конвертация в Unstructured нарушает декодирование List типов
2. **❌ Backend CRUD** - неполная реализация для AddressGroup и ServiceAlias
3. **⚠️ PATCH Operations** - отсутствует merge strategy
4. **🔄 Inconsistent Backend APIs** - смешанное использование Sync API и прямых методов

---

## 🔄 ЭТАП 1: КРИТИЧЕСКОЕ ИСПРАВЛЕНИЕ WATCH ОПЕРАЦИЙ (1-3 дня)

### [ ] 1.1 Диагностика проблемы с watch

**Цель:** Понять точную причину ошибки декодирования  
**Время:** 2-4 часа

```bash
# Тестирование текущего состояния watch
kubectl get services.v1beta1.netguard.sgroups.io -n netguard-test --watch --timeout=10s

# Проверка API discovery
kubectl get --raw /apis/netguard.sgroups.io/v1beta1 | jq '.resources[] | select(.name == "services")'

# Проверка логов API Server
kubectl logs deployment/netguard-apiserver -n netguard-test | grep -E "(watch|stream|decode)" | tail -20
```

**Ожидаемый результат:** Подтвердить что проблема в конвертации в Unstructured

---

### [ ] 1.2 Исправление PollerWatchInterface

**Цель:** Убрать конвертацию в Unstructured, возвращать типизированные объекты  
**Время:** 2-3 часа

**Файл:** `internal/k8s/registry/watch/poller_watch_interface.go`

**КОРНЕВАЯ ПРОБЛЕМА:** В методе `ResultChan()` происходит конвертация в `unstructured.Unstructured`, что нарушает типизацию

**Текущий (НЕПРАВИЛЬНЫЙ) код:**
```go
func (w *PollerWatchInterface) ResultChan() <-chan watch.Event {
    unstructuredChan := make(chan watch.Event)
    go func() {
        defer close(unstructuredChan)
        for event := range w.client.eventChan {
            // ПРОБЛЕМА: конвертируем в Unstructured
            unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(event.Object)
            if err != nil {
                klog.Errorf("failed to convert object to unstructured: %v", err)
                continue
            }
            unstructuredEvent := watch.Event{
                Type:   event.Type,
                Object: &unstructured.Unstructured{Object: unstructuredObj},
            }
            unstructuredChan <- unstructuredEvent
        }
    }()
    return unstructuredChan
}
```

**ИСПРАВЛЕННАЯ версия:**
```go
func (w *PollerWatchInterface) ResultChan() <-chan watch.Event {
    // ИСПРАВЛЕНИЕ: возвращаем прямо канал с типизированными объектами
    return w.client.eventChan
}
```

**Тест после изменений:**
```bash
timeout 15s kubectl get services.v1beta1.netguard.sgroups.io -n netguard-test --watch
# ОЖИДАЕМЫЙ РЕЗУЛЬТАТ: НЕ должно быть ошибки "no kind 'ServiceList' is registered"
```

---

### [ ] 1.3 Проверка конверторов watch

**Цель:** Убедиться что конверторы возвращают правильные типизированные объекты  
**Время:** 1-2 часа

**Файл:** `internal/k8s/registry/watch/converters.go`

**Проверить ServiceConverter:**
```go
func (c *ServiceConverter) ConvertToK8s(resource interface{}) runtime.Object {
    service, ok := resource.(models.Service)
    if !ok {
        return nil
    }

    k8sService := &netguardv1beta1.Service{
        TypeMeta: metav1.TypeMeta{
            Kind:       "Service",           // КРИТИЧНО: правильный Kind
            APIVersion: "netguard.sgroups.io/v1beta1", // КРИТИЧНО: правильный APIVersion
        },
        ObjectMeta: metav1.ObjectMeta{
            Name:      service.ResourceIdentifier.Name,
            Namespace: service.ResourceIdentifier.Namespace,
        },
        Spec: netguardv1beta1.ServiceSpec{
            Description: service.Description,
        },
    }

    // Convert IngressPorts
    for _, port := range service.IngressPorts {
        k8sPort := netguardv1beta1.IngressPort{
            Protocol:    netguardv1beta1.TransportProtocol(port.Protocol),
            Port:        port.Port,
            Description: port.Description,
        }
        k8sService.Spec.IngressPorts = append(k8sService.Spec.IngressPorts, k8sPort)
    }

    return k8sService  // Возвращаем ТИПИЗИРОВАННЫЙ объект
}
```

---

### [ ] 1.4 Тестирование исправлений watch

**Цель:** Проверить что watch операции работают на Service ресурсе  
**Время:** 1 час

**Создать тестовый скрипт:** `scripts/test-watch-fix.sh`

```bash
#!/bin/bash
NAMESPACE="netguard-test"
RESOURCE_NAME="test-watch-service"

echo "🧪 Тестирование исправления watch операций..."

# 1. Запуск watch в фоне
timeout 30s kubectl get services.v1beta1.netguard.sgroups.io -n "$NAMESPACE" --watch > /tmp/watch_output 2>&1 &
WATCH_PID=$!

sleep 3

# 2. CREATE событие
echo "Creating service..."
kubectl apply -f - <<EOF
apiVersion: netguard.sgroups.io/v1beta1
kind: Service
metadata:
  name: $RESOURCE_NAME
  namespace: $NAMESPACE
spec:
  description: "Watch test service"
  ingressPorts:
  - protocol: TCP
    port: "80"
EOF

sleep 5

# 3. MODIFY событие
echo "Updating service..."
kubectl patch services.v1beta1.netguard.sgroups.io "$RESOURCE_NAME" -n "$NAMESPACE" \
  --type=merge -p '{"spec":{"description":"Updated by watch test"}}'

sleep 5

# 4. DELETE событие
echo "Deleting service..."
kubectl delete services.v1beta1.netguard.sgroups.io "$RESOURCE_NAME" -n "$NAMESPACE"

sleep 3

# 5. Остановить watch
kill $WATCH_PID 2>/dev/null || true
wait $WATCH_PID 2>/dev/null || true

# 6. Проверить результаты
echo "=== Watch Output ==="
cat /tmp/watch_output

echo ""
echo "=== Проверка результатов ==="
if grep -q "unable to decode" /tmp/watch_output; then
    echo "❌ FAILED: Найдены ошибки декодирования"
    exit 1
elif grep -q "ADDED.*$RESOURCE_NAME" /tmp/watch_output && grep -q "DELETED.*$RESOURCE_NAME" /tmp/watch_output; then
    echo "✅ SUCCESS: Watch события корректно обработаны"
    exit 0
else
    echo "⚠️ PARTIAL: Watch работает, но не все события обнаружены"
    exit 1
fi
```

**Критерии успеха:**
- [ ] Нет ошибок декодирования типа "no kind 'ServiceList' is registered"
- [ ] События ADDED, MODIFIED, DELETED отображаются корректно
- [ ] Объекты сериализуются без ошибок

---

## 🛠️ ЭТАП 2: BACKEND CRUD РЕАЛИЗАЦИЯ (1-2 недели)

### [ ] 2.1 AddressGroup backend методы

**Цель:** Реализовать полные CRUD операции для AddressGroup  
**Время:** 3-4 дня

**Проблема:** AddressGroup storage использует Sync API, но backend не реализует прямые CRUD методы

**Файлы для изменения:**
- `internal/k8s/client/backend.go`
- `internal/k8s/client/grpc_client.go`

**Добавить в BackendClient interface:**

```go
// AddressGroup operations
CreateAddressGroup(ctx context.Context, ag *models.AddressGroup) (*models.AddressGroup, error)
GetAddressGroup(ctx context.Context, id models.ResourceIdentifier) (*models.AddressGroup, error)
UpdateAddressGroup(ctx context.Context, ag *models.AddressGroup) (*models.AddressGroup, error)
DeleteAddressGroup(ctx context.Context, id models.ResourceIdentifier) error
ListAddressGroups(ctx context.Context, scope ports.Scope) ([]models.AddressGroup, error)
```

**Реализация с валидацией CIDR:**

```go
func (c *GRPCBackendClient) CreateAddressGroup(ctx context.Context, ag *models.AddressGroup) (*models.AddressGroup, error) {
    // Validate CIDR addresses
    for _, addr := range ag.Addresses {
        if _, _, err := net.ParseCIDR(addr); err != nil {
            return nil, fmt.Errorf("invalid CIDR %s: %w", addr, err)
        }
    }
    
    // Convert to proto
    protoAG := convertAddressGroupToProto(ag)
    
    // Call backend
    resp, err := c.client.CreateAddressGroup(ctx, &api.CreateAddressGroupRequest{
        AddressGroup: protoAG,
    })
    if err != nil {
        return nil, fmt.Errorf("failed to create AddressGroup: %w", err)
    }
    
    return convertAddressGroupFromProto(resp.AddressGroup), nil
}
```

**Тест:**
```bash
kubectl apply -f - <<EOF
apiVersion: netguard.sgroups.io/v1beta1
kind: AddressGroup
metadata:
  name: test-ag
  namespace: netguard-test
spec:
  description: "Test address group"
  addresses:
  - "192.168.1.0/24"
  - "10.0.0.0/8"
EOF

# КРИТЕРИЙ УСПЕХА: ресурс создается без ошибки "server rejected our request"
```

---

### [ ] 2.2 ServiceAlias resource-specific методы

**Цель:** Заменить generic sync на специализированные методы  
**Время:** 2-3 дня

**Проблема:** ServiceAlias получает ошибку "generic sync not implemented - use resource-specific methods"

**Добавить в BackendClient interface:**
```go
CreateServiceAlias(ctx context.Context, sa *models.ServiceAlias) (*models.ServiceAlias, error)
GetServiceAlias(ctx context.Context, id models.ResourceIdentifier) (*models.ServiceAlias, error)
UpdateServiceAlias(ctx context.Context, sa *models.ServiceAlias) (*models.ServiceAlias, error)
DeleteServiceAlias(ctx context.Context, id models.ResourceIdentifier) error
ListServiceAliases(ctx context.Context, scope ports.Scope) ([]models.ServiceAlias, error)
```

**Изменить storage Create метод:**

```go
// internal/k8s/registry/servicealias/storage.go
func (s *ServiceAliasStorage) Create(ctx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
    k8sAlias, ok := obj.(*netguardv1beta1.ServiceAlias)
    if !ok {
        return nil, fmt.Errorf("expected ServiceAlias, got %T", obj)
    }

    // Validation
    if createValidation != nil {
        if err := createValidation(ctx, obj); err != nil {
            return nil, err
        }
    }

    // Convert to backend model
    alias := convertServiceAliasFromK8s(k8sAlias)

    // ИСПРАВЛЕНИЕ: НЕ Sync API, а специализированный метод!
    createdAlias, err := s.backendClient.CreateServiceAlias(ctx, &alias)
    if err != nil {
        return nil, fmt.Errorf("failed to create ServiceAlias: %w", err)
    }

    // Convert back
    result := convertServiceAliasToK8s(*createdAlias)
    return result, nil
}
```

**Тест:**
```bash
kubectl apply -f - <<EOF
apiVersion: netguard.sgroups.io/v1beta1
kind: ServiceAlias
metadata:
  name: test-alias
  namespace: netguard-test
spec:
  description: "Test service alias"
  alias: "web-service"
  target: "target-service"
EOF

# КРИТЕРИЙ УСПЕХА: НЕ должно быть ошибки "generic sync not implemented"
```

---

### [ ] 2.3 PATCH operations support

**Цель:** Добавить поддержку strategic merge patch  
**Время:** 2-3 дня

**Проблема:** PATCH операции не работают из-за отсутствия proper merge strategy

**Изменить каждый storage Update метод:**

```go
func (s *ServiceStorage) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
    // Get current object
    currentObj, err := s.Get(ctx, name, &metav1.GetOptions{})
    if err != nil {
        if forceAllowCreate {
            newObj, err := objInfo.UpdatedObject(ctx, nil)
            if err != nil {
                return nil, false, err
            }
            createdObj, err := s.Create(ctx, newObj, createValidation, &metav1.CreateOptions{})
            return createdObj, true, err
        }
        return nil, false, err
    }

    // Apply strategic merge patch
    updatedObj, err := objInfo.UpdatedObject(ctx, currentObj)
    if err != nil {
        return nil, false, err
    }

    // Validation
    if updateValidation != nil {
        if err := updateValidation(ctx, updatedObj, currentObj); err != nil {
            return nil, false, err
        }
    }

    // Update via backend specific method
    updatedService, ok := updatedObj.(*netguardv1beta1.Service)
    if !ok {
        return nil, false, fmt.Errorf("expected Service, got %T", updatedObj)
    }

    // Convert and update
    backendService := convertServiceFromK8s(*updatedService)
    result, err := s.backendClient.UpdateService(ctx, &backendService)
    if err != nil {
        return nil, false, fmt.Errorf("failed to update service: %w", err)
    }

    return convertServiceToK8s(*result), false, nil
}
```

**Тест PATCH операций:**
```bash
# Test merge patch
kubectl patch services.v1beta1.netguard.sgroups.io test-service -n netguard-test \
  --type=merge -p '{"spec":{"description":"Patched description"}}'

# Verify patch applied
kubectl get services.v1beta1.netguard.sgroups.io test-service -n netguard-test -o jsonpath='{.spec.description}'

# КРИТЕРИЙ УСПЕХА: описание изменилось на "Patched description"
```

---

## 📊 ЭТАП 3: COMPREHENSIVE TESTING (3-5 дней)

### [ ] 3.1 Automated test suite

**Цель:** Создать автоматизированные тесты для всех ресурсов  
**Время:** 2-3 дня

**Создать:** `scripts/test-complete-api.sh`

```bash
#!/bin/bash
set -e

NAMESPACE=${NAMESPACE:-netguard-test}
VERBOSE=${VERBOSE:-false}

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Test counters
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

log() {
    if [[ "$VERBOSE" == "true" ]]; then
        echo -e "$1"
    fi
}

test_resource_crud() {
    local resource_type=$1
    local resource_file=$2
    local resource_name=$3
    
    TOTAL_TESTS=$((TOTAL_TESTS + 4))
    
    echo "🧪 Testing $resource_type CRUD operations..."
    
    # CREATE
    if kubectl apply -f "$resource_file" &>/dev/null; then
        echo -e "  ${GREEN}✓${NC} CREATE: $resource_type"
        PASSED_TESTS=$((PASSED_TESTS + 1))
    else
        echo -e "  ${RED}✗${NC} CREATE: $resource_type"
        FAILED_TESTS=$((FAILED_TESTS + 1))
        return 1
    fi
    
    # READ
    if kubectl get "$resource_type" "$resource_name" -n "$NAMESPACE" &>/dev/null; then
        echo -e "  ${GREEN}✓${NC} READ: $resource_type"
        PASSED_TESTS=$((PASSED_TESTS + 1))
    else
        echo -e "  ${RED}✗${NC} READ: $resource_type"
        FAILED_TESTS=$((FAILED_TESTS + 1))
    fi
    
    # UPDATE (PATCH)
    if kubectl patch "$resource_type" "$resource_name" -n "$NAMESPACE" \
        --type=merge -p '{"spec":{"description":"Updated by automated test"}}' &>/dev/null; then
        echo -e "  ${GREEN}✓${NC} UPDATE: $resource_type"
        PASSED_TESTS=$((PASSED_TESTS + 1))
    else
        echo -e "  ${RED}✗${NC} UPDATE: $resource_type"
        FAILED_TESTS=$((FAILED_TESTS + 1))
    fi
    
    # DELETE
    if kubectl delete "$resource_type" "$resource_name" -n "$NAMESPACE" &>/dev/null; then
        echo -e "  ${GREEN}✓${NC} DELETE: $resource_type"
        PASSED_TESTS=$((PASSED_TESTS + 1))
    else
        echo -e "  ${RED}✗${NC} DELETE: $resource_type"
        FAILED_TESTS=$((FAILED_TESTS + 1))
    fi
}

test_watch_operations() {
    local resource_type=$1
    local test_name="watch-test-$(date +%s)"
    
    echo "🔄 Testing $resource_type WATCH operations..."
    
    # Start watch in background
    timeout 20s kubectl get "$resource_type" -n "$NAMESPACE" --watch > /tmp/watch_output_$$ 2>&1 &
    local watch_pid=$!
    
    sleep 2
    
    # Create resource
    kubectl apply -f - <<EOF &>/dev/null
apiVersion: netguard.sgroups.io/v1beta1
kind: Service
metadata:
  name: $test_name
  namespace: $NAMESPACE
spec:
  description: "Watch test"
  ingressPorts:
  - protocol: TCP
    port: "80"
EOF
    
    sleep 3
    
    # Delete resource
    kubectl delete services.v1beta1.netguard.sgroups.io "$test_name" -n "$NAMESPACE" &>/dev/null
    
    sleep 2
    kill $watch_pid 2>/dev/null || true
    wait $watch_pid 2>/dev/null || true
    
    # Check results
    if grep -q "ADDED" /tmp/watch_output_$$ && ! grep -q "unable to decode" /tmp/watch_output_$$; then
        echo -e "  ${GREEN}✓${NC} WATCH: $resource_type"
        PASSED_TESTS=$((PASSED_TESTS + 1))
    else
        echo -e "  ${RED}✗${NC} WATCH: $resource_type"
        FAILED_TESTS=$((FAILED_TESTS + 1))
    fi
    
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    rm -f /tmp/watch_output_$$
}

# Main execution
echo "🚀 Starting comprehensive API testing..."

mkdir -p /tmp/test-resources

# Test data
cat > /tmp/test-resources/service.yaml <<EOF
apiVersion: netguard.sgroups.io/v1beta1
kind: Service
metadata:
  name: test-service
  namespace: $NAMESPACE
spec:
  description: "Automated test service"
  ingressPorts:
  - protocol: TCP
    port: "80"
EOF

cat > /tmp/test-resources/addressgroup.yaml <<EOF
apiVersion: netguard.sgroups.io/v1beta1
kind: AddressGroup
metadata:
  name: test-addressgroup
  namespace: $NAMESPACE
spec:
  description: "Automated test address group"
  addresses:
  - "192.168.1.0/24"
  - "10.0.0.0/8"
EOF

# Run tests
test_resource_crud "services.v1beta1.netguard.sgroups.io" "/tmp/test-resources/service.yaml" "test-service"
test_resource_crud "addressgroups.v1beta1.netguard.sgroups.io" "/tmp/test-resources/addressgroup.yaml" "test-addressgroup"
test_watch_operations "services.v1beta1.netguard.sgroups.io"

# Cleanup
rm -rf /tmp/test-resources

# Summary
echo ""
echo "📊 Test Summary:"
echo "Total tests: ${TOTAL_TESTS}"
echo -e "Passed: ${GREEN}${PASSED_TESTS}${NC}"
echo -e "Failed: ${RED}${FAILED_TESTS}${NC}"

if [[ $FAILED_TESTS -eq 0 ]]; then
    echo -e "\n🎉 ${GREEN}All tests passed!${NC}"
    exit 0
else
    echo -e "\n❌ ${RED}Some tests failed.${NC}"
    exit 1
fi
```

---

### [ ] 3.2 Performance testing

**Время:** 1-2 дня

**Создать:** `scripts/load-test.sh`

```bash
#!/bin/bash
NAMESPACE=${NAMESPACE:-netguard-test}
CONCURRENT_CLIENTS=${CONCURRENT_CLIENTS:-5}
OPERATIONS_PER_CLIENT=${OPERATIONS_PER_CLIENT:-20}

echo "🚀 Load Testing: $CONCURRENT_CLIENTS clients x $OPERATIONS_PER_CLIENT operations"

perform_load_operations() {
    local client_id=$1
    local operations=$2
    local start_time=$(date +%s)
    local successful_ops=0
    
    for i in $(seq 1 $operations); do
        local resource_name="load-test-client-${client_id}-op-${i}"
        
        if kubectl apply -f - <<EOF &>/dev/null
apiVersion: netguard.sgroups.io/v1beta1
kind: Service
metadata:
  name: $resource_name
  namespace: $NAMESPACE
spec:
  description: "Load test client $client_id operation $i"
  ingressPorts:
  - protocol: TCP
    port: "$(( 8000 + i ))"
EOF
        then
            if kubectl get services.v1beta1.netguard.sgroups.io "$resource_name" -n "$NAMESPACE" &>/dev/null; then
                if kubectl delete services.v1beta1.netguard.sgroups.io "$resource_name" -n "$NAMESPACE" &>/dev/null; then
                    successful_ops=$((successful_ops + 1))
                fi
            fi
        fi
    done
    
    local end_time=$(date +%s)
    local duration=$((end_time - start_time))
    
    echo "Client $client_id: $successful_ops/$operations successful in ${duration}s"
}

# Start concurrent clients
for client_id in $(seq 1 $CONCURRENT_CLIENTS); do
    perform_load_operations "$client_id" "$OPERATIONS_PER_CLIENT" &
done

wait
echo "Load test completed"
```

---

## ✅ КРИТЕРИИ ГОТОВНОСТИ

### ЭТАП 1 - ГОТОВ когда:
- [ ] `kubectl get services.v1beta1.netguard.sgroups.io -n netguard-test --watch` работает без ошибок декодирования
- [ ] Watch события (ADDED, MODIFIED, DELETED) корректно отображаются
- [ ] Нет ошибок типа "no kind 'ServiceList' is registered"
- [ ] `scripts/test-watch-fix.sh` завершается с кодом 0

### ЭТАП 2 - ГОТОВ когда:
- [ ] AddressGroup CRUD операции работают без ошибки "server rejected our request"
- [ ] ServiceAlias создается без ошибки "generic sync not implemented"
- [ ] PATCH операции применяются успешно для всех ресурсов
- [ ] Все backend CRUD методы реализованы

### ЭТАП 3 - ГОТОВ когда:
- [ ] `scripts/test-complete-api.sh` показывает 100% успешных тестов
- [ ] Load test показывает стабильную производительность
- [ ] Все автоматизированные тесты проходят

---

## 🚨 КРИТИЧЕСКИЕ ЗАМЕЧАНИЯ

1. **ПРИОРИТЕТ 0** - Исправление watch операций (проблема в Unstructured конвертации)
2. **НЕ ИЗМЕНЯТЬ СХЕМУ** - List типы уже правильно зарегистрированы
3. **ТЕСТИРОВАТЬ ПОЭТАПНО** - каждый этап полностью завершить перед следующим
4. **FOCUS ON ONE RESOURCE** - начать с Service, потом распространить на остальные

---

## 📞 КОНТАКТЫ ДЛЯ ВОПРОСОВ

- **Backend issues:** Backend team
- **Kubernetes issues:** Platform team  
- **Testing:** QA team

**Документ создан:** 29 декабря 2024  
**Следующий review:** После завершения Этапа 1 