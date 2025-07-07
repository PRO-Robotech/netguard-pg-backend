# Netguard Platform Automation Scripts

Набор скриптов для автоматизации развертывания, тестирования и управления Netguard платформой в Kubernetes.

## 📋 Обзор скриптов

| Скрипт | Назначение | Использование |
|--------|------------|---------------|
| `analyze-current-state.sh` | Анализ текущего состояния | `./analyze-current-state.sh` |
| `fix-namespaces.sh` | Исправление namespace'ов | `./fix-namespaces.sh` |
| `deploy-complete.sh` | Полное развертывание | `./deploy-complete.sh` |
| `test-complete.sh` | Комплексное тестирование | `./test-complete.sh` |
| `compare-implementations.sh` | Сравнение CRD vs Aggregation | `./compare-implementations.sh` |
| `clean-redeploy.sh` | Полная переустановка | `./clean-redeploy.sh` |
| `cleanup-old-deployment.sh` | Очистка старого развертывания | `./cleanup-old-deployment.sh` |
| `generate-certs.sh` | Генерация TLS сертификатов | `./generate-certs.sh` |
| `dev-k8s.sh` | Разработческие утилиты | `./dev-k8s.sh [command]` |

## 🚀 Быстрый старт

### 1. Анализ текущего состояния
```bash
cd netguard-pg-backend/scripts
./analyze-current-state.sh
```

### 2. Исправление конфигурации (при необходимости)
```bash
./fix-namespaces.sh
```

### 3. Полное развертывание
```bash
./deploy-complete.sh
```

### 4. Сравнение реализаций (если есть обе)
```bash
./compare-implementations.sh
```

### 5. Тестирование
```bash
./test-complete.sh
```

## 🔄 Полная переустановка (при проблемах с namespace'ами)

Если у вас есть старое развертывание в `default` namespace:

```bash
# Автоматическая полная переустановка
./clean-redeploy.sh
```

Этот скрипт автоматически:
1. 🧹 Очистит старое развертывание из default namespace
2. 🔧 Исправит namespace'ы в конфигурации 
3. 🔐 Перегенерирует TLS сертификаты
4. 🚀 Развернет в netguard-system namespace
5. 🧪 Протестирует v1beta1 (Aggregation Layer)

## 📖 Детальное описание скриптов

### `analyze-current-state.sh`

**Назначение:** Комплексный анализ текущего состояния Netguard в Kubernetes кластере.

**Возможности:**
- ✅ Проверка подключения к minikube
- 📁 Анализ namespace'ов и их содержимого
- 🚀 Проверка deployments и подов
- 🔌 Анализ сервисов и их доступности
- 🎯 Проверка API ресурсов и CRD
- 🔍 Выявление проблем конфигурации
- 📈 Анализ использования ресурсов
- 💡 Генерация рекомендаций

**Команды:**
```bash
./analyze-current-state.sh                # Полный анализ
./analyze-current-state.sh namespaces     # Только namespace'ы
./analyze-current-state.sh workloads      # Только поды и deployments
./analyze-current-state.sh api            # Только API ресурсы
./analyze-current-state.sh recommendations # Только рекомендации
```

### `fix-namespaces.sh`

**Назначение:** Исправление несоответствий namespace'ов в конфигурации.

**Что исправляет:**
- ✅ Заменяет `namespace: default` на `namespace: netguard-system`
- 🔧 Обновляет BACKEND_ENDPOINT с полным FQDN
- 🗑️ Удаляет дублирующиеся определения сервисов
- 📋 Обновляет kustomization.yaml
- 💾 Создает резервную копию перед изменениями

**Пример:**
```bash
./fix-namespaces.sh

# Результат:
# ✅ configmap.yaml - исправлен (1 namespace'ов netguard-system)
# ✅ backend-deployment.yaml - исправлен (2 namespace'ов netguard-system)
# ✅ deployment.yaml - исправлен (1 namespace'ов netguard-system)
# 🎉 Все namespace'ы успешно исправлены!
```

### `deploy-complete.sh`

**Назначение:** Полное развертывание Netguard платформы в Kubernetes.

**Этапы развертывания:**
1. 🔍 Проверка предварительных требований
2. 🧹 Очистка предыдущих развертываний
3. 📁 Создание namespace с правильными метками
4. 🔧 Генерация Kubernetes кода
5. 🏗️ Сборка Docker образов
6. 📦 Загрузка образов в minikube (если обнаружен)
7. 🚀 Развертывание в Kubernetes
8. ⏳ Ожидание готовности всех компонентов
9. 🧪 Создание и проверка тестового ресурса

**Команды:**
```bash
./deploy-complete.sh         # Полное развертывание
./deploy-complete.sh cleanup # Только очистка
./deploy-complete.sh status  # Показать статус
```

**Особенности:**
- 🎯 Автоматическое обнаружение minikube
- 📊 Подробный статус развертывания
- ⚡ Проверка APIService доступности
- 🔄 Таймауты и retry логика

### `test-complete.sh`

**Назначение:** Комплексное тестирование всех аспектов Netguard платформы.

**Тесты:**
1. **Инфраструктурные тесты:**
   - ✅ Namespace существует
   - 🚀 Развертывания готовы
   - 🟢 Поды запущены
   - 🔌 Сервисы доступны

2. **API тесты:**
   - 🔗 APIService зарегистрирован
   - 🎯 API ресурсы обнаруживаются
   - 📝 CRUD операции работают

3. **Тесты подключения:**
   - 🔄 Backend доступен
   - ❤️ Health endpoints работают

4. **Тесты качества:**
   - 📊 Логи без критических ошибок

**Команды:**
```bash
./test-complete.sh              # Все тесты
./test-complete.sh quick        # Быстрая проверка
./test-complete.sh performance  # Нагрузочное тестирование
./test-complete.sh status       # Детальный статус
./test-complete.sh logs         # Показать логи
```

**Пример вывода:**
```
🧪 Комплексное тестирование Netguard Platform
=============================================

🧪 Тест: Namespace существует
✅ PASSED: Namespace существует

🧪 Тест: CRUD операции работают
  Создание тестового ресурса: test-crud-service-1699123456
  Чтение созданного ресурса
  Обновление ресурса
  Удаление тестового ресурса
  ✓ Все CRUD операции выполнены успешно
✅ PASSED: CRUD операции работают

📊 Результаты тестирования:
==========================
Общее количество тестов: 10
Прошло успешно: 10
Провалилось: 0
🎉 Все тесты прошли успешно!
```

### `compare-implementations.sh`

**Назначение:** Сравнение CRD (v1alpha1) и Aggregation Layer (v1beta1) реализаций.

**Важно:** Netguard имеет две реализации:
- **v1alpha1** - традиционная реализация через CustomResourceDefinitions (CRD)
- **v1beta1** - современная реализация через Kubernetes Aggregation Layer

**Возможности:**
- 🔍 Обнаружение обеих реализаций
- 📊 Сравнение API ресурсов по версиям
- 🧪 Тестирование CRUD операций для каждой версии
- ⚡ Сравнение производительности
- 📦 Анализ существующих ресурсов
- 💡 Рекомендации по выбору реализации

**Команды:**
```bash
./compare-implementations.sh                # Полное сравнение
./compare-implementations.sh check          # Проверка доступности
./compare-implementations.sh crud           # Тестирование CRUD
./compare-implementations.sh performance    # Сравнение производительности
./compare-implementations.sh recommendations # Рекомендации
```

**Пример вывода:**
```
🔍 Сравнение реализаций Netguard
================================
v1alpha1 (CRD) vs v1beta1 (Aggregation Layer)

🔸 Aggregation Layer (v1beta1):
✅ APIService v1beta1.netguard.sgroups.io доступен

🔸 CRD реализация (v1alpha1):
✅ Найдено 8 netguard CRD

📊 Статистика по версиям:
  - v1alpha1 (CRD): 8 ресурсов
  - v1beta1 (Aggregation): 8 ресурсов

💡 Рекомендации:
⚠ Обнаружены ОБЕ реализации одновременно
1. 🔧 Рекомендуется использовать только одну реализацию
2. 🎯 Для тестирования Aggregation Layer фокусируйтесь на v1beta1
```

### `dev-k8s.sh`

**Назначение:** Разработческие утилиты (существующий скрипт).

**Основные команды:**
```bash
./dev-k8s.sh dev       # Полный цикл разработки
./dev-k8s.sh test      # Тестирование API
./dev-k8s.sh logs      # Просмотр логов
./dev-k8s.sh forward   # Port forwarding
```

## 🔄 Рекомендуемые workflow'ы

### Первичное развертывание
```bash
# 1. Анализ состояния
./analyze-current-state.sh

# 2. Исправление конфигурации (если нужно)
./fix-namespaces.sh

# 3. Развертывание
./deploy-complete.sh

# 4. Тестирование
./test-complete.sh
```

### Обновление существующего развертывания
```bash
# 1. Анализ текущего состояния
./analyze-current-state.sh

# 2. Очистка и переразвертывание
./deploy-complete.sh cleanup
./deploy-complete.sh

# 3. Проверка
./test-complete.sh quick
```

### Отладка проблем
```bash
# 1. Анализ проблем
./analyze-current-state.sh config

# 2. Просмотр логов
./test-complete.sh logs

# 3. Статус системы
./deploy-complete.sh status

# 4. Ручная диагностика
kubectl get all -n netguard-system
kubectl describe apiservice v1beta1.netguard.sgroups.io
```

### Разработческий цикл
```bash
# 1. Быстрая проверка
./test-complete.sh quick

# 2. Просмотр логов
./dev-k8s.sh logs

# 3. Port forwarding для локального доступа
./dev-k8s.sh forward
```

## 🛡️ Best Practices

### Namespace'ы
- ✅ Все netguard компоненты в `netguard-system`
- ❌ Избегайте `default` namespace
- 🏷️ Используйте правильные метки и аннотации

### Безопасность
- 🔒 Pod Security Standards: `restricted`
- 🛡️ Security contexts для всех контейнеров
- 🔐 TLS для всех внешних соединений

### Мониторинг
- 📊 Health и readiness probes
- 📝 Структурированное логирование
- 🔍 Tracing через OpenTelemetry

### Ресурсы
- ⚡ Resource requests и limits
- 📈 Horizontal Pod Autoscaler при необходимости
- 💾 Persistent volumes для stateful компонентов

## ❗ Устранение неисправностей

### APIService недоступен
```bash
# Проверить статус
kubectl describe apiservice v1beta1.netguard.sgroups.io

# Проверить поды
kubectl get pods -n netguard-system

# Логи API сервера
kubectl logs -n netguard-system deployment/netguard-apiserver
```

### Backend недоступен
```bash
# Проверить connectivity
./test-complete.sh quick

# Логи backend
kubectl logs -n netguard-system deployment/netguard-backend

# Port forwarding для отладки
kubectl port-forward -n netguard-system service/netguard-backend 8080:8080
```

### Проблемы с образами
```bash
# Для minikube - загрузить образы заново
minikube image load netguard/k8s-apiserver:latest
minikube image load netguard/pg-backend:latest

# Пересобрать образы
make docker-build-k8s-apiserver
```

## 📚 Дополнительные ресурсы

- **Конфигурация:** `../config/k8s/README.md`
- **Makefile targets:** `../Makefile`
- **API документация:** `../swagger-ui/`
- **Примеры ресурсов:** `../test/k8s/`

## 🤝 Поддержка

При возникновении проблем:

1. Запустите полный анализ: `./analyze-current-state.sh`
2. Проверьте рекомендации: `./analyze-current-state.sh recommendations`
3. Соберите диагностическую информацию:
   ```bash
   kubectl get all -A | grep netguard > debug-info.txt
   kubectl describe apiservice v1beta1.netguard.sgroups.io >> debug-info.txt
   ./test-complete.sh logs >> debug-info.txt
   ``` 