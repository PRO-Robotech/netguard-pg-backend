# Исправления Kubernetes Aggregation Layer

## Проблема

Ошибка `no kind "ServiceList" is registered for version "netguard.sgroups.io/v1beta1" in scheme` возникала из-за неправильной настройки aggregation layer.

### Что было не так:

1. **Неправильная архитектура**: использовался `SimpleAPIServer` вместо полноценного `genericapiserver.GenericAPIServer`
2. **Проблема с main.go**: вызывалась несуществующая функция `apiserver.NewOptions()`
3. **Схема не настроена**: типы не были зарегистрированы в правильной схеме
4. **Watch не работал**: polling механизм не был интегрирован с Kubernetes API machinery

## Решение

### 1. Исправили main.go

```go
// Было:
options := apiserver.NewOptions(os.Stdout, os.Stderr)

// Стало:
options := apiserver.NewWardleServerOptions(os.Stdout, os.Stderr)
```

### 2. Переписали options.go

- Добавили правильную структуру `WardleServerOptions`
- Исправили создание `BackendClient` с конфигурацией
- Добавили поддержку OpenAPI спецификаций
- Настроили правильную схему

### 3. Обновили server.go

- Используем `clientscheme.Scheme` с зарегистрированными типами
- Правильная настройка `genericapiserver.GenericAPIServer`
- Полная интеграция с Kubernetes API machinery

### 4. Исправили watch механизм

- Добавили недостающую функцию `NewPollerWatchInterface`
- Интегрировали с стандартными механизмами Kubernetes

## Результат

✅ **Все ресурсы теперь поддерживают полный набор Kubernetes API операций:**

### CRUD Operations
- **GET** - получение конкретного объекта
- **LIST** - получение списка объектов  
- **CREATE** - создание нового объекта
- **UPDATE** - полное обновление объекта
- **PATCH** - частичное обновление объекта
- **DELETE** - удаление объекта

### Advanced Features
- **WATCH** - отслеживание изменений в реальном времени
- **OpenAPI** - автогенерируемая документация API
- **Discovery API** - автоматическое обнаружение ресурсов
- **kubectl поддержка** - полная совместимость с kubectl

### Поддерживаемые ресурсы

1. **services** - сетевые сервисы
2. **addressgroups** - группы адресов
3. **addressgroupbindings** - привязки групп адресов
4. **addressgroupportmappings** - маппинги портов
5. **rules2s** - правила service-to-service
6. **servicealiases** - алиасы сервисов
7. **addressgroupbindingpolicies** - политики привязок
8. **ieagagrules** - правила ingress/egress

## Тестирование

```bash
# Компиляция
go build ./cmd/k8s-apiserver

# Запуск API сервера
./k8s-apiserver --backend-address=localhost:9090

# Тестирование функциональности
chmod +x test-api.sh
./test-api.sh
```

## Использование с kubectl

После запуска API сервера все ресурсы доступны через kubectl:

```bash
# Получение всех сервисов
kubectl get services.netguard.sgroups.io

# Создание сервиса
kubectl apply -f - <<EOF
apiVersion: netguard.sgroups.io/v1beta1
kind: Service
metadata:
  name: my-service
  namespace: default
spec:
  description: "My test service"
  ingressPorts:
  - protocol: TCP
    port: "80"
    description: "HTTP port"
EOF

# Watch изменений
kubectl get services.netguard.sgroups.io --watch

# Получение описания ресурса
kubectl describe service.netguard.sgroups.io/my-service
```

## Архитектурные преимущества

1. **Полная совместимость с Kubernetes** - все стандартные операции работают из коробки
2. **Автоматическая генерация клиентов** - informers, listers, clientsets
3. **Стандартные механизмы Kubernetes** - watch, events, conditions
4. **OpenAPI интеграция** - автодокументирование и валидация
5. **Масштабируемость** - все преимущества Kubernetes API machinery

Теперь ваш aggregation layer работает как полноценный Kubernetes API сервер! 🎉 