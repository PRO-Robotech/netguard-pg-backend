# Сценарий 3: Синхронизация данных между слоями

## Описание
Система выполняет синхронизацию данных между различными слоями: Aggregated API Server, Backend Service и Repository. Синхронизация обеспечивает консистентность данных и обработку пакетных операций.

## Последовательность действий

```plantuml
@startuml
!theme plain
skinparam backgroundColor #FFFFFF
skinparam sequenceArrowThickness 2
skinparam roundcorner 20
skinparam maxmessagesize 60

participant User
participant K8S as Kubernetes API Server
participant AGG as Aggregated API Server
participant GRPC as gRPC Client
participant BL as Business Logic
participant REPO as Repository
participant DB as Database
participant Cache as Cache Layer

User->>K8S: kubectl apply -f batch-resources.yaml
K8S->>AGG: Multiple resource operations
AGG->>GRPC: SyncRequest(batch_operations)
GRPC->>BL: ProcessSync(batch_operations)

BL->>BL: Validate batch operations
BL->>REPO: BeginTransaction()
REPO->>DB: BEGIN TRANSACTION

loop For each operation
    BL->>REPO: ExecuteOperation(operation)
    REPO->>DB: Execute SQL
    DB-->>REPO: Operation result
    REPO-->>BL: Operation completed
end

alt All operations successful
    BL->>REPO: CommitTransaction()
    REPO->>DB: COMMIT
    DB-->>REPO: Transaction committed
    REPO-->>BL: Transaction committed
    BL->>Cache: InvalidateCache()
    Cache->>Cache: Clear affected data
    BL-->>GRPC: SyncSuccess
    GRPC-->>AGG: SyncSuccess
    AGG-->>K8S: 200 OK
    K8S-->>User: Resources synchronized
else Some operations failed
    BL->>REPO: RollbackTransaction()
    REPO->>DB: ROLLBACK
    DB-->>REPO: Transaction rolled back
    REPO-->>BL: Transaction rolled back
    BL-->>GRPC: SyncFailed
    GRPC-->>AGG: SyncFailed
    AGG-->>K8S: 500 Internal Server Error
    K8S-->>User: Sync failed
end
@enduml
```

## Примеры пакетных операций

### 1. Пакетное создание ресурсов

#### YAML файл с множественными ресурсами
```yaml
# batch-resources.yaml
apiVersion: netguard.sgroups.io/v1beta1
kind: Service
metadata:
  name: web-service
  namespace: default
spec:
  selfRef:
    name: web-service
    namespace: default
  ingressPorts:
    - protocol: TCP
      port: "80"
      description: "HTTP port"
    - protocol: TCP
      port: "443"
      description: "HTTPS port"
---
apiVersion: netguard.sgroups.io/v1beta1
kind: AddressGroup
metadata:
  name: web-clients
  namespace: default
spec:
  name: web-clients
  namespace: default
  addresses:
    - "10.0.0.0/8"
    - "172.16.0.0/12"
  description: "Web application clients"
---
apiVersion: netguard.sgroups.io/v1beta1
kind: AddressGroupBinding
metadata:
  name: web-service-binding
  namespace: default
spec:
  serviceRef:
    name: web-service
    namespace: default
  addressGroupRef:
    name: web-clients
    namespace: default
  description: "Binding web service to client address group"
```

#### Команда применения
```bash
kubectl apply -f batch-resources.yaml
```

#### Ожидаемый результат
```bash
service.netguard.sgroups.io/web-service created
addressgroup.netguard.sgroups.io/web-clients created
addressgroupbinding.netguard.sgroups.io/web-service-binding created
```

### 2. Конфигурация транзакций

#### Настройка транзакционной обработки
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: netguard-transaction-config
  namespace: netguard-system
data:
  # Настройки транзакций
  transaction_settings: |
    max_batch_size: 100
    transaction_timeout: "30s"
    retry_attempts: 3
    retry_delay: "1s"
    isolation_level: "READ_COMMITTED"
  
  # Правила отката
  rollback_rules: |
    - operation_type: "CREATE"
      rollback_on_failure: true
      cleanup_resources: true
    - operation_type: "UPDATE"
      rollback_on_failure: true
      restore_previous_state: true
    - operation_type: "DELETE"
      rollback_on_failure: false
      preserve_resources: true
```

### 3. Примеры успешной синхронизации

#### Успешная пакетная операция
```yaml
# Результат успешной синхронизации
status:
  conditions:
    - type: Ready
      status: "True"
      lastTransitionTime: "2024-01-15T10:30:00Z"
      reason: "Synchronized"
      message: "All resources synchronized successfully"
  synchronizedAt: "2024-01-15T10:30:00Z"
  batchId: "batch-12345678-1234-1234-1234-123456789abc"
  operations:
    - resource: "service/web-service"
      operation: "CREATE"
      status: "SUCCESS"
      duration: "23ms"
    - resource: "addressgroup/web-clients"
      operation: "CREATE"
      status: "SUCCESS"
      duration: "15ms"
    - resource: "addressgroupbinding/web-service-binding"
      operation: "CREATE"
      status: "SUCCESS"
      duration: "18ms"
```

### 4. Примеры ошибок синхронизации

#### Частичная ошибка в пакете
```yaml
# Ресурс с ошибкой
apiVersion: netguard.sgroups.io/v1beta1
kind: Service
metadata:
  name: invalid-service
  namespace: default
spec:
  selfRef:
    name: invalid-service
    namespace: default
  ingressPorts:
    - protocol: TCP
      port: "99999"  # Неверный порт
---
# Валидный ресурс
apiVersion: netguard.sgroups.io/v1beta1
kind: AddressGroup
metadata:
  name: valid-group
  namespace: default
spec:
  name: valid-group
  namespace: default
  addresses:
    - "192.168.0.0/16"
```

#### Результат с ошибкой
```yaml
status:
  conditions:
    - type: Ready
      status: "False"
      lastTransitionTime: "2024-01-15T10:30:00Z"
      reason: "SyncFailed"
      message: "Batch synchronization failed: 1 operation failed"
  synchronizedAt: "2024-01-15T10:30:00Z"
  batchId: "batch-12345678-1234-1234-1234-123456789abc"
  operations:
    - resource: "service/invalid-service"
      operation: "CREATE"
      status: "FAILED"
      error: "spec.ingressPorts[0].port: Invalid value: 99999: port number must be between 1 and 65535"
      duration: "12ms"
    - resource: "addressgroup/valid-group"
      operation: "CREATE"
      status: "SUCCESS"
      duration: "15ms"
  rollbackStatus: "COMPLETED"
  rollbackMessage: "All operations rolled back due to validation error"
```

### 5. Конфигурация кэширования

#### Настройка кэша
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: netguard-cache-config
  namespace: netguard-system
data:
  # Настройки кэша
  cache_settings: |
    enabled: true
    ttl: "5m"
    max_size: 1000
    eviction_policy: "LRU"
  
  # Правила инвалидации
  invalidation_rules: |
    - resource_type: "Service"
      invalidate_on: ["CREATE", "UPDATE", "DELETE"]
      affected_resources: ["AddressGroupBinding"]
    - resource_type: "AddressGroup"
      invalidate_on: ["CREATE", "UPDATE", "DELETE"]
      affected_resources: ["AddressGroupBinding", "Service"]
    - resource_type: "AddressGroupBinding"
      invalidate_on: ["CREATE", "UPDATE", "DELETE"]
      affected_resources: []
```

### 6. Метрики синхронизации

#### Prometheus метрики
```yaml
# Метрики пакетных операций
netguard_batch_operations_total{status="success"} 1
netguard_batch_operations_total{status="failed"} 0
netguard_batch_operations_total{status="partial_failure"} 0

# Время синхронизации
netguard_sync_duration_seconds{operation_type="batch",quantile="0.5"} 0.056
netguard_sync_duration_seconds{operation_type="batch",quantile="0.9"} 0.089
netguard_sync_duration_seconds{operation_type="batch",quantile="0.99"} 0.123

# Размер пакетов
netguard_batch_size{operation_type="create",quantile="0.5"} 3
netguard_batch_size{operation_type="create",quantile="0.9"} 10
netguard_batch_size{operation_type="create",quantile="0.99"} 25

# Метрики транзакций
netguard_transaction_total{status="committed"} 1
netguard_transaction_total{status="rolled_back"} 0
netguard_transaction_duration_seconds{status="committed",quantile="0.5"} 0.045
```

#### Grafana Dashboard
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: netguard-sync-dashboard
  namespace: monitoring
data:
  dashboard.json: |
    {
      "dashboard": {
        "title": "Netguard Synchronization Metrics",
        "panels": [
          {
            "title": "Batch Operation Success Rate",
            "type": "graph",
            "targets": [
              {
                "expr": "rate(netguard_batch_operations_total{status=\"success\"}[5m]) / rate(netguard_batch_operations_total[5m])",
                "legendFormat": "Success Rate"
              }
            ]
          },
          {
            "title": "Sync Duration",
            "type": "graph",
            "targets": [
              {
                "expr": "histogram_quantile(0.5, rate(netguard_sync_duration_seconds_bucket[5m]))",
                "legendFormat": "50th percentile"
              },
              {
                "expr": "histogram_quantile(0.9, rate(netguard_sync_duration_seconds_bucket[5m]))",
                "legendFormat": "90th percentile"
              }
            ]
          },
          {
            "title": "Transaction Status",
            "type": "graph",
            "targets": [
              {
                "expr": "rate(netguard_transaction_total[5m])",
                "legendFormat": "{{status}}"
              }
            ]
          }
        ]
      }
    }
```

### 7. Логирование синхронизации

#### Структура логов
```yaml
# Лог успешной синхронизации
{
  "level": "info",
  "timestamp": "2024-01-15T10:30:00Z",
  "batch_id": "batch-12345678-1234-1234-1234-123456789abc",
  "operation": "batch_sync",
  "status": "success",
  "operations_count": 3,
  "successful_operations": 3,
  "failed_operations": 0,
  "duration_ms": 56,
  "transaction_id": "tx-87654321-4321-4321-4321-cba987654321"
}

# Лог ошибки синхронизации
{
  "level": "error",
  "timestamp": "2024-01-15T10:30:00Z",
  "batch_id": "batch-12345678-1234-1234-1234-123456789abc",
  "operation": "batch_sync",
  "status": "failed",
  "operations_count": 2,
  "successful_operations": 1,
  "failed_operations": 1,
  "duration_ms": 45,
  "error": "Validation failed for service/invalid-service",
  "rollback_status": "completed",
  "rollback_duration_ms": 12
}
```

#### Конфигурация логирования
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: netguard-sync-logging
  namespace: netguard-system
data:
  log-level: "info"
  log-format: "json"
  log-fields: "batch_id,operation,status,operations_count"
  sync-log-level: "debug"
  transaction-log-level: "info"
``` 