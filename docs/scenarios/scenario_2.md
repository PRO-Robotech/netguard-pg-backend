# Сценарий 2: Валидация через Admission Controllers

## Описание
Система выполняет многоуровневую валидацию ресурсов через Admission Controllers на уровне Aggregated API Server. Валидация включает проверку схемы, бизнес-правил и зависимостей.

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
participant VAL as Validator
participant GRPC as gRPC Client
participant BL as Business Logic

User->>K8S: kubectl apply -f resource.yaml
K8S->>AGG: POST /apis/netguard.sgroups.io/v1beta1/resources
AGG->>AC: ValidateCreate(resource)

AC->>VAL: SchemaValidation(resource)
VAL-->>AC: SchemaValidationResult

alt Schema validation failed
    AC-->>AGG: ValidationResult{Allowed: false, SchemaError}
    AGG-->>K8S: 400 Bad Request
    K8S-->>User: Schema validation error
else Schema validation passed
    AC->>VAL: BusinessRuleValidation(resource)
    VAL->>GRPC: CheckDependencies(resource)
    GRPC->>BL: ValidateDependencies(resource)
    BL-->>GRPC: DependencyValidationResult
    GRPC-->>VAL: DependencyCheckResult
    VAL-->>AC: BusinessValidationResult
    
    alt Business validation failed
        AC-->>AGG: ValidationResult{Allowed: false, BusinessError}
        AGG-->>K8S: 400 Bad Request
        K8S-->>User: Business rule validation error
    else Business validation passed
        AC-->>AGG: ValidationResult{Allowed: true}
        AGG->>AGG: Proceed with creation
    end
end
@enduml
```

## Примеры валидации

### 1. Schema Validation

#### JSON Schema для Service
```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: services.netguard.sgroups.io
spec:
  group: netguard.sgroups.io
  names:
    kind: Service
    plural: services
    singular: service
  scope: Namespaced
  versions:
    - name: v1beta1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          required: [spec]
          properties:
            spec:
              type: object
              required: [selfRef]
              properties:
                selfRef:
                  type: object
                  required: [name, namespace]
                  properties:
                    name:
                      type: string
                      pattern: '^[a-z0-9]([a-z0-9-]*[a-z0-9])?$'
                      minLength: 1
                      maxLength: 253
                    namespace:
                      type: string
                      pattern: '^[a-z0-9]([a-z0-9-]*[a-z0-9])?$'
                      minLength: 1
                      maxLength: 63
                description:
                  type: string
                  maxLength: 1000
                ingressPorts:
                  type: array
                  maxItems: 100
                  items:
                    type: object
                    required: [protocol, port]
                    properties:
                      protocol:
                        type: string
                        enum: [TCP, UDP]
                      port:
                        type: string
                        pattern: '^([1-9]|[1-9]\d{1,3}|[1-5]\d{4}|6[0-4]\d{3}|65[0-4]\d{2}|655[0-2]\d|6553[0-5])$'
                      description:
                        type: string
                        maxLength: 255
```

### 2. Примеры успешной валидации

#### Валидный Service ресурс
```yaml
apiVersion: netguard.sgroups.io/v1beta1
kind: Service
metadata:
  name: valid-service
  namespace: default
spec:
  selfRef:
    name: valid-service
    namespace: default
  description: "A valid service with proper schema"
  ingressPorts:
    - protocol: TCP
      port: "80"
      description: "HTTP port"
    - protocol: TCP
      port: "443"
      description: "HTTPS port"
```

**Результат валидации:**
```yaml
status:
  conditions:
    - type: Ready
      status: "True"
      lastTransitionTime: "2024-01-15T10:30:00Z"
      reason: "Validated"
      message: "Service validation passed"
```

### 3. Примеры ошибок валидации

#### Ошибка: неверный формат имени
```yaml
apiVersion: netguard.sgroups.io/v1beta1
kind: Service
metadata:
  name: Invalid-Service  # Неверный формат - заглавные буквы
  namespace: default
spec:
  selfRef:
    name: Invalid-Service
    namespace: default
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
      message: "spec.selfRef.name: Invalid value: Invalid-Service: must match regex '^[a-z0-9]([a-z0-9-]*[a-z0-9])?$'"
```

#### Ошибка: неверный протокол
```yaml
apiVersion: netguard.sgroups.io/v1beta1
kind: Service
metadata:
  name: invalid-protocol-service
  namespace: default
spec:
  selfRef:
    name: invalid-protocol-service
    namespace: default
  ingressPorts:
    - protocol: ICMP  # Неверный протокол
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
      message: "spec.ingressPorts[0].protocol: Unsupported value: ICMP: supported values: TCP, UDP"
```

#### Ошибка: неверный номер порта
```yaml
apiVersion: netguard.sgroups.io/v1beta1
kind: Service
metadata:
  name: invalid-port-service
  namespace: default
spec:
  selfRef:
    name: invalid-port-service
    namespace: default
  ingressPorts:
    - protocol: TCP
      port: "99999"  # Неверный номер порта
```

**Ошибка валидации:**
```yaml
status:
  conditions:
    - type: Ready
      status: "False"
      lastTransitionTime: "2024-01-15T10:30:00Z"
      reason: "ValidationFailed"
      message: "spec.ingressPorts[0].port: Invalid value: 99999: must match regex '^([1-9]|[1-9]\d{1,3}|[1-5]\d{4}|6[0-4]\d{3}|65[0-4]\d{2}|655[0-2]\d|6553[0-5])$'"
```

### 4. Business Rule Validation

#### Конфигурация бизнес-правил
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: netguard-business-rules
  namespace: netguard-system
data:
  # Правила уникальности
  uniqueness_rules: |
    - resource_type: Service
      fields: [spec.selfRef.name, spec.selfRef.namespace]
      scope: namespace
    - resource_type: AddressGroup
      fields: [spec.name, spec.namespace]
      scope: namespace
    - resource_type: AddressGroupBinding
      fields: [spec.serviceRef.name, spec.addressGroupRef.name]
      scope: namespace
  
  # Правила конфликтов портов
  port_conflict_rules: |
    - resource_type: Service
      check_ports: true
      scope: namespace
      allowed_protocols: [TCP, UDP]
      port_range: [1, 65535]
  
  # Правила зависимостей
  dependency_rules: |
    - resource_type: Service
      dependencies:
        - type: AddressGroup
          field: spec.addressGroups
          required: false
        - type: AddressGroupBinding
          field: spec.bindings
          required: false
```

### 5. Примеры бизнес-правил

#### Ошибка: дублирование имени сервиса
```yaml
# Первый сервис
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
---
# Второй сервис с тем же именем (ошибка)
apiVersion: netguard.sgroups.io/v1beta1
kind: Service
metadata:
  name: web-service  # Дублирование имени
  namespace: default
spec:
  selfRef:
    name: web-service
    namespace: default
  ingressPorts:
    - protocol: TCP
      port: "8080"
```

**Ошибка валидации:**
```yaml
status:
  conditions:
    - type: Ready
      status: "False"
      lastTransitionTime: "2024-01-15T10:30:00Z"
      reason: "ValidationFailed"
      message: "spec.selfRef.name: Duplicate value: web-service: service with this name already exists in namespace default"
```

#### Ошибка: конфликт портов
```yaml
# Первый сервис
apiVersion: netguard.sgroups.io/v1beta1
kind: Service
metadata:
  name: web-service-1
  namespace: default
spec:
  selfRef:
    name: web-service-1
    namespace: default
  ingressPorts:
    - protocol: TCP
      port: "80"
---
# Второй сервис с конфликтующим портом
apiVersion: netguard.sgroups.io/v1beta1
kind: Service
metadata:
  name: web-service-2
  namespace: default
spec:
  selfRef:
    name: web-service-2
    namespace: default
  ingressPorts:
    - protocol: TCP
      port: "80"  # Конфликт порта
```

**Ошибка валидации:**
```yaml
status:
  conditions:
    - type: Ready
      status: "False"
      lastTransitionTime: "2024-01-15T10:30:00Z"
      reason: "ValidationFailed"
      message: "spec.ingressPorts[0]: Invalid value: TCP/80: port TCP/80 conflicts with service web-service-1"
```

#### Ошибка: отсутствующая зависимость
```yaml
apiVersion: netguard.sgroups.io/v1beta1
kind: Service
metadata:
  name: service-with-dependency
  namespace: default
spec:
  selfRef:
    name: service-with-dependency
    namespace: default
  addressGroups:
    - identifier:
        name: non-existent-group  # Несуществующая группа
        namespace: default
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
      message: "spec.addressGroups[0]: Invalid value: non-existent-group: address group default/non-existent-group does not exist"
```

### 6. Конфигурация Admission Controller

#### Настройка валидации
```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: netguard-validating-webhook
webhooks:
  - name: netguard.sgroups.io
    clientConfig:
      service:
        namespace: netguard-system
        name: netguard-webhook
        path: "/validate"
        port: 8443
    rules:
      - apiGroups: ["netguard.sgroups.io"]
        apiVersions: ["v1beta1"]
        operations: ["CREATE", "UPDATE"]
        resources: ["services", "addressgroups", "addressgroupbindings"]
    failurePolicy: Fail
    sideEffects: None
    admissionReviewVersions: ["v1"]
```

#### Конфигурация неизменяемых полей
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: netguard-immutability-rules
  namespace: netguard-system
data:
  immutable_fields: |
    - resource_type: Service
      fields:
        - spec.selfRef.name
        - spec.selfRef.namespace
    - resource_type: AddressGroup
      fields:
        - spec.name
        - spec.namespace
    - resource_type: AddressGroupBinding
      fields:
        - spec.serviceRef.name
        - spec.addressGroupRef.name
```

### 7. Метрики и мониторинг

#### Prometheus метрики
```yaml
# Метрики валидации
netguard_validation_total{resource_type="service",validation_type="schema",status="success"} 1
netguard_validation_total{resource_type="service",validation_type="schema",status="failed"} 0
netguard_validation_total{resource_type="service",validation_type="business_rule",status="success"} 1
netguard_validation_total{resource_type="service",validation_type="business_rule",status="failed"} 0

# Время валидации
netguard_validation_duration_seconds{resource_type="service",validation_type="schema",quantile="0.5"} 0.015
netguard_validation_duration_seconds{resource_type="service",validation_type="business_rule",quantile="0.5"} 0.045
```

#### Grafana Dashboard
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: netguard-validation-dashboard
  namespace: monitoring
data:
  dashboard.json: |
    {
      "dashboard": {
        "title": "Netguard Validation Metrics",
        "panels": [
          {
            "title": "Validation Success Rate",
            "type": "graph",
            "targets": [
              {
                "expr": "rate(netguard_validation_total{status=\"success\"}[5m]) / rate(netguard_validation_total[5m])",
                "legendFormat": "{{resource_type}} - {{validation_type}}"
              }
            ]
          },
          {
            "title": "Validation Errors",
            "type": "graph",
            "targets": [
              {
                "expr": "rate(netguard_validation_total{status=\"failed\"}[5m])",
                "legendFormat": "{{resource_type}} - {{validation_type}}"
              }
            ]
          }
        ]
      }
    }
```

### 8. Логирование

#### Структура логов валидации
```yaml
# Лог успешной валидации
{
  "level": "info",
  "timestamp": "2024-01-15T10:30:00Z",
  "resource_type": "service",
  "resource_name": "web-service",
  "namespace": "default",
  "validation_type": "schema",
  "status": "success",
  "duration_ms": 15
}

# Лог ошибки валидации
{
  "level": "error",
  "timestamp": "2024-01-15T10:30:00Z",
  "resource_type": "service",
  "resource_name": "invalid-service",
  "namespace": "default",
  "validation_type": "schema",
  "status": "failed",
  "error": "spec.selfRef.name: Invalid value: Invalid-Service: must match regex",
  "field_path": "spec.selfRef.name",
  "invalid_value": "Invalid-Service"
}
```

#### Конфигурация логирования
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: netguard-validation-logging
  namespace: netguard-system
data:
  log-level: "info"
  log-format: "json"
  log-fields: "resource_type,resource_name,namespace,validation_type,status"
  validation-log-level: "debug"
``` 