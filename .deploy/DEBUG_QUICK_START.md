# 🐛 Debug Mode - Quick Start

## ✅ Полностью автоматизированный debug режим

Теперь **ВСЁ работает автоматически** - никаких ручных команд!

## 🚀 Запуск debug режима

```bash
cd .deploy/skaffold
./run-debug-quiet.sh debug-all
```

**Что происходит автоматически:**

1. ✅ Skaffold билдит и деплоит все 3 сервиса в debug режиме:
   - netguard-backend-debug (с Delve на :2345)
   - netguard-apiserver-debug (с Delve на :2346)
   - netguard-webhook-debug (с Delve на :2347)

2. ✅ Port-forwards настраиваются автоматически:
   - Backend Delve: `localhost:2345`
   - Backend gRPC: `localhost:9090`
   - Backend HTTP: `localhost:8080`
   - APIServer Delve: `localhost:2346`
   - Webhook Delve: `localhost:2347`
   - PostgreSQL: `localhost:5432`

3. ✅ Production services переключаются на debug поды:
   - `netguard-backend` → debug pod
   - `netguard-apiserver` → debug pod
   - `netguard-webhook` → debug pod

4. ✅ Весь request flow работает в debug режиме:
   ```
   UI → APIServer (debug) → Backend (debug)
   UI → Webhook (debug) → Backend (debug)
   ```

## ⚙️ Поведение при отключении debugger

**Delve настроен с флагом `--continue`:**
- ✅ При отключении debugger процесс **продолжает работать** нормально
- ✅ Под **не перезапускается**
- ✅ Можно переподключиться в любой момент
- ⚠️ Если отключились на breakpoint - он будет пропущен

**Это значит:**
- Можно свободно подключать/отключать debugger без последствий
- Сервисы продолжают обрабатывать запросы даже без debugger

## 🔌 Подключение debugger

### ⚠️ КРИТИЧЕСКИ ВАЖНАЯ настройка IDE

**БЕЗ ЭТОЙ НАСТРОЙКИ ПОД БУДЕТ ПЕРЕЗАПУСКАТЬСЯ ПРИ ОТКЛЮЧЕНИИ DEBUGGER!**

**Проблема:**
При отключении debugger IDE по умолчанию отправляет команду `halt` в Delve, что завершает процесс и приводит к перезапуску пода Kubernetes.

**Решение для GoLand/IntelliJ:**
В настройках debug конфигурации **ОБЯЗАТЕЛЬНО** установить:
- **On disconnect** → **`Leave it running`** ✅

**Не используйте:**
- ❌ "Stop remote Delve process" - это убьет процесс и под перезапустится

**Почему это происходит:**
1. GoLand отправляет команду `halt` при отключении
2. Delve останавливает процесс приложения
3. Контейнер завершается (exit code 0)
4. Kubernetes видит что контейнер упал → перезапускает под

С правильной настройкой:
- ✅ Debugger просто отключается
- ✅ Процесс продолжает работать
- ✅ Под НЕ перезапускается
- ✅ Можно переподключиться в любой момент

---

### VS Code

`.vscode/launch.json`:

```json
{
    "version": "0.2.0",
    "configurations": [
        {
            "name": "Backend (2345)",
            "type": "go",
            "request": "attach",
            "mode": "remote",
            "remotePath": "/src",
            "port": 2345,
            "host": "localhost"
        },
        {
            "name": "APIServer (2346)",
            "type": "go",
            "request": "attach",
            "mode": "remote",
            "remotePath": "/app",
            "port": 2346,
            "host": "localhost"
        },
        {
            "name": "Webhook (2347)",
            "type": "go",
            "request": "attach",
            "mode": "remote",
            "remotePath": "/app",
            "port": 2347,
            "host": "localhost"
        }
    ]
}
```

### GoLand / IntelliJ IDEA

**Пошаговая настройка:**

1. **Run → Edit Configurations** (или Alt+Shift+F10 → Edit Configurations)
2. **Add New Configuration (+)** → **Go Remote**
3. **Заполните поля:**
   - **Name**: `Netguard Backend Debug` (или любое другое имя)
   - **Host**: `localhost`
   - **Port**:
     - `2345` для Backend
     - `2346` для APIServer
     - `2347` для Webhook
   - ⚠️ **On disconnect**: **`Leave it running`** ← **КРИТИЧЕСКИ ВАЖНО!**

4. **Apply → OK**

**Скриншот где найти настройку:**
```
┌─────────────────────────────────────┐
│ Run/Debug Configurations            │
├─────────────────────────────────────┤
│ Name: [Netguard Backend Debug    ] │
│ Host: [localhost                 ] │
│ Port: [2345                      ] │
│                                     │
│ ┌─ On disconnect: ────────────────┐│
│ │ ○ Stop remote Delve process     ││  ← НЕ ВЫБИРАЙТЕ ЭТО!
│ │ ● Leave it running         ✅   ││  ← ВЫБЕРИТЕ ЭТО!
│ └─────────────────────────────────┘│
│                                     │
│           [Apply]  [OK]  [Cancel]   │
└─────────────────────────────────────┘
```

**Для отладки разных сервисов создайте 3 конфигурации:**
- `Backend Debug` (port 2345)
- `APIServer Debug` (port 2346)
- `Webhook Debug` (port 2347)

## 🧪 Тестирование

### Проверить что endpoints указывают на debug поды

```bash
kubectl get endpoints netguard-backend netguard-apiserver netguard-webhook -n incloud-sgroups
kubectl get pods -n incloud-sgroups -l app.kubernetes.io/instance=netguard-debug -o wide
```

IP в endpoints должны совпадать с IP debug pods.

### Проверить port-forwards

```bash
lsof -i:2345 -i:2346 -i:2347 -i:8080 -i:9090 | grep LISTEN
```

Должны быть активны все 5 портов.

### Отправить тестовый запрос

```bash
# gRPC
grpcurl -plaintext localhost:9090 list

# HTTP
curl http://localhost:8080/health
```

## 📝 Просмотр логов

```bash
# Backend
kubectl logs -f deployment/netguard-backend-debug -n incloud-sgroups

# APIServer
kubectl logs -f deployment/netguard-apiserver-debug -n incloud-sgroups

# Webhook
kubectl logs -f deployment/netguard-webhook-debug -n incloud-sgroups
```

## 🛑 Остановка debug режима

Просто нажмите `Ctrl+C` в терминале где запущен Skaffold.

**Что происходит автоматически при остановке:**

1. ✅ Services переключаются обратно на production поды
2. ✅ Debug deployments удаляются
3. ✅ Port-forwards закрываются

## 🔄 Полный workflow

1. **Запустить debug**: `./run-debug-quiet.sh debug-all`
2. **Подождать ~1 минуту** пока поды станут Ready
3. **Подключить debugger** к нужному сервису
4. **Поставить breakpoint** в коде
5. **Отправить запрос** через UI или curl/grpcurl
6. **Debugger остановится** на breakpoint
7. **После отладки**: `Ctrl+C` для выхода

## 🆚 Сравнение с hot reload

| Функция | debug-all | debug-hot-reload |
|---------|-----------|------------------|
| Все 3 сервиса | ✅ | ❌ (только backend) |
| Delve debugger | ✅ | ✅ |
| Hot reload | ❌ | ✅ (нестабильный) |
| Auto port-forward | ✅ | ✅ |
| Auto service switch | ✅ | ❌ |
| Время сборки | 30-60 сек | 30-60 сек первый раз |
| Rebuild при изменениях | Полный | 2-5 сек (если работает) |

**Рекомендация:** Используйте `debug-all` для полноценной отладки всего request flow.

## 💡 Полезные команды

```bash
# Статус debug pods
kubectl get pods -n incloud-sgroups -l app.kubernetes.io/instance=netguard-debug

# Проверить какие services используют debug
kubectl get svc -n incloud-sgroups -o json | jq -r '.items[] | select(.spec.selector."app.kubernetes.io/instance" == "netguard-debug") | .metadata.name'

# Логи всех debug pods одновременно
kubectl logs -f -l app.kubernetes.io/instance=netguard-debug -n incloud-sgroups --all-containers=true --prefix=true
```

## 🚨 Troubleshooting

### Под перезапускается при отключении debugger

**Симптомы:**
- Отключил debugger в IDE
- Под сразу перезапустился (restart count увеличился)
- В логах видно: `debugger halting`, `debugger detaching`

**Причина:**
IDE отправляет команду `halt` при отключении, что завершает процесс Delve.

**Решение:**
Проверьте настройку IDE:

**GoLand/IntelliJ:**
```
Run → Edit Configurations → [Ваша конфигурация]
→ On disconnect: "Leave it running" ✅ (НЕ "Stop remote Delve process"!)
```

**Как проверить что это именно эта проблема:**
```bash
# Посмотреть логи ПЕРЕД перезапуском
kubectl logs -n incloud-sgroups deployment/netguard-backend-debug --previous --tail=20

# Если видите:
# "← Command("halt")"
# "debugger halting"
# "debugger detaching"
# → Это точно настройка IDE!
```

**После исправления:**
- Под больше не будет перезапускаться
- Можно свободно подключать/отключать debugger
- Процесс продолжает работать нормально

### Port-forward не работает

**Проверка:**
```bash
lsof -i:2345 -i:2346 -i:2347 | grep LISTEN
```

Должны быть процессы `kubectl`.

**Решение:**
Port-forwards должны настроиться автоматически. Если нет - перезапустите debug режим.

### Services не указывают на debug поды

**Проверка:**
```bash
kubectl get svc netguard-backend netguard-apiserver netguard-webhook -n incloud-sgroups \
  -o custom-columns='NAME:.metadata.name,INSTANCE:.spec.selector.app\.kubernetes\.io/instance'
```

Должно быть `instance: netguard-debug` для всех сервисов.

**Решение:**
```bash
cd .deploy
./switch-to-debug.sh
```
