#!/bin/bash

# Script to cleanup old netguard deployment from default namespace
# Очистка старого развертывания netguard из default namespace

set -e

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

echo "🧹 Очистка старого развертывания Netguard из default namespace"
echo "=============================================================="
echo ""

# Check current state
log_info "Проверка текущего состояния..."
echo "Текущие netguard ресурсы в default:"
kubectl get all -n default | grep netguard || echo "  Нет ресурсов"

echo -e "\nТекущие netguard secrets в default:"
kubectl get secret -n default | grep netguard || echo "  Нет секретов"

echo -e "\nТекущие netguard configmaps в default:"
kubectl get configmap -n default | grep netguard || echo "  Нет configmaps"

echo ""
read -p "Продолжить удаление? (y/N): " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    log_info "Отменено пользователем"
    exit 0
fi

echo ""
log_info "Начинаем очистку..."

# Remove deployments
log_info "Удаление deployments..."
kubectl delete deployment netguard-apiserver -n default --ignore-not-found=true
kubectl delete deployment netguard-backend -n default --ignore-not-found=true

# Remove services
log_info "Удаление services..."
kubectl delete service netguard-apiserver -n default --ignore-not-found=true
kubectl delete service netguard-backend -n default --ignore-not-found=true

# Remove configmaps
log_info "Удаление configmaps..."
kubectl delete configmap netguard-apiserver-config -n default --ignore-not-found=true

# Remove secrets
log_info "Удаление secrets..."
kubectl delete secret netguard-apiserver-certs -n default --ignore-not-found=true

# Remove APIService (it will be recreated with new service reference)
log_info "Удаление APIService (будет пересоздан)..."
kubectl delete apiservice v1beta1.netguard.sgroups.io --ignore-not-found=true

# Wait for resources to be deleted
log_info "Ожидание завершения удаления..."
sleep 10

# Check if everything is cleaned up
log_info "Проверка результата очистки..."
REMAINING_RESOURCES=$(kubectl get all -n default | grep netguard || echo "")
if [ -z "$REMAINING_RESOURCES" ]; then
    log_success "✅ Все ресурсы netguard удалены из default namespace"
else
    log_warning "⚠ Остались ресурсы:"
    echo "$REMAINING_RESOURCES"
fi

REMAINING_SECRETS=$(kubectl get secret -n default | grep netguard || echo "")
if [ -z "$REMAINING_SECRETS" ]; then
    log_success "✅ Все секреты netguard удалены из default namespace"
else
    log_warning "⚠ Остались секреты:"
    echo "$REMAINING_SECRETS"
fi

REMAINING_CONFIGMAPS=$(kubectl get configmap -n default | grep netguard || echo "")
if [ -z "$REMAINING_CONFIGMAPS" ]; then
    log_success "✅ Все configmaps netguard удалены из default namespace"
else
    log_warning "⚠ Остались configmaps:"
    echo "$REMAINING_CONFIGMAPS"
fi

# Check APIService
APISERVICE_STATUS=$(kubectl get apiservice v1beta1.netguard.sgroups.io 2>/dev/null || echo "NotFound")
if [ "$APISERVICE_STATUS" = "NotFound" ]; then
    log_success "✅ APIService удален"
else
    log_warning "⚠ APIService все еще существует:"
    kubectl get apiservice v1beta1.netguard.sgroups.io
fi

echo ""
log_success "🎉 Очистка завершена!"
echo ""
echo "📝 Следующие шаги:"
echo "1. Исправить namespace'ы: ./fix-namespaces.sh"
echo "2. Развернуть в правильном namespace: ./deploy-complete.sh"
echo "3. Протестировать: ./test-complete.sh" 