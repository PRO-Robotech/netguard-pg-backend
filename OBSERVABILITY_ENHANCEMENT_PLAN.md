# План доработок: Observability и Monitoring

## Цель
Добавить полноценную observability в Kubernetes Aggregated API с использованием современных инструментов мониторинга.

---

## Этап 1: OpenTelemetry Integration

### 1.1 Базовая настройка OpenTelemetry
- [ ] **Задача:** Интегрировать OpenTelemetry для distributed tracing
- [ ] **Зачем:** Отслеживание запросов через все компоненты системы

**Зависимости:**
```go
require (
    go.opentelemetry.io/otel v1.24.0
    go.opentelemetry.io/otel/trace v1.24.0
    go.opentelemetry.io/otel/exporters/jaeger v1.24.0
    go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.24.0
    go.opentelemetry.io/otel/sdk v1.24.0
    go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.49.0
    go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.49.0
)
```

**Файл:** `internal/k8s/observability/tracing.go`

```go
func setupTracing(serviceName string) (func(), error) {
    // Jaeger exporter
    exp, err := jaeger.New(jaeger.WithCollectorEndpoint(
        jaeger.WithEndpoint("http://jaeger:14268/api/traces"),
    ))
    if err != nil {
        return nil, err
    }
    
    tp := trace.NewTracerProvider(
        trace.WithBatcher(exp),
        trace.WithResource(resource.NewWithAttributes(
            semconv.SchemaURL,
            semconv.ServiceName(serviceName),
            semconv.ServiceVersion("v1beta1"),
        )),
    )
    
    otel.SetTracerProvider(tp)
    
    return func() { tp.Shutdown(context.Background()) }, nil
}
```

### 1.2 Tracing в Backend Client
- [ ] **Задача:** Добавить tracing во все gRPC вызовы
- [ ] **Зачем:** Отслеживание latency и ошибок в backend

**Файл:** `internal/k8s/client/backend_traced.go`

```go
func (c *TracedBackendClient) CreateService(ctx context.Context, svc *proto.Service) error {
    ctx, span := otel.Tracer("netguard-apiserver").Start(ctx, "backend.CreateService")
    defer span.End()
    
    span.SetAttributes(
        attribute.String("service.name", svc.Name),
        attribute.String("service.namespace", svc.Namespace),
        attribute.Int("service.ports_count", len(svc.IngressPorts)),
    )
    
    err := c.backend.CreateService(ctx, svc)
    if err != nil {
        span.RecordError(err)
        span.SetStatus(codes.Error, err.Error())
    }
    
    return err
}
```

### 1.3 Tracing в Storage Operations
- [ ] **Задача:** Добавить spans для всех Storage операций
- [ ] **Зачем:** Полная картина обработки Kubernetes API запросов

**Файл:** `internal/k8s/registry/service/storage_traced.go`

```go
func (s *TracedServiceStorage) Create(ctx context.Context, obj runtime.Object, ...) (runtime.Object, error) {
    ctx, span := otel.Tracer("netguard-apiserver").Start(ctx, "storage.service.Create")
    defer span.End()
    
    service := obj.(*v1beta1.Service)
    span.SetAttributes(
        attribute.String("k8s.resource.name", service.Name),
        attribute.String("k8s.resource.namespace", service.Namespace),
        attribute.String("k8s.resource.kind", "Service"),
    )
    
    result, err := s.storage.Create(ctx, obj, createValidation, options)
    if err != nil {
        span.RecordError(err)
        span.SetStatus(codes.Error, err.Error())
    }
    
    return result, err
}
```

---

## Этап 2: Prometheus Metrics

### 2.1 Базовые метрики API Server
- [ ] **Задача:** Добавить стандартные метрики для Kubernetes API
- [ ] **Зачем:** Мониторинг производительности и здоровья API

**Файл:** `internal/k8s/metrics/api_metrics.go`

```go
var (
    // HTTP метрики
    httpRequestsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "netguard_api_http_requests_total",
            Help: "Total number of HTTP requests",
        },
        []string{"method", "code", "resource"},
    )
    
    httpRequestDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "netguard_api_http_request_duration_seconds",
            Help:    "Duration of HTTP requests",
            Buckets: prometheus.DefBuckets,
        },
        []string{"method", "resource"},
    )
    
    // Backend метрики
    backendRequestsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "netguard_backend_requests_total",
            Help: "Total number of backend requests",
        },
        []string{"method", "status"},
    )
    
    backendRequestDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "netguard_backend_request_duration_seconds",
            Help:    "Duration of backend requests",
            Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
        },
        []string{"method"},
    )
)
```

### 2.2 Business метрики
- [ ] **Задача:** Добавить метрики специфичные для netguard
- [ ] **Зачем:** Мониторинг бизнес-логики и использования

**Файл:** `internal/k8s/metrics/business_metrics.go`

```go
var (
    // Ресурсы
    resourcesTotal = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "netguard_resources_total",
            Help: "Total number of netguard resources",
        },
        []string{"kind", "namespace"},
    )
    
    // Валидации
    validationFailuresTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "netguard_validation_failures_total",
            Help: "Total number of validation failures",
        },
        []string{"kind", "reason"},
    )
    
    // Circuit Breaker
    circuitBreakerState = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "netguard_circuit_breaker_state",
            Help: "Circuit breaker state (0=closed, 1=open, 2=half-open)",
        },
        []string{"component"},
    )
    
    // Cache
    cacheHitsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "netguard_cache_hits_total",
            Help: "Total number of cache hits",
        },
        []string{"resource"},
    )
    
    cacheMissesTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "netguard_cache_misses_total",
            Help: "Total number of cache misses",
        },
        []string{"resource"},
    )
)
```

### 2.3 Middleware для метрик
- [ ] **Задача:** Автоматический сбор метрик для всех HTTP запросов
- [ ] **Зачем:** Без изменения существующего кода

**Файл:** `internal/k8s/middleware/metrics.go`

```go
func MetricsMiddleware() func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            start := time.Now()
            
            // Обернуть ResponseWriter для захвата status code
            wrapped := &responseWriter{ResponseWriter: w, statusCode: 200}
            
            next.ServeHTTP(wrapped, r)
            
            duration := time.Since(start).Seconds()
            resource := extractResourceFromPath(r.URL.Path)
            
            httpRequestsTotal.WithLabelValues(
                r.Method, 
                strconv.Itoa(wrapped.statusCode),
                resource,
            ).Inc()
            
            httpRequestDuration.WithLabelValues(
                r.Method,
                resource,
            ).Observe(duration)
        })
    }
}
```

---

## Этап 3: Structured Logging

### 3.1 Логирование с trace correlation
- [ ] **Задача:** Связать логи с trace ID
- [ ] **Зачем:** Корреляция между логами и traces

**Файл:** `internal/k8s/logging/logger.go`

```go
func LoggerWithTrace(ctx context.Context) logr.Logger {
    logger := klog.FromContext(ctx)
    
    // Добавить trace ID в логи
    if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
        traceID := span.SpanContext().TraceID().String()
        logger = logger.WithValues("trace_id", traceID)
    }
    
    return logger
}
```

### 3.2 Структурированные логи для аудита
- [ ] **Задача:** Логирование всех изменений ресурсов
- [ ] **Зачем:** Аудит и troubleshooting

```go
func LogResourceChange(ctx context.Context, operation string, obj runtime.Object) {
    logger := LoggerWithTrace(ctx)
    
    accessor, _ := meta.Accessor(obj)
    logger.Info("Resource operation",
        "operation", operation,
        "kind", obj.GetObjectKind().GroupVersionKind().Kind,
        "name", accessor.GetName(),
        "namespace", accessor.GetNamespace(),
        "resourceVersion", accessor.GetResourceVersion(),
        "generation", accessor.GetGeneration(),
    )
}
```

---

## Этап 4: Dashboards и Alerting

### 4.1 Grafana Dashboard
- [ ] **Задача:** Создать dashboard для мониторинга API server
- [ ] **Зачем:** Визуализация метрик и быстрое обнаружение проблем

**Файл:** `config/monitoring/grafana-dashboard.json`

**Панели:**
- Request Rate (RPS)
- Request Duration (P50, P95, P99)
- Error Rate (5xx responses)
- Backend Health
- Circuit Breaker Status
- Cache Hit Rate
- Resource Counts

### 4.2 Prometheus Alerts
- [ ] **Задача:** Настроить алерты для критических ситуаций
- [ ] **Зачем:** Проактивное обнаружение проблем

**Файл:** `config/monitoring/alerts.yaml`

```yaml
groups:
- name: netguard-apiserver
  rules:
  - alert: NetguardAPIHighErrorRate
    expr: rate(netguard_api_http_requests_total{code=~"5.."}[5m]) > 0.1
    for: 2m
    labels:
      severity: critical
    annotations:
      summary: "High error rate in Netguard API"
      
  - alert: NetguardBackendDown
    expr: up{job="netguard-backend"} == 0
    for: 1m
    labels:
      severity: critical
    annotations:
      summary: "Netguard backend is down"
      
  - alert: NetguardAPIHighLatency
    expr: histogram_quantile(0.95, rate(netguard_api_http_request_duration_seconds_bucket[5m])) > 1
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "High latency in Netguard API"
```

---

## Этап 5: Integration и Testing

### 5.1 Тестирование метрик
- [ ] **Задача:** Unit тесты для сбора метрик
- [ ] **Зачем:** Гарантия корректности метрик

**Файл:** `internal/k8s/metrics/metrics_test.go`

### 5.2 E2E тестирование observability
- [ ] **Задача:** Проверка работы tracing и metrics в E2E тестах
- [ ] **Зачем:** Интеграционное тестирование observability stack

### 5.3 Performance testing
- [ ] **Задача:** Нагрузочное тестирование с включенной observability
- [ ] **Зачем:** Оценка overhead от observability

---

## Deployment Configuration

### ServiceMonitor для Prometheus
```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: netguard-apiserver
spec:
  selector:
    matchLabels:
      app: netguard-apiserver
  endpoints:
  - port: metrics
    path: /metrics
```

### Jaeger Configuration
```yaml
apiVersion: jaegertracing.io/v1
kind: Jaeger
metadata:
  name: netguard-jaeger
spec:
  strategy: production
  storage:
    type: elasticsearch
```

---

## Критерии готовности

✅ **Observability готова, когда:**
- [ ] Все HTTP запросы трассируются
- [ ] Backend вызовы трассируются  
- [ ] Метрики собираются и экспортируются
- [ ] Dashboard показывает актуальные данные
- [ ] Алерты настроены и тестированы
- [ ] Логи содержат trace ID
- [ ] Performance overhead < 5%

---

## Примерное время выполнения

| Этап | Время (часы) | Сложность |
|------|-------------|-----------|
| 1: OpenTelemetry | 8-12 | Высокая |
| 2: Prometheus Metrics | 6-8 | Средняя |
| 3: Structured Logging | 4-6 | Средняя |
| 4: Dashboards + Alerts | 6-8 | Средняя |
| 5: Testing | 4-6 | Средняя |

**Общее время:** 28-40 часов (4-5 рабочих дней) 