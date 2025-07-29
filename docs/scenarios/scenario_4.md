# Сценарий 4: Обработка ошибок и откатов

## Описание
Система обрабатывает различные типы ошибок на всех слоях и обеспечивает корректные откаты операций. Включает обработку сетевых ошибок, ошибок валидации, конфликтов и внутренних ошибок системы.

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
participant AC as Admission Controller
participant GRPC as gRPC Client
participant BL as Business Logic
participant REPO as Repository
participant DB as Database

User->>K8S: kubectl apply -f resource.yaml
K8S->>AGG: POST /apis/netguard.sgroups.io/v1beta1/resources
AGG->>AC: ValidateCreate(resource)

alt Validation Error
    AC-->>AGG: ValidationResult{Allowed: false, Error}
    AGG-->>K8S: 400 Bad Request
    K8S-->>User: Validation error
else Validation passed
    AGG->>GRPC: CreateResource(resource)
    
    alt Network Error
        GRPC-->>AGG: Network error
        AGG->>AGG: Retry with backoff
        GRPC->>BL: CreateResource(resource)
    end
    
    alt Business Logic Error
        BL-->>GRPC: Business error
        GRPC-->>AGG: Business error
        AGG-->>K8S: 400 Bad Request
        K8S-->>User: Business rule violation
    else Business Logic Success
        BL->>REPO: Create(resource)
        
        alt Database Error
            REPO-->>BL: Database error
            BL->>BL: Rollback changes
            BL-->>GRPC: Database error
            GRPC-->>AGG: Internal error
            AGG-->>K8S: 500 Internal Server Error
            K8S-->>User: Internal server error
        else Database Success
            REPO-->>BL: Resource created
            BL-->>GRPC: Success
            GRPC-->>AGG: Success
            AGG-->>K8S: 201 Created
            K8S-->>User: Resource created
        end
    end
end
@enduml
```

## Типы ошибок и их обработка

### 1. Ошибки валидации

#### Ошибка схемы ресурса
```yaml
# Неверный ресурс
apiVersion: netguard.sgroups.io/v1beta1
kind: Service
metadata:
  name: invalid-service
  namespace: default
spec:
  # Отсутствует обязательное поле selfRef
  ingressPorts:
    - protocol: TCP
      port: "80"
```

**Ошибка валидации:**
```yaml
status:
  conditions:
    - type: Ready
      status: "False"
      lastTransitionTime: "2024-01-15T10:30:00Z"
      reason: "ValidationFailed"
      message: "spec.selfRef: Required value: selfRef is required"
  errorCode: "VALIDATION_ERROR"
  errorType: "SCHEMA_ERROR"
  fieldPath: "spec.selfRef"
```

#### Ошибка бизнес-правил
```yaml
# Конфликт портов
apiVersion: netguard.sgroups.io/v1beta1
kind: Service
metadata:
  name: conflicting-service
  namespace: default
spec:
  selfRef:
    name: conflicting-service
    namespace: default
  ingressPorts:
    - protocol: TCP
      port: "80"  # Порт уже занят другим сервисом
```

**Ошибка валидации:**
```yaml
status:
  conditions:
    - type: Ready
      status: "False"
      lastTransitionTime: "2024-01-15T10:30:00Z"
      reason: "ValidationFailed"
      message: "spec.ingressPorts[0]: Invalid value: TCP/80: port conflicts with existing service web-service"
  errorCode: "VALIDATION_ERROR"
  errorType: "BUSINESS_RULE_ERROR"
  fieldPath: "spec.ingressPorts[0]"
```

### 2. Сетевые ошибки

#### Конфигурация retry механизма
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: netguard-retry-config
  namespace: netguard-system
data:
  # Настройки retry
  retry_settings: |
    max_retries: 3
    initial_delay: "100ms"
    max_delay: "5s"
    backoff_multiplier: 2.0
    jitter: 0.1
  
  # Правила retry для разных ошибок
  retry_rules: |
    - error_type: "NETWORK_ERROR"
      retry_enabled: true
      max_retries: 5
    - error_type: "TIMEOUT_ERROR"
      retry_enabled: true
      max_retries: 3
    - error_type: "VALIDATION_ERROR"
      retry_enabled: false
      max_retries: 0
    - error_type: "BUSINESS_RULE_ERROR"
      retry_enabled: false
      max_retries: 0
```

#### Пример сетевой ошибки
```yaml
# Лог сетевой ошибки
{
  "level": "error",
  "timestamp": "2024-01-15T10:30:00Z",
  "operation": "create_service",
  "resource_name": "web-service",
  "namespace": "default",
  "error_type": "NETWORK_ERROR",
  "error": "connection refused",
  "retry_attempt": 1,
  "max_retries": 3,
  "next_retry_delay": "200ms"
}

# Успешный retry
{
  "level": "info",
  "timestamp": "2024-01-15T10:30:02Z",
  "operation": "create_service",
  "resource_name": "web-service",
  "namespace": "default",
  "status": "success",
  "retry_attempt": 2,
  "total_duration": "2.1s"
}
```

### 3. Ошибки базы данных

#### Конфигурация обработки ошибок БД
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: netguard-database-error-config
  namespace: netguard-system
data:
  # Настройки обработки ошибок БД
  database_error_handling: |
    - error_code: "23505"  # PostgreSQL unique_violation
      action: "ROLLBACK"
      user_message: "Resource already exists"
      log_level: "warn"
    - error_code: "23503"  # PostgreSQL foreign_key_violation
      action: "ROLLBACK"
      user_message: "Referenced resource does not exist"
      log_level: "error"
    - error_code: "23514"  # PostgreSQL check_violation
      action: "ROLLBACK"
      user_message: "Data validation failed"
      log_level: "error"
    - error_code: "08000"  # PostgreSQL connection_exception
      action: "RETRY"
      max_retries: 3
      user_message: "Database connection error"
      log_level: "error"
```

#### Пример ошибки БД
```yaml
# Ошибка уникальности
status:
  conditions:
    - type: Ready
      status: "False"
      lastTransitionTime: "2024-01-15T10:30:00Z"
      reason: "DatabaseError"
      message: "Resource already exists: service web-service in namespace default"
  errorCode: "DATABASE_ERROR"
  errorType: "UNIQUE_VIOLATION"
  databaseErrorCode: "23505"
  rollbackStatus: "COMPLETED"
```

### 4. Механизмы отката

#### Конфигурация откатов
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: netguard-rollback-config
  namespace: netguard-system
data:
  # Настройки откатов
  rollback_settings: |
    enabled: true
    automatic_rollback: true
    manual_rollback: true
    rollback_timeout: "30s"
    cleanup_resources: true
  
  # Правила отката
  rollback_rules: |
    - operation_type: "CREATE"
      trigger_conditions:
        - "VALIDATION_ERROR"
        - "BUSINESS_RULE_ERROR"
        - "DATABASE_ERROR"
      actions:
        - "DELETE_CREATED_RESOURCE"
        - "CLEANUP_METADATA"
        - "INVALIDATE_CACHE"
    - operation_type: "UPDATE"
      trigger_conditions:
        - "VALIDATION_ERROR"
        - "BUSINESS_RULE_ERROR"
        - "DATABASE_ERROR"
      actions:
        - "RESTORE_PREVIOUS_STATE"
        - "UPDATE_METADATA"
        - "INVALIDATE_CACHE"
    - operation_type: "DELETE"
      trigger_conditions:
        - "DEPENDENCY_ERROR"
        - "PERMISSION_ERROR"
      actions:
        - "RESTORE_RESOURCE"
        - "UPDATE_METADATA"
        - "INVALIDATE_CACHE"
```

#### Пример отката операции
```yaml
# Лог отката
{
  "level": "info",
  "timestamp": "2024-01-15T10:30:00Z",
  "operation": "rollback_create_service",
  "resource_name": "web-service",
  "namespace": "default",
  "trigger_error": "VALIDATION_ERROR",
  "rollback_actions": [
    "DELETE_CREATED_RESOURCE",
    "CLEANUP_METADATA",
    "INVALIDATE_CACHE"
  ],
  "rollback_status": "COMPLETED",
  "rollback_duration": "45ms"
}

# Статус после отката
status:
  conditions:
    - type: Ready
      status: "False"
      lastTransitionTime: "2024-01-15T10:30:00Z"
      reason: "RollbackCompleted"
      message: "Operation rolled back due to validation error"
  rollbackInfo:
    triggerError: "VALIDATION_ERROR"
    rollbackActions: ["DELETE_CREATED_RESOURCE", "CLEANUP_METADATA", "INVALIDATE_CACHE"]
    rollbackStatus: "COMPLETED"
    rollbackDuration: "45ms"
```

### 5. Обработка конфликтов

#### Конфигурация разрешения конфликтов
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: netguard-conflict-resolution
  namespace: netguard-system
data:
  # Стратегии разрешения конфликтов
  conflict_resolution: |
    - conflict_type: "RESOURCE_EXISTS"
      strategy: "REJECT"
      message: "Resource already exists"
    - conflict_type: "PORT_CONFLICT"
      strategy: "REJECT"
      message: "Port is already in use"
    - conflict_type: "DEPENDENCY_CONFLICT"
      strategy: "RETRY"
      max_retries: 3
      message: "Dependency conflict, retrying"
    - conflict_type: "VERSION_CONFLICT"
      strategy: "RETRY"
      max_retries: 1
      message: "Version conflict, retrying with latest version"
```

#### Пример конфликта версий
```yaml
# Конфликт версий
status:
  conditions:
    - type: Ready
      status: "False"
      lastTransitionTime: "2024-01-15T10:30:00Z"
      reason: "Conflict"
      message: "Version conflict: resource was modified by another operation"
  errorCode: "CONFLICT"
  errorType: "VERSION_CONFLICT"
  currentVersion: "123"
  expectedVersion: "122"
  conflictResolution: "RETRY"
```

### 6. Метрики ошибок

#### Prometheus метрики
```yaml
# Метрики ошибок
netguard_errors_total{error_type="validation_error",resource_type="service"} 5
netguard_errors_total{error_type="network_error",resource_type="service"} 2
netguard_errors_total{error_type="database_error",resource_type="service"} 1
netguard_errors_total{error_type="business_rule_error",resource_type="service"} 3

# Метрики retry
netguard_retry_total{operation="create",resource_type="service",status="success"} 8
netguard_retry_total{operation="create",resource_type="service",status="failed"} 2
netguard_retry_attempts{operation="create",resource_type="service",quantile="0.5"} 1
netguard_retry_attempts{operation="create",resource_type="service",quantile="0.9"} 3

# Метрики откатов
netguard_rollback_total{operation="create",status="completed"} 3
netguard_rollback_total{operation="update",status="completed"} 1
netguard_rollback_duration_seconds{operation="create",quantile="0.5"} 0.045
netguard_rollback_duration_seconds{operation="create",quantile="0.9"} 0.089
```

#### Grafana Dashboard
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: netguard-error-dashboard
  namespace: monitoring
data:
  dashboard.json: |
    {
      "dashboard": {
        "title": "Netguard Error Handling Metrics",
        "panels": [
          {
            "title": "Error Rate by Type",
            "type": "graph",
            "targets": [
              {
                "expr": "rate(netguard_errors_total[5m])",
                "legendFormat": "{{error_type}} - {{resource_type}}"
              }
            ]
          },
          {
            "title": "Retry Success Rate",
            "type": "graph",
            "targets": [
              {
                "expr": "rate(netguard_retry_total{status=\"success\"}[5m]) / rate(netguard_retry_total[5m])",
                "legendFormat": "{{operation}} - {{resource_type}}"
              }
            ]
          },
          {
            "title": "Rollback Duration",
            "type": "graph",
            "targets": [
              {
                "expr": "histogram_quantile(0.5, rate(netguard_rollback_duration_seconds_bucket[5m]))",
                "legendFormat": "{{operation}} - 50th percentile"
              },
              {
                "expr": "histogram_quantile(0.9, rate(netguard_rollback_duration_seconds_bucket[5m]))",
                "legendFormat": "{{operation}} - 90th percentile"
              }
            ]
          }
        ]
      }
    }
```

### 7. Логирование ошибок

#### Структура логов ошибок
```yaml
# Лог ошибки валидации
{
  "level": "error",
  "timestamp": "2024-01-15T10:30:00Z",
  "operation": "create_service",
  "resource_name": "invalid-service",
  "namespace": "default",
  "error_type": "VALIDATION_ERROR",
  "error_code": "VALIDATION_ERROR",
  "error_message": "spec.selfRef: Required value: selfRef is required",
  "field_path": "spec.selfRef",
  "user_message": "Validation failed: selfRef is required",
  "rollback_required": false
}

# Лог сетевой ошибки с retry
{
  "level": "warn",
  "timestamp": "2024-01-15T10:30:00Z",
  "operation": "create_service",
  "resource_name": "web-service",
  "namespace": "default",
  "error_type": "NETWORK_ERROR",
  "error_code": "NETWORK_ERROR",
  "error_message": "connection refused",
  "retry_attempt": 1,
  "max_retries": 3,
  "next_retry_delay": "200ms",
  "rollback_required": false
}

# Лог отката
{
  "level": "info",
  "timestamp": "2024-01-15T10:30:00Z",
  "operation": "rollback_create_service",
  "resource_name": "web-service",
  "namespace": "default",
  "trigger_error": "DATABASE_ERROR",
  "rollback_actions": ["DELETE_CREATED_RESOURCE", "CLEANUP_METADATA"],
  "rollback_status": "COMPLETED",
  "rollback_duration": "45ms"
}
```

#### Конфигурация логирования ошибок
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: netguard-error-logging
  namespace: netguard-system
data:
  log-level: "info"
  log-format: "json"
  log-fields: "operation,resource_name,namespace,error_type,error_code"
  error-log-level: "error"
  retry-log-level: "warn"
  rollback-log-level: "info"
  include-stack-trace: "true"
  include-error-details: "true"
``` 