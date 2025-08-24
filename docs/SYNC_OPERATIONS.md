# Операционное руководство по синхронизации с SGROUP

## Обзор

Данное руководство предназначено для операторов и администраторов системы Netguard PG Backend. Оно содержит практические инструкции по мониторингу, диагностике и устранению проблем синхронизации с SGROUP.

## Мониторинг синхронизации

### Ключевые метрики

#### 1. Метрики успешности синхронизации

```bash
# Общее количество запросов синхронизации
curl -s http://netguard-backend:8080/metrics | grep netguard_sync_requests_total

# Пример вывода:
netguard_sync_requests_total{subject_type="Groups",operation="Upsert",status="success"} 1245
netguard_sync_requests_total{subject_type="Groups",operation="Upsert",status="error"} 12
netguard_sync_requests_total{subject_type="Networks",operation="Upsert",status="success"} 567
netguard_sync_requests_total{subject_type="IEAgAgRules",operation="Upsert",status="success"} 890
```

#### 2. Метрики производительности

```bash
# Время выполнения синхронизации
curl -s http://netguard-backend:8080/metrics | grep netguard_sync_duration_seconds

# Пример вывода:
netguard_sync_duration_seconds{subject_type="Groups",operation="Upsert",quantile="0.5"} 0.125
netguard_sync_duration_seconds{subject_type="Groups",operation="Upsert",quantile="0.95"} 0.450
netguard_sync_duration_seconds{subject_type="Groups",operation="Upsert",quantile="0.99"} 1.200
```

#### 3. Метрики состояния соединения

```bash
# Статус соединения с SGROUP
curl -s http://netguard-backend:8080/metrics | grep netguard_sgroup_connection_status

# Пример вывода:
netguard_sgroup_connection_status{endpoint="sgroup-service:9090"} 1
```

### Dashboards в Grafana

#### Dashboard "SGROUP Sync Overview"

```json
{
  "dashboard": {
    "title": "SGROUP Sync Overview",
    "panels": [
      {
        "title": "Sync Success Rate",
        "type": "stat",
        "targets": [
          {
            "expr": "rate(netguard_sync_requests_total{status=\"success\"}[5m]) / rate(netguard_sync_requests_total[5m]) * 100"
          }
        ]
      },
      {
        "title": "Sync Requests per Second",
        "type": "graph",
        "targets": [
          {
            "expr": "rate(netguard_sync_requests_total[5m])",
            "legendFormat": "{{subject_type}} - {{operation}}"
          }
        ]
      },
      {
        "title": "Sync Duration",
        "type": "graph",
        "targets": [
          {
            "expr": "histogram_quantile(0.95, rate(netguard_sync_duration_seconds_bucket[5m]))",
            "legendFormat": "95th percentile"
          },
          {
            "expr": "histogram_quantile(0.50, rate(netguard_sync_duration_seconds_bucket[5m]))",
            "legendFormat": "50th percentile"
          }
        ]
      }
    ]
  }
}
```

### Алерты

#### 1. Высокий процент ошибок синхронизации

```yaml
groups:
- name: sgroup_sync_alerts
  rules:
  - alert: HighSyncErrorRate
    expr: |
      (
        rate(netguard_sync_requests_total{status="error"}[5m]) /
        rate(netguard_sync_requests_total[5m])
      ) * 100 > 5
    for: 2m
    labels:
      severity: warning
    annotations:
      summary: "High SGROUP sync error rate"
      description: "SGROUP sync error rate is {{ $value }}% for the last 5 minutes"
```

#### 2. SGROUP недоступен

```yaml
  - alert: SGROUPConnectionDown
    expr: netguard_sgroup_connection_status == 0
    for: 1m
    labels:
      severity: critical
    annotations:
      summary: "SGROUP connection is down"
      description: "Cannot connect to SGROUP service at {{ $labels.endpoint }}"
```

#### 3. Медленная синхронизация

```yaml
  - alert: SlowSyncPerformance
    expr: |
      histogram_quantile(0.95, rate(netguard_sync_duration_seconds_bucket[5m])) > 2.0
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "Slow SGROUP sync performance"
      description: "95th percentile sync duration is {{ $value }}s"
```

## Диагностика проблем

### Проверка состояния синхронизации

#### 1. Общий статус синхронизации

```bash
# Проверка статуса через API
curl -s http://netguard-backend:8080/sync/status | jq '.'

# Пример успешного ответа:
{
  "is_healthy": true,
  "last_sync_timestamp": 1640995200,
  "subjects": {
    "Groups": {
      "total_requests": 1257,
      "successful_syncs": 1245,
      "failed_syncs": 12,
      "last_sync_time": 1640995200,
      "average_latency": 125
    },
    "Networks": {
      "total_requests": 567,
      "successful_syncs": 567,
      "failed_syncs": 0,
      "last_sync_time": 1640995180,
      "average_latency": 98
    }
  }
}
```

#### 2. Health check SGROUP соединения

```bash
# Проверка здоровья SGROUP
curl -s http://netguard-backend:8080/health/sgroup | jq '.'

# Пример успешного ответа:
{
  "status": "healthy",
  "endpoint": "sgroup-service:9090",
  "last_check": "2024-01-01T12:00:00Z",
  "response_time": "45ms"
}

# Пример ответа при проблемах:
{
  "status": "unhealthy",
  "endpoint": "sgroup-service:9090",
  "last_check": "2024-01-01T12:00:00Z",
  "error": "connection refused"
}
```

### Анализ логов

#### 1. Логи синхронизации

```bash
# Просмотр логов синхронизации
kubectl logs -n netguard-system deployment/netguard-backend | grep -E "(sync|SYNC)" | tail -50

# Примеры логов:
2024-01-01T12:00:00Z INFO Sync completed successfully subject_type=Groups operation=Upsert entity=production/web-servers duration=125ms
2024-01-01T12:00:05Z ERROR Sync failed subject_type=Groups operation=Delete entity=production/old-servers error="SGROUP returned INVALID_ARGUMENT: group not found"
2024-01-01T12:00:05Z INFO Retry attempt 1/3 subject_type=Groups operation=Delete entity=production/old-servers delay=100ms
2024-01-01T12:00:10Z WARN Debouncing sync request subject_type=Networks entity=production/app-network reason="recent sync within 1s"
```

#### 2. Фильтрация по типу сущности

```bash
# Логи синхронизации AddressGroups
kubectl logs -n netguard-system deployment/netguard-backend | grep "subject_type=Groups"

# Логи синхронизации Networks
kubectl logs -n netguard-system deployment/netguard-backend | grep "subject_type=Networks"

# Логи синхронизации IEAgAgRules
kubectl logs -n netguard-system deployment/netguard-backend | grep "subject_type=IEAgAgRules"
```

#### 3. Логи ошибок

```bash
# Только ошибки синхронизации
kubectl logs -n netguard-system deployment/netguard-backend | grep -E "(ERROR|FATAL).*sync"

# Логи с контекстом (5 строк до и после ошибки)
kubectl logs -n netguard-system deployment/netguard-backend | grep -A 5 -B 5 "ERROR.*sync"
```

### Диагностические команды

#### 1. Проверка конфигурации

```bash
# Проверка конфигурации SGROUP
kubectl get configmap -n netguard-system netguard-config -o yaml | grep -A 20 sgroup

# Проверка секретов TLS
kubectl get secret -n netguard-system sgroup-tls-certs -o yaml
```

#### 2. Проверка сетевой связности

```bash
# Проверка доступности SGROUP из пода
kubectl exec -n netguard-system deployment/netguard-backend -- nslookup sgroup-service

# Проверка порта
kubectl exec -n netguard-system deployment/netguard-backend -- nc -zv sgroup-service 9090

# Проверка TLS соединения
kubectl exec -n netguard-system deployment/netguard-backend -- openssl s_client -connect sgroup-service:9090 -servername sgroup-service
```

#### 3. Проверка ресурсов

```bash
# Использование CPU и памяти
kubectl top pod -n netguard-system -l app=netguard-backend

# Описание пода для проверки событий
kubectl describe pod -n netguard-system -l app=netguard-backend
```

## Устранение проблем

### Частые проблемы и решения

#### 1. SGROUP недоступен

**Симптомы:**
- `netguard_sgroup_connection_status = 0`
- Ошибки "connection refused" в логах
- Накопление неотправленных запросов синхронизации

**Диагностика:**
```bash
# Проверка доступности SGROUP
kubectl exec -n netguard-system deployment/netguard-backend -- nc -zv sgroup-service 9090

# Проверка DNS разрешения
kubectl exec -n netguard-system deployment/netguard-backend -- nslookup sgroup-service
```

**Решения:**
1. Проверить статус SGROUP сервиса:
   ```bash
   kubectl get pods -n sgroup-system -l app=sgroup
   kubectl logs -n sgroup-system deployment/sgroup
   ```

2. Проверить сетевые политики:
   ```bash
   kubectl get networkpolicy -n netguard-system
   kubectl get networkpolicy -n sgroup-system
   ```

3. Перезапустить netguard-backend:
   ```bash
   kubectl rollout restart deployment/netguard-backend -n netguard-system
   ```

#### 2. Ошибки TLS аутентификации

**Симптомы:**
- Ошибки "tls: certificate verify failed" в логах
- Ошибки "UNAUTHENTICATED" от SGROUP

**Диагностика:**
```bash
# Проверка сертификатов
kubectl get secret -n netguard-system sgroup-tls-certs -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -text -noout

# Проверка срока действия
kubectl get secret -n netguard-system sgroup-tls-certs -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -dates -noout
```

**Решения:**
1. Обновить сертификаты:
   ```bash
   # Создать новый секрет с обновленными сертификатами
   kubectl create secret tls sgroup-tls-certs \
     --cert=client.crt \
     --key=client.key \
     -n netguard-system \
     --dry-run=client -o yaml | kubectl apply -f -
   ```

2. Перезапустить поды для загрузки новых сертификатов:
   ```bash
   kubectl rollout restart deployment/netguard-backend -n netguard-system
   ```

#### 3. Высокая задержка синхронизации

**Симптомы:**
- Высокие значения в метрике `netguard_sync_duration_seconds`
- Жалобы пользователей на медленное применение изменений

**Диагностика:**
```bash
# Проверка производительности SGROUP
curl -s http://netguard-backend:8080/metrics | grep netguard_sync_duration_seconds

# Проверка нагрузки на систему
kubectl top pod -n netguard-system
kubectl top node
```

**Решения:**
1. Увеличить ресурсы для netguard-backend:
   ```yaml
   resources:
     requests:
       cpu: 500m
       memory: 512Mi
     limits:
       cpu: 1000m
       memory: 1Gi
   ```

2. Настроить batch синхронизацию:
   ```yaml
   sync:
     batch:
       enabled: true
       max_size: 50
       timeout: "3s"
   ```

3. Оптимизировать debouncing:
   ```yaml
   sync:
     debouncing:
       enabled: true
       window: "500ms"
   ```

#### 4. Ошибки конвертации данных

**Симптомы:**
- Ошибки "failed to convert entity to sgroups proto" в логах
- Успешное создание ресурсов в Kubernetes, но неудачная синхронизация

**Диагностика:**
```bash
# Поиск ошибок конвертации
kubectl logs -n netguard-system deployment/netguard-backend | grep "convert.*proto"

# Проверка конкретного ресурса
kubectl get addressgroup problematic-group -o yaml
```

**Решения:**
1. Проверить корректность данных в ресурсе
2. Обновить версию netguard-backend с исправлениями
3. Временно отключить синхронизацию для проблемного типа:
   ```yaml
   sync:
     enabled_subjects:
       - Groups
       - Networks
       # - IEAgAgRules  # временно отключено
   ```

### Процедуры восстановления

#### 1. Полная ресинхронизация

```bash
# 1. Остановить синхронизацию
kubectl patch configmap netguard-config -n netguard-system --patch '{"data":{"sync.enabled":"false"}}'
kubectl rollout restart deployment/netguard-backend -n netguard-system

# 2. Очистить данные в SGROUP (если необходимо)
# Выполнить через SGROUP API или интерфейс

# 3. Включить синхронизацию с полной синхронизацией
kubectl patch configmap netguard-config -n netguard-system --patch '{"data":{"sync.enabled":"true","sync.force_full_sync":"true"}}'
kubectl rollout restart deployment/netguard-backend -n netguard-system

# 4. Мониторить процесс восстановления
kubectl logs -n netguard-system deployment/netguard-backend -f | grep sync
```

#### 2. Восстановление после сбоя SGROUP

```bash
# 1. Проверить накопленную очередь
curl -s http://netguard-backend:8080/sync/queue/status

# 2. После восстановления SGROUP проверить автоматическую обработку очереди
kubectl logs -n netguard-system deployment/netguard-backend | grep "queued.*sync"

# 3. При необходимости принудительно обработать очередь
curl -X POST http://netguard-backend:8080/sync/queue/process
```

## Настройка и оптимизация

### Конфигурация производительности

#### 1. Настройки retry

```yaml
sgroup:
  retry:
    max_retries: 3
    initial_delay: "100ms"
    max_delay: "5s"
    backoff_factor: 2.0
```

#### 2. Настройки batch операций

```yaml
sync:
  batch:
    enabled: true
    max_size: 100        # максимальный размер batch
    timeout: "5s"        # таймаут сбора batch
    max_wait: "10s"      # максимальное время ожидания
```

#### 3. Настройки debouncing

```yaml
sync:
  debouncing:
    enabled: true
    window: "1s"         # окно debouncing
    max_pending: 1000    # максимум отложенных операций
```

### Мониторинг производительности

#### 1. Ключевые SLI/SLO

```yaml
sli_slo:
  sync_success_rate:
    sli: "rate(netguard_sync_requests_total{status='success'}[5m]) / rate(netguard_sync_requests_total[5m])"
    slo: "> 0.99"  # 99% успешных синхронизаций
  
  sync_latency:
    sli: "histogram_quantile(0.95, rate(netguard_sync_duration_seconds_bucket[5m]))"
    slo: "< 1.0"   # 95% запросов быстрее 1 секунды
  
  sgroup_availability:
    sli: "netguard_sgroup_connection_status"
    slo: "> 0.999" # 99.9% доступности
```

#### 2. Capacity planning

```bash
# Анализ трендов нагрузки
# Запросы в секунду
rate(netguard_sync_requests_total[1h])

# Рост объема данных
increase(netguard_sync_requests_total[24h])

# Использование ресурсов
rate(container_cpu_usage_seconds_total{pod=~"netguard-backend.*"}[5m])
container_memory_usage_bytes{pod=~"netguard-backend.*"}
```

## Безопасность

### Аудит синхронизации

#### 1. Логирование операций

```yaml
logging:
  sync:
    level: "info"
    audit: true
    include_data: false  # не логировать содержимое для безопасности
```

#### 2. Мониторинг безопасности

```bash
# Неудачные попытки аутентификации
kubectl logs -n netguard-system deployment/netguard-backend | grep "UNAUTHENTICATED"

# Подозрительная активность
kubectl logs -n netguard-system deployment/netguard-backend | grep -E "(PERMISSION_DENIED|FORBIDDEN)"
```

### Ротация сертификатов

```bash
# Автоматическая ротация через cert-manager
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: sgroup-client-cert
  namespace: netguard-system
spec:
  secretName: sgroup-tls-certs
  issuerRef:
    name: sgroup-ca-issuer
    kind: ClusterIssuer
  commonName: netguard-client
  duration: 8760h  # 1 год
  renewBefore: 720h # обновлять за 30 дней до истечения
```

## Заключение

Данное руководство покрывает основные аспекты операционного управления синхронизацией с SGROUP:

1. **Мониторинг** - ключевые метрики, dashboards и алерты
2. **Диагностика** - методы выявления и анализа проблем
3. **Устранение проблем** - решения для частых проблем
4. **Оптимизация** - настройки производительности и capacity planning
5. **Безопасность** - аудит и управление сертификатами

Регулярное применение этих практик обеспечит стабильную и эффективную работу системы синхронизации.