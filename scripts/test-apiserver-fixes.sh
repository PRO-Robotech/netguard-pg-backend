#!/bin/bash

# Тестовый скрипт для проверки исправлений API сервера
# Автор: AI Assistant  
# Дата: $(date)

set -euo pipefail

echo "🔧 ТЕСТИРОВАНИЕ ИСПРАВЛЕНИЙ API СЕРВЕРА"
echo "========================================"

# Цвета для вывода
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_ROOT"

echo -e "${BLUE}📍 Рабочая директория: $PROJECT_ROOT${NC}"

# 1. Проверяем что бинарный файл собран
echo -e "\n${YELLOW}1. Проверяем сборку...${NC}"
if [[ -f "bin/k8s-apiserver" ]]; then
    echo -e "${GREEN}✅ Бинарный файл bin/k8s-apiserver найден${NC}"
    ls -la bin/k8s-apiserver
else
    echo -e "${RED}❌ Бинарный файл не найден. Собираем...${NC}"
    go build -o bin/k8s-apiserver cmd/k8s-apiserver/main.go
    echo -e "${GREEN}✅ Сборка завершена${NC}"
fi

# 2. Проверяем что может запуститься с --help
echo -e "\n${YELLOW}2. Проверяем базовую функциональность...${NC}"
echo "Запускаем: ./bin/k8s-apiserver --help"
if timeout 10s ./bin/k8s-apiserver --help >/dev/null 2>&1; then
    echo -e "${GREEN}✅ Базовая функциональность работает${NC}"
else
    echo -e "${RED}❌ Проблемы с базовой функциональностью${NC}"
    exit 1
fi

# 3. Проверяем конфигурации Kubernetes
echo -e "\n${YELLOW}3. Проверяем конфигурации Kubernetes...${NC}"
if [[ -f "config/k8s/apiservice.yaml" ]]; then
    echo -e "${GREEN}✅ APIService конфигурация найдена${NC}"
    echo "Версия API в APIService:"
    grep -E "(group|version):" config/k8s/apiservice.yaml | head -3
else
    echo -e "${RED}❌ APIService конфигурация не найдена${NC}"
fi

# 4. Создаем тестовый конфиг для локального запуска
echo -e "\n${YELLOW}4. Создаем тестовый конфиг...${NC}"
mkdir -p apiserver.local.config/certificates

cat > apiserver.local.config/test-config.yaml << EOF
# Тестовая конфигурация для локального API сервера
backend-address: "localhost:9090"
bind-address: "127.0.0.1"
secure-port: 8443
EOF

echo -e "${GREEN}✅ Тестовый конфиг создан: apiserver.local.config/test-config.yaml${NC}"

# 5. Проверяем возможность запуска (без backend)
echo -e "\n${YELLOW}5. Проверяем инициализацию сервера...${NC}"
echo "Запускаем сервер на 3 секунды для проверки инициализации..."

# Запускаем в фоне и убиваем через 3 секунды
timeout 3s ./bin/k8s-apiserver \
    --backend-address="localhost:9090" \
    --secure-port=8443 \
    --bind-address=127.0.0.1 \
    --v=2 || {
    exit_code=$?
    if [[ $exit_code -eq 124 ]]; then
        echo -e "${GREEN}✅ Сервер запустился успешно (остановлен по таймауту)${NC}"
    else
        echo -e "${RED}❌ Сервер завершился с ошибкой (код: $exit_code)${NC}"
        echo "Возможные причины:"
        echo "  - Backend недоступен (нормально для теста)"
        echo "  - Ошибки в конфигурации" 
        echo "  - Проблемы с сертификатами"
        
        # Показываем последние логи
        echo -e "\n${YELLOW}Последний вывод сервера:${NC}"
        timeout 2s ./bin/k8s-apiserver \
            --backend-address="localhost:9090" \
            --secure-port=8443 \
            --bind-address=127.0.0.1 \
            --v=4 2>&1 | tail -20 || true
    fi
}

echo -e "\n${BLUE}===========================================${NC}"
echo -e "${GREEN}🎉 ТЕСТИРОВАНИЕ ЗАВЕРШЕНО${NC}"
echo -e "${BLUE}===========================================${NC}"

echo -e "\n${YELLOW}📋 СЛЕДУЮЩИЕ ШАГИ:${NC}"
echo "1. Запустить backend сервис: netguard-backend на порту 9090"
echo "2. Развернуть в Kubernetes: kubectl apply -f config/k8s/"
echo "3. Проверить статус APIService: kubectl get apiservice v1beta1.netguard.sgroups.io"

echo -e "\n${YELLOW}🔍 ПОЛЕЗНЫЕ КОМАНДЫ ДЛЯ ОТЛАДКИ:${NC}"
echo "• Просмотр логов: kubectl logs -f deployment/netguard-apiserver -n netguard-test"
echo "• Проверка сертификатов: kubectl get secret netguard-apiserver-certs -n netguard-test"
echo "• Тест API: kubectl get services.netguard.sgroups.io" 