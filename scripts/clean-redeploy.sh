#!/bin/bash

# Complete clean redeploy script for Netguard Platform
# Полная переустановка netguard платформы с правильными namespace'ами

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
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

log_step() {
    echo -e "\n${CYAN}=== $1 ===${NC}"
}

main() {
    echo "🔄 Полная переустановка Netguard Platform"
    echo "========================================="
    echo "Переход с default namespace на netguard-system"
    echo "Время: $(date)"
    echo ""
    
    log_warning "⚠️ ВНИМАНИЕ: Этот скрипт удалит существующее развертывание из default namespace!"
    echo ""
    echo "Что будет сделано:"
    echo "1. 🧹 Очистка старого развертывания из default namespace"
    echo "2. 🔧 Исправление namespace'ов в конфигурации"
    echo "3. 🔐 Перегенерация TLS сертификатов"
    echo "4. 🚀 Развертывание в netguard-system namespace"
    echo "5. 🧪 Тестирование v1beta1 (Aggregation Layer)"
    echo ""
    
    if [[ "$NON_INTERACTIVE" != "true" ]]; then
        read -p "Продолжить? (y/N): " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            log_info "Отменено пользователем"
            exit 0
        fi
    else
        log_info "Неинтерактивный режим - автоматически продолжаем"
    fi
    
    cd "$SCRIPT_DIR"
    
    # Step 1: Cleanup old deployment
    log_step "Шаг 1: Очистка старого развертывания"
    if [ -f "./cleanup-old-deployment.sh" ]; then
        # Run cleanup in non-interactive mode
        echo "y" | ./cleanup-old-deployment.sh
    else
        log_error "Скрипт очистки не найден"
        exit 1
    fi
    
    # Step 2: Fix namespaces
    log_step "Шаг 2: Исправление namespace'ов"
    if [ -f "./fix-namespaces.sh" ]; then
        ./fix-namespaces.sh
    else
        log_error "Скрипт исправления namespace'ов не найден"
        exit 1
    fi
    
    # Step 3: Regenerate certificates
    log_step "Шаг 3: Перегенерация TLS сертификатов"
    cd "$PROJECT_ROOT"
    if [ -f "./scripts/generate-certs.sh" ]; then
        ./scripts/generate-certs.sh
        log_success "Сертификаты обновлены для netguard-system namespace"
    else
        log_error "Скрипт генерации сертификатов не найден"
        exit 1
    fi
    
    # Step 4: Deploy
    log_step "Шаг 4: Развертывание в netguard-system"
    cd "$SCRIPT_DIR"
    if [ -f "./deploy-complete.sh" ]; then
        ./deploy-complete.sh
    else
        log_error "Скрипт развертывания не найден"
        exit 1
    fi
    
    # Step 5: Test
    log_step "Шаг 5: Тестирование Aggregation Layer (v1beta1)"
    if [ -f "./test-complete.sh" ]; then
        ./test-complete.sh quick
    else
        log_warning "Скрипт тестирования не найден - пропускаем"
    fi
    
    # Final status
    log_step "Итоговый статус"
    
    echo "📊 Проверка развертывания:"
    kubectl get all -n netguard-system | head -10
    
    echo -e "\n🔗 Проверка APIService:"
    kubectl get apiservice v1beta1.netguard.sgroups.io
    
    echo -e "\n🎯 Доступные API ресурсы:"
    kubectl api-resources --api-group=netguard.sgroups.io | head -5
    
    log_success "🎉 Переустановка завершена успешно!"
    echo ""
    echo "📝 Полезные команды:"
    echo "  kubectl get all -n netguard-system"
    echo "  kubectl logs -f deployment/netguard-apiserver -n netguard-system"
    echo "  ./test-complete.sh  # Полное тестирование"
    echo "  ./compare-implementations.sh  # Сравнение с CRD реализацией"
}

# Parse arguments
NON_INTERACTIVE=false
while [[ $# -gt 0 ]]; do
    case $1 in
        -y|--yes)
            NON_INTERACTIVE=true
            shift
            ;;
        run|help)
            COMMAND=$1
            shift
            ;;
        *)
            shift
            ;;
    esac
done

# Handle script arguments
case "${COMMAND:-run}" in
    run)
        main
        ;;
    help|*)
        echo "Usage: $0 [run|help] [-y|--yes]"
        echo ""
        echo "Скрипт для полной переустановки Netguard Platform:"
        echo "- Удаляет старое развертывание из default namespace"
        echo "- Исправляет namespace'ы в конфигурации"  
        echo "- Перегенерирует TLS сертификаты"
        echo "- Развертывает в netguard-system namespace"
        echo "- Тестирует Aggregation Layer (v1beta1)"
        echo ""
        echo "Команды:"
        echo "  run   - Запустить полную переустановку (по умолчанию)"
        echo "  help  - Показать эту справку"
        echo ""
        echo "Опции:"
        echo "  -y, --yes  - Неинтерактивный режим (автоматически отвечать yes)"
        ;;
esac 