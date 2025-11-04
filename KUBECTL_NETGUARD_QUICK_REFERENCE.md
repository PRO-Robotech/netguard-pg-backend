# Kubectl Commands для Netguard ресурсов - Быстрая справка

## ✅ Статус

**APIService v1beta1 настроен как приоритетный!**

- API Group: `netguard.sgroups.io`
- Версии: `v1alpha1` (CRD), `v1beta1` (Aggregated API) ← **приоритетная**
- Namespace: `incloud-sgroups`
- Приоритеты: v1beta1 (2000/200) > v1alpha1 (1000/100)

---

## Основные команды

### Просмотр всех доступных ресурсов

```bash
# Показать все netguard API ресурсы с версиями
kubectl api-resources --api-group=netguard.sgroups.io

# Показать доступные API версии
kubectl api-versions | grep netguard
```

### Получение всех ресурсов во всех namespace

```bash
# v1beta1 ресурсы (используются по умолчанию)
kubectl get addressgroups.netguard.sgroups.io -A
kubectl get hosts.netguard.sgroups.io -A
kubectl get networks.netguard.sgroups.io -A
kubectl get networkbindings.netguard.sgroups.io -A
kubectl get hostbindings.netguard.sgroups.io -A
kubectl get rules2s.netguard.sgroups.io -A
kubectl get ieagagrules.netguard.sgroups.io -A
kubectl get services.netguard.sgroups.io -A
kubectl get servicealiases.netguard.sgroups.io -A
kubectl get addressgroupbindings.netguard.sgroups.io -A
kubectl get addressgroupportmappings.netguard.sgroups.io -A
kubectl get addressgroupbindingpolicies.netguard.sgroups.io -A
```

### Получение ресурсов в конкретном namespace

```bash
# Пример для namespace incloud-sgroups
kubectl get addressgroups.netguard.sgroups.io -n incloud-sgroups
kubectl get services.netguard.sgroups.io -n incloud-sgroups
kubectl get hosts.netguard.sgroups.io -n incloud-sgroups
```

---

## Примеры CRUD операций

### Создание Service (v1beta1)

```yaml
apiVersion: netguard.sgroups.io/v1beta1
kind: Service
metadata:
  name: my-service
  namespace: incloud-sgroups
spec:
  ingressPorts:
    - protocol: TCP
      port: "8080"
      description: "HTTP port"
```

```bash
kubectl apply -f service.yaml
```

### Создание AddressGroup

```yaml
apiVersion: netguard.sgroups.io/v1beta1
kind: AddressGroup
metadata:
  name: my-ag
  namespace: incloud-sgroups
spec:
  defaultAction: ACCEPT
  logs: false
  trace: false
```

### Создание Host

```yaml
apiVersion: netguard.sgroups.io/v1beta1
kind: Host
metadata:
  name: my-host
  namespace: incloud-sgroups
spec:
  uuid: "550e8400-e29b-41d4-a716-446655440000"
```

### Создание Network

```yaml
apiVersion: netguard.sgroups.io/v1beta1
kind: Network
metadata:
  name: my-network
  namespace: incloud-sgroups
spec:
  cidr: "10.0.0.0/16"
```

### Получение детальной информации

```bash
# YAML формат
kubectl get services.netguard.sgroups.io -n incloud-sgroups my-service -o yaml

# JSON формат
kubectl get addressgroups.netguard.sgroups.io -n incloud-sgroups my-ag -o json

# Описание с событиями
kubectl describe hosts.netguard.sgroups.io -n incloud-sgroups my-host
```

### Редактирование ресурса

```bash
# Открыть в редакторе
kubectl edit services.netguard.sgroups.io -n incloud-sgroups my-service

# Патч через командную строку
kubectl patch services.netguard.sgroups.io -n incloud-sgroups my-service \
  --type merge -p '{"spec":{"description":"Updated description"}}'
```

### Удаление ресурса

```bash
# Удалить конкретный ресурс
kubectl delete services.netguard.sgroups.io -n incloud-sgroups my-service

# Удалить все ресурсы типа в namespace
kubectl delete addressgroups.netguard.sgroups.io -n incloud-sgroups --all
```

---

## Полезные команды для отладки

### Проверка статуса APIService

```bash
# Статус обоих APIService
kubectl get apiservices | grep netguard

# Детальная информация v1beta1
kubectl get apiservice v1beta1.netguard.sgroups.io -o yaml

# Проверить приоритеты
echo "v1beta1:" && kubectl get apiservice v1beta1.netguard.sgroups.io \
  -o jsonpath='{.spec.groupPriorityMinimum}{" "}{.spec.versionPriority}{"\n"}'
echo "v1alpha1:" && kubectl get apiservice v1alpha1.netguard.sgroups.io \
  -o jsonpath='{.spec.groupPriorityMinimum}{" "}{.spec.versionPriority}{"\n"}'
```

**Ожидаемый вывод приоритетов:**
```
v1beta1: 2000 200
v1alpha1: 1000 100
```

### Проверка API server pods и service

```bash
# Проверить pod API сервера
kubectl get pods -n incloud-sgroups | grep apiserver

# Логи API сервера
kubectl logs -n incloud-sgroups -l app.kubernetes.io/name=apiserver --tail=50

# Проверить service
kubectl get svc -n incloud-sgroups netguard-apiserver

# Проверить endpoints
kubectl get endpoints -n incloud-sgroups netguard-apiserver
```

### Watch режим (мониторинг изменений)

```bash
# Следить за изменениями AddressGroup
kubectl get addressgroups.netguard.sgroups.io -n incloud-sgroups -w

# Следить за всеми основными ресурсами
kubectl get addressgroups,hosts,networks,services.netguard.sgroups.io -n incloud-sgroups -w
```

### Custom columns и jsonpath

```bash
# AddressGroup с custom колонками
kubectl get addressgroups.netguard.sgroups.io -n incloud-sgroups \
  -o custom-columns=NAME:.metadata.name,ACTION:.spec.defaultAction,AGE:.metadata.creationTimestamp

# Host с UUID
kubectl get hosts.netguard.sgroups.io -n incloud-sgroups \
  -o custom-columns=NAME:.metadata.name,UUID:.spec.uuid,BOUND:.status.isBound

# Получить только имена Service
kubectl get services.netguard.sgroups.io -n incloud-sgroups -o jsonpath='{.items[*].metadata.name}'
```

---

## Важные замечания

### Использование правильной версии API

После изменения приоритетов **kubectl по умолчанию использует v1beta1** для всех ресурсов.

Если нужно явно обратиться к v1alpha1 (CRD):
```bash
kubectl get services.v1alpha1.netguard.sgroups.io -A
```

Для v1beta1 (обычно не требуется, т.к. это приоритетная версия):
```bash
kubectl get services.v1beta1.netguard.sgroups.io -A
# или просто
kubectl get services.netguard.sgroups.io -A
```

### Проверка используемой версии API

```bash
# При получении ресурса проверить apiVersion
kubectl get services.netguard.sgroups.io -n incloud-sgroups my-service \
  -o jsonpath='{.apiVersion}{"\n"}'
```

**Должно вывести:** `netguard.sgroups.io/v1beta1`

---

## Восстановление приоритетов (если понадобится)

Если нужно вернуть v1beta1 на более высокий приоритет после перезапуска:

```bash
kubectl patch apiservice v1beta1.netguard.sgroups.io --type='json' -p='[
  {"op": "replace", "path": "/spec/groupPriorityMinimum", "value": 2000},
  {"op": "replace", "path": "/spec/versionPriority", "value": 200}
]'
```

Или понизить обратно (если требуется приоритет v1alpha1):

```bash
kubectl patch apiservice v1beta1.netguard.sgroups.io --type='json' -p='[
  {"op": "replace", "path": "/spec/groupPriorityMinimum", "value": 100},
  {"op": "replace", "path": "/spec/versionPriority", "value": 15}
]'
```

---

## Краткая шпаргалка команд

```bash
# Показать все типы ресурсов
kubectl api-resources --api-group=netguard.sgroups.io

# Получить все ресурсы основных типов
kubectl get addressgroups,hosts,networks,services.netguard.sgroups.io,rules2s -A

# Создать из файла
kubectl apply -f resource.yaml

# Редактировать
kubectl edit <resource-type>.netguard.sgroups.io -n <namespace> <name>

# Удалить
kubectl delete <resource-type>.netguard.sgroups.io -n <namespace> <name>

# Watch
kubectl get <resource-type>.netguard.sgroups.io -n <namespace> -w

# Детальная информация
kubectl describe <resource-type>.netguard.sgroups.io -n <namespace> <name>
```

---

## Тестирование

Проверить что всё работает:

```bash
# 1. Проверить приоритеты
kubectl get apiservices | grep netguard

# 2. Проверить что Service использует v1beta1
kubectl get services.netguard.sgroups.io -A

# 3. Проверить apiVersion конкретного ресурса
kubectl get services.netguard.sgroups.io -n incloud-sgroups <service-name> \
  -o jsonpath='{.apiVersion}{"\n"}'
```

**Ожидаемый результат шага 3:** `netguard.sgroups.io/v1beta1`

---

✅ Все команды протестированы и работают в Minikube с Netguard APIServer!

## Дополнительные примеры

### Создание RuleS2S

```yaml
apiVersion: netguard.sgroups.io/v1beta1
kind: RuleS2S
metadata:
  name: my-rule
  namespace: incloud-sgroups
spec:
  traffic: INGRESS  # или EGRESS
  serviceLocalRef:
    apiVersion: netguard.sgroups.io/v1beta1
    kind: Service
    name: local-service
    namespace: incloud-sgroups
  serviceRef:
    apiVersion: netguard.sgroups.io/v1beta1
    kind: Service
    name: target-service
    namespace: incloud-sgroups
  trace: false
```

### Создание NetworkBinding

```yaml
apiVersion: netguard.sgroups.io/v1beta1
kind: NetworkBinding
metadata:
  name: my-binding
  namespace: incloud-sgroups
spec:
  networkRef:
    apiVersion: netguard.sgroups.io/v1beta1
    kind: Network
    name: my-network
  addressGroupRef:
    apiVersion: netguard.sgroups.io/v1beta1
    kind: AddressGroup
    name: my-ag
```

### Создание HostBinding

```yaml
apiVersion: netguard.sgroups.io/v1beta1
kind: HostBinding
metadata:
  name: my-host-binding
  namespace: incloud-sgroups
spec:
  hostRef:
    apiVersion: netguard.sgroups.io/v1beta1
    kind: Host
    name: my-host
    namespace: incloud-sgroups
  addressGroupRef:
    apiVersion: netguard.sgroups.io/v1beta1
    kind: AddressGroup
    name: my-ag
    namespace: incloud-sgroups
```