# Сценарии синхронизации с SGROUP

## Обзор

Данный документ описывает практические сценарии использования системы синхронизации с SGROUP для различных типов сущностей: AddressGroup, Network и IEAgAgRule. Каждый сценарий включает пошаговое описание процесса, примеры запросов и ожидаемые результаты.

## Сценарий 1: Создание AddressGroup с автоматической синхронизацией

### Описание
Создание новой группы адресов через Kubernetes API с автоматической синхронизацией в SGROUP.

### Предварительные условия
- SGROUP сервис доступен и настроен
- Система синхронизации включена
- Пользователь имеет права на создание AddressGroup

### Шаги выполнения

#### 1. Создание AddressGroup через kubectl

```yaml
apiVersion: netguard.sgroups.io/v1beta1
kind: AddressGroup
metadata:
  name: web-servers
  namespace: production
spec:
  addresses:
    - "10.0.1.10"
    - "10.0.1.11" 
    - "10.0.1.12"
  description: "Web servers in production environment"
```

```bash
kubectl apply -f web-servers-addressgroup.yaml
```

#### 2. Валидация через Admission Controller

```
INFO: Validating AddressGroup creation
INFO: Schema validation passed
INFO: Business rules validation passed
INFO: AddressGroup web-servers/production is valid
```

#### 3. Сохранение в PostgreSQL

```sql
INSERT INTO address_groups (name, namespace, addresses, description, created_at)
VALUES ('web-servers', 'production', 
        ARRAY['10.0.1.10', '10.0.1.11', '10.0.1.12'],
        'Web servers in production environment',
        NOW());
```

#### 4. Автоматическая синхронизация с SGROUP

```
INFO: Triggering sync for AddressGroup production/web-servers
INFO: Converting AddressGroup to SGROUP format
INFO: Sending sync request to SGROUP
```

**Sync Request:**
```json
{
  "operation": "Upsert",
  "subject_type": "Groups",
  "data": {
    "groups": [
      {
        "name": "production/web-servers",
        "members": ["10.0.1.10", "10.0.1.11", "10.0.1.12"]
      }
    ]
  }
}
```

#### 5. Подтверждение успешной синхронизации

```
INFO: SGROUP sync completed successfully
INFO: AddressGroup production/web-servers synchronized
```

### Ожидаемый результат
- AddressGroup создана в Kubernetes
- Данные сохранены в PostgreSQL
- Группа синхронизирована в SGROUP как "production/web-servers"
- Метрики синхронизации обновлены

---

## Сценарий 2: Обновление Network с конфликтом имен

### Описание
Обновление существующей сети с обработкой конфликта имен в SGROUP.

### Предварительные условия
- Network уже существует в системе
- В SGROUP есть конфликтующее имя сети

### Шаги выполнения

#### 1. Обновление Network

```yaml
apiVersion: netguard.sgroups.io/v1beta1
kind: Network
metadata:
  name: database-network
  namespace: production
spec:
  cidr: "10.0.2.0/24"
  description: "Updated database network range"
```

#### 2. Попытка синхронизации

```
INFO: Triggering sync for Network production/database-network
INFO: Converting Network to SGROUP format
INFO: Sending sync request to SGROUP
```

**Sync Request:**
```json
{
  "operation": "Upsert",
  "subject_type": "Networks",
  "data": {
    "networks": [
      {
        "name": "production/database-network",
        "network": {
          "CIDR": "10.0.2.0/24"
        }
      }
    ]
  }
}
```

#### 3. Обработка ошибки конфликта

```
ERROR: SGROUP sync failed: ALREADY_EXISTS - Network with name 'production/database-network' already exists
INFO: Initiating retry with exponential backoff
INFO: Retry attempt 1/3 after 100ms
```

#### 4. Retry с обновленной стратегией

```
INFO: Switching to FullSync operation to resolve conflict
INFO: Sending FullSync request to SGROUP
```

**Updated Sync Request:**
```json
{
  "operation": "FullSync",
  "subject_type": "Networks",
  "data": {
    "networks": [
      {
        "name": "production/database-network",
        "network": {
          "CIDR": "10.0.2.0/24"
        }
      }
    ]
  }
}
```

#### 5. Успешная синхронизация

```
INFO: SGROUP FullSync completed successfully
INFO: Network production/database-network synchronized
INFO: Conflict resolved
```

### Ожидаемый результат
- Network обновлена в Kubernetes
- Конфликт имен разрешен через FullSync
- Сеть корректно синхронизирована в SGROUP
- Метрики ошибок и retry обновлены

---

## Сценарий 3: Batch синхронизация IEAgAgRules

### Описание
Массовое создание правил IEAgAg с использованием batch синхронизации для повышения производительности.

### Предварительные условия
- Batch синхронизация включена
- Существуют соответствующие AddressGroups

### Шаги выполнения

#### 1. Создание множественных IEAgAgRules

```yaml
# Rule 1: Web to DB
apiVersion: netguard.sgroups.io/v1beta1
kind: IEAgAgRule
metadata:
  name: web-to-db-mysql
  namespace: production
spec:
  from: "web-servers"
  to: "db-servers"
  ports:
    - protocol: "TCP"
      port: 3306
---
# Rule 2: Web to Cache
apiVersion: netguard.sgroups.io/v1beta1
kind: IEAgAgRule
metadata:
  name: web-to-cache-redis
  namespace: production
spec:
  from: "web-servers"
  to: "cache-servers"
  ports:
    - protocol: "TCP"
      port: 6379
---
# Rule 3: API to DB
apiVersion: netguard.sgroups.io/v1beta1
kind: IEAgAgRule
metadata:
  name: api-to-db-postgres
  namespace: production
spec:
  from: "api-servers"
  to: "db-servers"
  ports:
    - protocol: "TCP"
      port: 5432
```

#### 2. Группировка для batch синхронизации

```
INFO: Collecting IEAgAgRules for batch sync
INFO: Found 3 IEAgAgRules for batch processing
INFO: Grouping by subject type: IEAgAgRules
```

#### 3. Batch синхронизация

```
INFO: Starting batch sync for 3 IEAgAgRules
INFO: Converting all rules to SGROUP format
INFO: Creating batch sync request
```

**Batch Sync Request:**
```json
{
  "operation": "Upsert",
  "subject_type": "IEAgAgRules",
  "data": {
    "rules": [
      {
        "from_group": "production/web-servers",
        "to_group": "production/db-servers",
        "ports": [
          {
            "protocol": "TCP",
            "port_range": "3306-3306"
          }
        ]
      },
      {
        "from_group": "production/web-servers",
        "to_group": "production/cache-servers",
        "ports": [
          {
            "protocol": "TCP",
            "port_range": "6379-6379"
          }
        ]
      },
      {
        "from_group": "production/api-servers",
        "to_group": "production/db-servers",
        "ports": [
          {
            "protocol": "TCP",
            "port_range": "5432-5432"
          }
        ]
      }
    ]
  }
}
```

#### 4. Успешная batch синхронизация

```
INFO: SGROUP batch sync completed successfully
INFO: Synchronized 3 IEAgAgRules in single request
INFO: Batch sync duration: 250ms
```

### Ожидаемый результат
- Все 3 IEAgAgRules созданы в Kubernetes
- Правила синхронизированы в SGROUP одним batch запросом
- Значительное улучшение производительности по сравнению с индивидуальными запросами
- Метрики batch операций обновлены

---

## Сценарий 4: Удаление AddressGroup с каскадной очисткой

### Описание
Удаление AddressGroup с автоматической очисткой связанных правил и синхронизацией удаления в SGROUP.

### Предварительные условия
- AddressGroup существует и используется в правилах
- Настроена каскадная очистка

### Шаги выполнения

#### 1. Удаление AddressGroup

```bash
kubectl delete addressgroup web-servers -n production
```

#### 2. Проверка зависимостей

```
INFO: Checking dependencies for AddressGroup production/web-servers
INFO: Found 2 dependent IEAgAgRules:
  - production/web-to-db-mysql
  - production/web-to-cache-redis
INFO: Initiating cascading delete
```

#### 3. Удаление зависимых правил

```
INFO: Deleting dependent IEAgAgRule production/web-to-db-mysql
INFO: Deleting dependent IEAgAgRule production/web-to-cache-redis
INFO: Triggering sync for deleted rules
```

**Sync Request for Rules:**
```json
{
  "operation": "Delete",
  "subject_type": "IEAgAgRules",
  "data": {
    "rules": [
      {
        "from_group": "production/web-servers",
        "to_group": "production/db-servers"
      },
      {
        "from_group": "production/web-servers", 
        "to_group": "production/cache-servers"
      }
    ]
  }
}
```

#### 4. Удаление AddressGroup

```
INFO: Deleting AddressGroup production/web-servers
INFO: Triggering sync for deleted AddressGroup
```

**Sync Request for AddressGroup:**
```json
{
  "operation": "Delete",
  "subject_type": "Groups",
  "data": {
    "groups": [
      {
        "name": "production/web-servers"
      }
    ]
  }
}
```

#### 5. Подтверждение удаления

```
INFO: SGROUP sync completed successfully
INFO: AddressGroup production/web-servers and dependent rules deleted
INFO: Cascading delete completed
```

### Ожидаемый результат
- AddressGroup удалена из Kubernetes
- Зависимые IEAgAgRules автоматически удалены
- Все связанные данные удалены из SGROUP
- Метрики удаления обновлены

---

## Сценарий 5: Восстановление после сбоя SGROUP

### Описание
Обработка недоступности SGROUP сервиса с последующим восстановлением и ресинхронизацией.

### Предварительные условия
- SGROUP сервис временно недоступен
- В системе есть несинхронизированные изменения

### Шаги выполнения

#### 1. Обнаружение сбоя

```
ERROR: SGROUP health check failed: connection refused
ERROR: Failed to sync AddressGroup production/new-servers: UNAVAILABLE
INFO: SGROUP marked as unhealthy
INFO: Queuing sync operations for retry
```

#### 2. Накопление изменений

```
INFO: AddressGroup production/app-servers created (queued for sync)
INFO: Network production/app-network updated (queued for sync)  
INFO: IEAgAgRule production/app-to-db deleted (queued for sync)
INFO: 3 operations queued for sync when SGROUP becomes available
```

#### 3. Восстановление SGROUP

```
INFO: SGROUP health check successful
INFO: SGROUP marked as healthy
INFO: Starting queued sync operations
```

#### 4. Ресинхронизация накопленных изменений

```
INFO: Processing 3 queued sync operations
INFO: Sync 1/3: AddressGroup production/app-servers (Upsert)
INFO: Sync 2/3: Network production/app-network (Upsert)
INFO: Sync 3/3: IEAgAgRule production/app-to-db (Delete)
```

**Batch Resync Request:**
```json
{
  "operation": "FullSync",
  "subject_type": "Mixed",
  "data": {
    "groups": [
      {
        "name": "production/app-servers",
        "members": ["10.0.3.10", "10.0.3.11"]
      }
    ],
    "networks": [
      {
        "name": "production/app-network",
        "network": {
          "CIDR": "10.0.3.0/24"
        }
      }
    ],
    "rules_to_delete": [
      {
        "from_group": "production/app-servers",
        "to_group": "production/db-servers"
      }
    ]
  }
}
```

#### 5. Успешная ресинхронизация

```
INFO: All queued sync operations completed successfully
INFO: SGROUP fully synchronized
INFO: Queue cleared
```

### Ожидаемый результат
- Система корректно обработала недоступность SGROUP
- Все изменения накоплены в очереди
- После восстановления выполнена полная ресинхронизация
- Консистентность данных восстановлена

---

## Сценарий 6: Мониторинг и диагностика синхронизации

### Описание
Использование метрик и диагностических инструментов для мониторинга состояния синхронизации.

### Шаги выполнения

#### 1. Проверка метрик синхронизации

```bash
# Общие метрики синхронизации
curl http://backend:8080/metrics | grep sync

# Результат:
netguard_sync_requests_total{subject_type="Groups",operation="Upsert",status="success"} 45
netguard_sync_requests_total{subject_type="Networks",operation="Upsert",status="success"} 23
netguard_sync_requests_total{subject_type="IEAgAgRules",operation="Upsert",status="success"} 67
netguard_sync_requests_total{subject_type="Groups",operation="Delete",status="error"} 2
netguard_sync_duration_seconds{subject_type="Groups",operation="Upsert"} 0.125
netguard_sgroup_connection_status{endpoint="sgroup-service:9090"} 1
```

#### 2. Проверка статуса синхронизации

```bash
# Статус синхронизации через API
curl http://backend:8080/sync/status

# Результат:
{
  "is_healthy": true,
  "last_sync_timestamp": 1640995200,
  "subjects": {
    "Groups": {
      "total_requests": 47,
      "successful_syncs": 45,
      "failed_syncs": 2,
      "last_sync_time": 1640995200,
      "average_latency": 125
    },
    "Networks": {
      "total_requests": 23,
      "successful_syncs": 23,
      "failed_syncs": 0,
      "last_sync_time": 1640995180,
      "average_latency": 98
    },
    "IEAgAgRules": {
      "total_requests": 67,
      "successful_syncs": 67,
      "failed_syncs": 0,
      "last_sync_time": 1640995195,
      "average_latency": 156
    }
  }
}
```

#### 3. Диагностика проблем

```bash
# Проверка логов синхронизации
kubectl logs -n netguard-system deployment/netguard-backend | grep sync

# Результат:
2024-01-01T12:00:00Z INFO Sync completed successfully subject_type=Groups operation=Upsert duration=125ms
2024-01-01T12:00:05Z ERROR Sync failed subject_type=Groups operation=Delete error="SGROUP returned INVALID_ARGUMENT"
2024-01-01T12:00:05Z INFO Retry attempt 1/3 subject_type=Groups operation=Delete delay=100ms
```

#### 4. Health check SGROUP соединения

```bash
# Проверка здоровья SGROUP
curl http://backend:8080/health/sgroup

# Результат:
{
  "status": "healthy",
  "endpoint": "sgroup-service:9090",
  "last_check": "2024-01-01T12:00:00Z",
  "response_time": "45ms"
}
```

### Ожидаемый результат
- Получена полная картина состояния синхронизации
- Выявлены проблемные области (2 неудачных удаления Groups)
- Подтверждена работоспособность SGROUP соединения
- Собраны данные для оптимизации производительности

---

## Заключение

Представленные сценарии покрывают основные случаи использования системы синхронизации с SGROUP:

1. **Стандартные операции**: Создание, обновление, удаление сущностей
2. **Обработка ошибок**: Конфликты имен, недоступность сервиса
3. **Оптимизация производительности**: Batch операции
4. **Отказоустойчивость**: Восстановление после сбоев
5. **Мониторинг**: Метрики и диагностика

Каждый сценарий включает детальные шаги, примеры запросов и ожидаемые результаты, что позволяет использовать их как для понимания работы системы, так и для тестирования и отладки.