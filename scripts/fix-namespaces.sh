#!/bin/bash

# Script to fix namespace inconsistencies in netguard configuration
# Приводит все конфигурации к единому namespace: netguard-system

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
CONFIG_DIR="$PROJECT_ROOT/config/k8s"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

echo "=== Исправление namespace'ов в конфигурации Netguard ==="

# Проверяем существование конфигурационной директории
if [ ! -d "$CONFIG_DIR" ]; then
    log_error "Директория конфигурации не найдена: $CONFIG_DIR"
    exit 1
fi

log_info "Директория конфигурации: $CONFIG_DIR"

# Создаем резервную копию
BACKUP_DIR="$CONFIG_DIR.backup.$(date +%Y%m%d_%H%M%S)"
log_info "Создаем резервную копию в: $BACKUP_DIR"
cp -r "$CONFIG_DIR" "$BACKUP_DIR"
log_success "Резервная копия создана"

# Список файлов для обработки
FILES_TO_FIX=(
    "configmap.yaml"
    "backend-deployment.yaml"
    "deployment.yaml"
    "service.yaml"
    "apiservice.yaml"
)

log_info "Исправляем namespace: default -> netguard-system"

for file in "${FILES_TO_FIX[@]}"; do
    FILE_PATH="$CONFIG_DIR/$file"
    if [ -f "$FILE_PATH" ]; then
        log_info "Обрабатываем файл: $file"
        
        # Заменяем namespace: default на namespace: netguard-system
        sed -i.bak 's/namespace: default/namespace: netguard-system/g' "$FILE_PATH"
        
        # Удаляем временный файл .bak
        rm -f "$FILE_PATH.bak"
        
        log_success "✓ $file обновлен"
    else
        log_warning "Файл не найден: $file"
    fi
done

# Исправляем BACKEND_ENDPOINT для правильного FQDN
log_info "Исправляем BACKEND_ENDPOINT для межсервисной коммуникации"

DEPLOYMENT_FILE="$CONFIG_DIR/deployment.yaml"
if [ -f "$DEPLOYMENT_FILE" ]; then
    # Заменяем простое имя сервиса на FQDN
    sed -i.bak 's|BACKEND_ENDPOINT.*netguard-backend:9090|BACKEND_ENDPOINT: "netguard-backend.netguard-system.svc.cluster.local:9090"|g' "$DEPLOYMENT_FILE"
    
    # Также обновляем в случае если указан без кавычек
    sed -i.bak 's|value: netguard-backend:9090|value: "netguard-backend.netguard-system.svc.cluster.local:9090"|g' "$DEPLOYMENT_FILE"
    
    rm -f "$DEPLOYMENT_FILE.bak"
    log_success "✓ BACKEND_ENDPOINT обновлен с FQDN"
fi

# Проверяем дублирующиеся сервисы
log_info "Проверяем дублирующиеся определения сервисов"

SERVICE_FILE="$CONFIG_DIR/service.yaml"
DEPLOYMENT_SERVICE_COUNT=$(grep -c "kind: Service" "$CONFIG_DIR/deployment.yaml" 2>/dev/null || echo "0")

if [ -f "$SERVICE_FILE" ] && [ "$DEPLOYMENT_SERVICE_COUNT" -gt "0" ]; then
    log_warning "Найдены дублирующиеся определения Service"
    log_info "Перемещаем отдельный service.yaml в архив"
    mv "$SERVICE_FILE" "$SERVICE_FILE.duplicate_archived"
    log_success "✓ Дублирующий service.yaml перемещен в архив"
fi

# Проверяем корректность kustomization.yaml
KUSTOMIZATION_FILE="$CONFIG_DIR/kustomization.yaml"
if [ -f "$KUSTOMIZATION_FILE" ]; then
    log_info "Проверяем kustomization.yaml"
    
    # Проверяем что namespace указан правильно
    if grep -q "namespace: netguard-system" "$KUSTOMIZATION_FILE"; then
        log_success "✓ Namespace в kustomization.yaml корректен"
    else
        log_warning "Namespace в kustomization.yaml может быть некорректен"
    fi
    
    # Удаляем service.yaml из ресурсов если он был заархивирован
    if [ -f "$SERVICE_FILE.duplicate_archived" ]; then
        sed -i.bak '/^- service\.yaml$/d' "$KUSTOMIZATION_FILE"
        rm -f "$KUSTOMIZATION_FILE.bak"
        log_success "✓ service.yaml удален из kustomization.yaml"
    fi
fi

# Проверяем результат
log_info "Проверяем результат исправлений..."

echo -e "\n📊 Сводка изменений:"
echo "===================="

for file in "${FILES_TO_FIX[@]}"; do
    FILE_PATH="$CONFIG_DIR/$file"
    if [ -f "$FILE_PATH" ]; then
        DEFAULT_COUNT=$(grep -c "namespace: default" "$FILE_PATH" 2>/dev/null || echo "0")
        NETGUARD_COUNT=$(grep -c "namespace: netguard-system" "$FILE_PATH" 2>/dev/null || echo "0")
        
        if [ "$DEFAULT_COUNT" -eq "0" ] && [ "$NETGUARD_COUNT" -gt "0" ]; then
            echo "✅ $file - исправлен ($NETGUARD_COUNT namespace'ов netguard-system)"
        elif [ "$DEFAULT_COUNT" -gt "0" ]; then
            echo "⚠️  $file - остались default namespace'ы ($DEFAULT_COUNT)"
        else
            echo "ℹ️  $file - без namespace'ов"
        fi
    fi
done

# Финальная проверка
TOTAL_DEFAULT=$(find "$CONFIG_DIR" -name "*.yaml" -exec grep -l "namespace: default" {} \; 2>/dev/null | wc -l)

if [ "$TOTAL_DEFAULT" -eq "0" ]; then
    log_success "🎉 Все namespace'ы успешно исправлены!"
    echo -e "\n📁 Резервная копия сохранена в: $BACKUP_DIR"
    echo -e "🚀 Теперь можно запустить: ./scripts/deploy-complete.sh"
else
    log_warning "⚠️  Найдены файлы с namespace: default ($TOTAL_DEFAULT файлов)"
    echo "Проверьте файлы вручную:"
    find "$CONFIG_DIR" -name "*.yaml" -exec grep -l "namespace: default" {} \; 2>/dev/null
fi

echo -e "\n=== Исправление namespace'ов завершено ===" 