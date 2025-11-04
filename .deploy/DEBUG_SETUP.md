# Подключение Debugger к Hot Reload Pod

## ✅ Что работает

1. **Debug pod запущен** с Delve на порту 2345
2. **Port-forward активен**: `localhost:2345 → pod:2345`
3. **File sync работает** (иногда нужен manual sync)
4. **Production service перенаправлен** на debug pod

## 🔌 Подключение Debugger

### GoLand / IntelliJ IDEA

1. **Run → Edit Configurations**
2. **Add New Configuration → Go Remote**
3. **Настройки:**
   - **Name**: `Netguard Debug`
   - **Host**: `localhost`
   - **Port**: `2345`
   - **On disconnect**: `Leave it running`

4. **Apply → OK**
5. **Run → Debug 'Netguard Debug'** (или нажмите Debug icon)

### VS Code

Создайте `.vscode/launch.json`:

```json
{
    "version": "0.2.0",
    "configurations": [
        {
            "name": "Connect to Netguard Pod",
            "type": "go",
            "request": "attach",
            "mode": "remote",
            "remotePath": "/src",
            "port": 2345,
            "host": "localhost",
            "showLog": true,
            "trace": "verbose"
        }
    ]
}
```

Затем: **Run → Start Debugging** (F5)

## 🧪 Проверка подключения

### 1. Проверить что Delve слушает

```bash
# Локально
lsof -i:2345

# Должно показать:
# kubectl ... TCP localhost:websm (LISTEN)
```

### 2. Проверить в pod

```bash
POD=$(kubectl get pod -n incloud-sgroups -l app.kubernetes.io/component=debug-hot-reload -o jsonpath='{.items[0].metadata.name}')

kubectl exec -n incloud-sgroups $POD -- netstat -tlnp | grep 2345

# Должно показать:
# tcp   0   0 :::2345   :::*   LISTEN   XXX/dlv
```

### 3. Тестовый запрос

```bash
# Если у вас есть gRPC клиент:
grpcurl -plaintext localhost:9090 list

# Или HTTP:
curl http://localhost:8080/your-endpoint
```

## 🐛 Установка Breakpoints

1. **Откройте файл** в IDE (например, `internal/api/netguard/service.go`)
2. **Кликните слева от номера строки** чтобы поставить breakpoint
3. **Отправьте запрос** на API
4. **Debugger остановится** на breakpoint

### Пример: Debug ListServices

```go
// internal/api/netguard/service.go
func (s *ServiceServer) ListServices(ctx context.Context, req *netguardpb.ListServicesReq) (*netguardpb.ListServicesResp, error) {
    // <- ПОСТАВЬТЕ BREAKPOINT ЗДЕСЬ
    return s.serviceHandler.ListServices(ctx, req)
}
```

Отправьте gRPC запрос:
```bash
grpcurl -plaintext localhost:9090 netguard.v1beta1.NetguardService/ListServices
```

Debugger должен остановиться!

## 🔄 Hot Reload Workflow

### С автоматическим file sync (если работает)

1. **Измените код**
2. **Сохраните** (Cmd+S)
3. **Ждите 2-5 секунд** - Air пересоберёт
4. **Debugger переподключится** автоматически
5. **Breakpoints работают** с новым кодом

### С ручным sync (если auto не работает)

1. **Измените код**
2. **Сохраните** (Cmd+S)
3. **Синхронизируйте**:
   ```bash
   cd .deploy/skaffold
   ./sync-file.sh internal/api/netguard/service.go
   ```
4. **Ждите 2-5 секунд** - Air пересоберёт
5. **Debugger остаётся подключенным!**
6. **Breakpoints работают** с новым кодом

## 📊 Проверка что используется Debug Pod

```bash
# Посмотреть куда указывает production service
kubectl get endpoints netguard-backend -n incloud-sgroups

# Посмотреть IP debug pod
kubectl get pod -n incloud-sgroups -l app.kubernetes.io/component=debug-hot-reload -o wide

# IP должны совпадать!
```

## 🚨 Troubleshooting

### Debugger не подключается

1. **Проверить port-forward**:
   ```bash
   lsof -i:2345
   ```

2. **Перезапустить port-forward**:
   ```bash
   # Убить старые
   pkill -f "port-forward.*2345"

   # Запустить новый
   kubectl port-forward -n incloud-sgroups deployment/netguard-backend-debug 2345:2345
   ```

3. **Проверить логи pod**:
   ```bash
   kubectl logs -f deployment/netguard-backend-debug -n incloud-sgroups | grep -E "dlv|Delve|2345"
   ```

### Breakpoints не срабатывают

1. **Проверить что код синхронизирован**:
   ```bash
   POD=$(kubectl get pod -n incloud-sgroups -l app.kubernetes.io/component=debug-hot-reload -o jsonpath='{.items[0].metadata.name}')

   # Проверить содержимое файла в pod
   kubectl exec -n incloud-sgroups $POD -- cat /src/internal/api/netguard/service.go | head -20
   ```

2. **Пересобрать вручную** (триггер Air):
   ```bash
   ./sync-file.sh internal/api/netguard/service.go
   ```

3. **Переподключить debugger**:
   - Отключить (Stop)
   - Подключить снова (Debug)

### Запросы не идут на debug pod

```bash
# Переключить services на debug
cd /Users/zhd/Projects/newPro/netguard-pg-backend/.deploy
./switch-to-debug.sh
```

## 💡 Полезные команды

```bash
# Логи Air + Delve
kubectl logs -f deployment/netguard-backend-debug -n incloud-sgroups

# Проверить все port-forwards
lsof -i:2345 -i:8080 -i:9090 -i:5432

# Перезапустить hot reload
cd .deploy/skaffold
# Ctrl+C
./run-hot-reload.sh

# Синхронизировать изменённые файлы
find ../.. -name "*.go" -mmin -5 -not -path "*/vendor/*" | while read f; do
    ./sync-file.sh "${f#../../}"
done
```
