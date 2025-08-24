#!/bin/bash

# Complete deployment script for Netguard Platform
# Полное развертывание netguard платформы в Kubernetes

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
NAMESPACE="netguard-system"

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

# Check prerequisites
check_prereqs() {
    log_info "Проверка предварительных требований..."
    
    if ! command -v kubectl &> /dev/null; then
        log_error "kubectl не установлен"
        exit 1
    fi
    
    if ! command -v docker &> /dev/null; then
        log_error "docker не установлен"
        exit 1
    fi
    
    # Check if connected to kubernetes cluster
    if ! kubectl cluster-info &> /dev/null; then
        log_error "Нет подключения к Kubernetes кластеру"
        exit 1
    fi
    
    CLUSTER_NAME=$(kubectl config current-context)
    log_info "Подключен к кластеру: $CLUSTER_NAME"
    
    # Check if this is minikube
    if echo "$CLUSTER_NAME" | grep -q "minikube"; then
        MINIKUBE_DETECTED=true
        log_info "Обнаружен minikube кластер"
    else
        MINIKUBE_DETECTED=false
        log_warning "Не minikube кластер - некоторые оптимизации будут пропущены"
    fi
    
    log_success "Предварительные требования выполнены"
}

# Clean previous deployments
cleanup_previous() {
    log_info "Очистка предыдущих развертываний..."
    
    # Remove old deployments
    kubectl delete namespace "$NAMESPACE" --ignore-not-found=true --timeout=60s
    
    # Clean up APIService if exists
    kubectl delete apiservice v1beta1.netguard.sgroups.io --ignore-not-found=true
    
    # Clean up webhooks if exist
    kubectl delete validatingwebhookconfigurations netguard-validator --ignore-not-found=true
    kubectl delete mutatingwebhookconfigurations netguard-mutator --ignore-not-found=true
    
    # Remove any resources that might be stuck
    kubectl delete -k "$PROJECT_ROOT/config/k8s" --ignore-not-found=true --timeout=60s || true
    
    log_info "Ожидание завершения очистки..."
    sleep 15
    
    log_success "Предыдущие развертывания очищены"
}

# Create namespace with proper labels
create_namespace() {
    log_info "Создание namespace: $NAMESPACE"
    
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Namespace
metadata:
  name: $NAMESPACE
  labels:
    app.kubernetes.io/name: netguard
    app.kubernetes.io/part-of: netguard-platform
    app.kubernetes.io/managed-by: netguard-deploy-script
    pod-security.kubernetes.io/enforce: restricted
    pod-security.kubernetes.io/audit: restricted  
    pod-security.kubernetes.io/warn: restricted
  annotations:
    netguard.sgroups.io/deployment-time: "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    netguard.sgroups.io/deployed-by: "$(whoami)"
EOF
    
    log_success "Namespace $NAMESPACE создан"
}

# Generate K8s code
generate_code() {
    log_info "Генерация Kubernetes кода..."
    cd "$PROJECT_ROOT"
    
    if make generate-k8s; then
        log_success "Kubernetes код сгенерирован"
    else
        log_error "Ошибка генерации кода"
        exit 1
    fi
}

# Build images
build_images() {
    log_info "Сборка Docker образов..."
    cd "$PROJECT_ROOT"
    
    # Build API server image
    if make docker-build-k8s-apiserver; then
        log_success "API Server образ собран"
    else
        log_error "Ошибка сборки API Server образа"
        exit 1
    fi
    
    # Build backend image if Makefile target exists
    if make docker-build 2>/dev/null; then
        log_success "Backend образ собран"
    else
        log_warning "Backend образ не собран (target не найден)"
    fi
    
    # Load images to minikube if detected
    if [ "$MINIKUBE_DETECTED" = true ]; then
        log_info "Загрузка образов в minikube..."
        
        # Load images into minikube
        if command -v minikube &> /dev/null; then
            minikube image load netguard/k8s-apiserver:latest 2>/dev/null || log_warning "Не удалось загрузить k8s-apiserver образ"
            minikube image load netguard/pg-backend:latest 2>/dev/null || log_warning "Не удалось загрузить pg-backend образ"
            log_success "Образы загружены в minikube"
        fi
    fi
}

# Deploy to Kubernetes
deploy_k8s() {
    log_info "Развертывание в Kubernetes..."
    cd "$PROJECT_ROOT"
    
    # Apply all configurations
    if kubectl apply -k config/k8s/; then
        log_success "Конфигурации применены"
    else
        log_error "Ошибка применения конфигураций"
        exit 1
    fi
}

# Wait for deployments
wait_for_ready() {
    log_info "Ожидание готовности развертываний..."
    
    # Wait for API server
    log_info "Ожидание готовности netguard-apiserver..."
    if kubectl wait --for=condition=available --timeout=300s deployment/netguard-apiserver -n "$NAMESPACE"; then
        log_success "netguard-apiserver готов"
    else
        log_error "Timeout ожидания netguard-apiserver"
        kubectl describe deployment/netguard-apiserver -n "$NAMESPACE"
        exit 1
    fi
    
    # Wait for backend
    log_info "Ожидание готовности netguard-backend..."
    if kubectl wait --for=condition=available --timeout=300s deployment/netguard-backend -n "$NAMESPACE"; then
        log_success "netguard-backend готов"
    else
        log_error "Timeout ожидания netguard-backend"
        kubectl describe deployment/netguard-backend -n "$NAMESPACE"
        exit 1
    fi
    
    # Additional wait for API registration
    log_info "Ожидание регистрации API..."
    sleep 30
    
    # Check APIService status (v1beta1 for Aggregation Layer)
    if kubectl get apiservice v1beta1.netguard.sgroups.io -o jsonpath='{.status.conditions[?(@.type=="Available")].status}' | grep -q "True"; then
        log_success "v1beta1.netguard.sgroups.io (Aggregation Layer) зарегистрирован и доступен"
    else
        log_warning "v1beta1.netguard.sgroups.io (Aggregation Layer) может быть не полностью готов"
        kubectl describe apiservice v1beta1.netguard.sgroups.io
        
        # Check if CRD version exists
        if kubectl get crd | grep -q netguard; then
            log_info "ℹ️ Обнаружена CRD реализация (v1alpha1), но развертываем Aggregation Layer (v1beta1)"
        fi
    fi
}

# Show deployment status
show_status() {
    log_info "Статус развертывания:"
    echo "======================"
    
    echo -e "\n📦 Поды в namespace $NAMESPACE:"
    kubectl get pods -n "$NAMESPACE" -o wide
    
    echo -e "\n🔌 Сервисы в namespace $NAMESPACE:"
    kubectl get services -n "$NAMESPACE"
    
    echo -e "\n🚀 Развертывания в namespace $NAMESPACE:"
    kubectl get deployments -n "$NAMESPACE"
    
    echo -e "\n🔗 APIService статус:"
    kubectl get apiservice v1beta1.netguard.sgroups.io
    
    echo -e "\n🎯 API ресурсы netguard:"
    kubectl api-resources --api-group=netguard.sgroups.io 2>/dev/null || log_warning "API группа пока недоступна"
    
    echo -e "\n📝 Полезные команды:"
    echo "  kubectl get all -n $NAMESPACE"
    echo "  kubectl logs -f deployment/netguard-apiserver -n $NAMESPACE"
    echo "  kubectl logs -f deployment/netguard-backend -n $NAMESPACE"
    echo "  kubectl port-forward service/netguard-apiserver 8443:443 -n $NAMESPACE"
}

# Create quick test resource
create_test_resource() {
    log_info "Создание тестового ресурса для проверки..."
    
    sleep 10  # Wait a bit more for API to be fully ready
    
cat <<EOF | kubectl apply -f - || log_warning "Не удалось создать тестовый ресурс"
apiVersion: netguard.sgroups.io/v1beta1
kind: Service
metadata:
  name: deployment-test-service
  namespace: $NAMESPACE
spec:
  description: "Test service created during Aggregation Layer deployment (v1beta1)"
  ingressPorts:
  - protocol: TCP
    port: "80"
    description: "HTTP port"
EOF
    
    if kubectl get services.v1beta1.netguard.sgroups.io deployment-test-service -n "$NAMESPACE" &>/dev/null; then
        log_success "Тестовый ресурс v1beta1 (Aggregation Layer) создан и доступен"
        kubectl delete services.v1beta1.netguard.sgroups.io deployment-test-service -n "$NAMESPACE" &>/dev/null || true
        log_info "Тестовый ресурс v1beta1 очищен"
    else
        log_warning "Тестовый ресурс v1beta1 не удалось проверить"
    fi
}

# Main deployment function
main() {
    echo "🚀 Развертывание Netguard Platform"
    echo "=================================="
    echo "Namespace: $NAMESPACE"
    echo "Project: $PROJECT_ROOT"
    echo ""
    
    check_prereqs
    cleanup_previous
    create_namespace
    generate_code
    build_images
    deploy_k8s
    wait_for_ready
    create_test_resource
    
    log_success "🎉 Развертывание завершено успешно!"
    echo ""
    show_status
    echo ""
    echo "🧪 Для запуска комплексного тестирования выполните:"
    echo "   ./scripts/test-complete.sh"
}

# Handle script arguments
case "${1:-deploy}" in
    deploy)
        main
        ;;
    cleanup)
        check_prereqs
        cleanup_previous
        log_success "Очистка завершена"
        ;;
    status)
        show_status
        ;;
    help|*)
        echo "Usage: $0 [deploy|cleanup|status|help]"
        echo ""
        echo "Commands:"
        echo "  deploy  - Полное развертывание (по умолчанию)"
        echo "  cleanup - Только очистка предыдущих развертываний"
        echo "  status  - Показать текущий статус"
        echo "  help    - Показать эту справку"
        ;;
esac 