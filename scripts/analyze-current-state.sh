#!/bin/bash

# Script to analyze current netguard state in minikube
# Анализ текущего состояния netguard в minikube

set -e

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

log_section() {
    echo -e "\n${CYAN}=== $1 ===${NC}"
}

# Check if connected to minikube
check_minikube() {
    log_section "Проверка подключения к minikube"
    
    if ! command -v kubectl &> /dev/null; then
        log_error "kubectl не установлен"
        exit 1
    fi
    
    if ! command -v minikube &> /dev/null; then
        log_warning "minikube не установлен или недоступен"
    fi
    
    # Check current context
    CURRENT_CONTEXT=$(kubectl config current-context 2>/dev/null || echo "none")
    if [ "$CURRENT_CONTEXT" = "none" ]; then
        log_error "Нет активного Kubernetes контекста"
        exit 1
    fi
    
    log_info "Текущий контекст: $CURRENT_CONTEXT"
    
    if echo "$CURRENT_CONTEXT" | grep -q "minikube"; then
        log_success "✓ Подключен к minikube"
        
        # Check minikube status
        if command -v minikube &> /dev/null; then
            MINIKUBE_STATUS=$(minikube status -f '{{.Host}}' 2>/dev/null || echo "Unknown")
            log_info "Статус minikube: $MINIKUBE_STATUS"
        fi
    else
        log_warning "⚠ Подключен не к minikube кластеру"
    fi
    
    # Show cluster info
    log_info "Информация о кластере:"
    kubectl cluster-info | head -3
}

# Analyze namespaces
analyze_namespaces() {
    log_section "Анализ namespace'ов"
    
    echo "📁 Все namespace'ы в кластере:"
    kubectl get namespaces -o wide
    
    # Check for netguard-related namespaces
    NETGUARD_NAMESPACES=$(kubectl get namespaces -o name | grep -i netguard || echo "")
    
    if [ -n "$NETGUARD_NAMESPACES" ]; then
        echo -e "\n🎯 Найдены netguard namespace'ы:"
        for ns in $NETGUARD_NAMESPACES; do
            NS_NAME=$(echo "$ns" | cut -d'/' -f2)
            echo "  - $NS_NAME"
            
            # Show namespace details
            kubectl describe namespace "$NS_NAME" | grep -E "(Labels|Annotations)" || true
        done
    else
        log_warning "❌ Netguard namespace'ы не найдены"
    fi
    
    # Check default namespace for netguard resources
    DEFAULT_NETGUARD=$(kubectl get all -n default | grep -i netguard || echo "")
    if [ -n "$DEFAULT_NETGUARD" ]; then
        log_warning "⚠ Найдены netguard ресурсы в default namespace:"
        kubectl get all -n default | grep -i netguard
    fi
}

# Analyze deployments and pods
analyze_workloads() {
    log_section "Анализ рабочих нагрузок"
    
    echo "🚀 Все deployments в кластере:"
    kubectl get deployments -A | head -1  # Header
    kubectl get deployments -A | grep -i netguard || echo "  Netguard deployments не найдены"
    
    echo -e "\n🟢 Все поды в кластере:"
    kubectl get pods -A | head -1  # Header
    kubectl get pods -A | grep -i netguard || echo "  Netguard поды не найдены"
    
    # Check pod status details
    NETGUARD_PODS=$(kubectl get pods -A -o name | grep -i netguard || echo "")
    if [ -n "$NETGUARD_PODS" ]; then
        echo -e "\n📊 Детали netguard подов:"
        for pod in $NETGUARD_PODS; do
            POD_NAME=$(echo "$pod" | cut -d'/' -f2)
            POD_NAMESPACE=$(kubectl get "$pod" -o jsonpath='{.metadata.namespace}' 2>/dev/null || echo "unknown")
            POD_STATUS=$(kubectl get "$pod" -o jsonpath='{.status.phase}' 2>/dev/null || echo "unknown")
            
            echo "  - $POD_NAME (namespace: $POD_NAMESPACE, status: $POD_STATUS)"
            
            # Show recent events for problematic pods
            if [ "$POD_STATUS" != "Running" ]; then
                echo "    События:"
                kubectl get events -n "$POD_NAMESPACE" --field-selector involvedObject.name="$POD_NAME" --sort-by='.lastTimestamp' | tail -3
            fi
        done
    fi
}

# Analyze services
analyze_services() {
    log_section "Анализ сервисов"
    
    echo "🔌 Все сервисы в кластере:"
    kubectl get services -A | head -1  # Header
    kubectl get services -A | grep -i netguard || echo "  Netguard сервисы не найдены"
    
    # Check ClusterIP availability
    NETGUARD_SERVICES=$(kubectl get services -A -o name | grep -i netguard || echo "")
    if [ -n "$NETGUARD_SERVICES" ]; then
        echo -e "\n🔗 Детали netguard сервисов:"
        for svc in $NETGUARD_SERVICES; do
            SVC_NAME=$(echo "$svc" | cut -d'/' -f2)
            SVC_NAMESPACE=$(kubectl get "$svc" -o jsonpath='{.metadata.namespace}' 2>/dev/null || echo "unknown")
            SVC_TYPE=$(kubectl get "$svc" -o jsonpath='{.spec.type}' 2>/dev/null || echo "unknown")
            SVC_CLUSTER_IP=$(kubectl get "$svc" -o jsonpath='{.spec.clusterIP}' 2>/dev/null || echo "unknown")
            
            echo "  - $SVC_NAME (namespace: $SVC_NAMESPACE, type: $SVC_TYPE, ClusterIP: $SVC_CLUSTER_IP)"
        done
    fi
}

# Analyze API resources
analyze_api_resources() {
    log_section "Анализ API ресурсов"
    
    # Check APIServices (Aggregation Layer)
    echo "🔗 APIServices (Aggregation Layer):"
    kubectl get apiservices | head -1  # Header
    NETGUARD_APISERVICES=$(kubectl get apiservices | grep -i netguard || echo "")
    if [ -n "$NETGUARD_APISERVICES" ]; then
        echo "$NETGUARD_APISERVICES"
        
        # Check v1beta1 specifically (Aggregation Layer)
        echo -e "\n🎯 Проверка v1beta1.netguard.sgroups.io (Aggregation Layer):"
        V1BETA1_STATUS=$(kubectl get apiservice v1beta1.netguard.sgroups.io -o jsonpath='{.status.conditions[?(@.type=="Available")].status}' 2>/dev/null || echo "NotFound")
        if [ "$V1BETA1_STATUS" = "True" ]; then
            log_success "✅ v1beta1.netguard.sgroups.io (Aggregation Layer) доступен"
        elif [ "$V1BETA1_STATUS" = "NotFound" ]; then
            log_warning "❌ v1beta1.netguard.sgroups.io (Aggregation Layer) не найден"
        else
            log_warning "⚠ v1beta1.netguard.sgroups.io статус: $V1BETA1_STATUS"
        fi
    else
        echo "  Netguard APIServices не найдены"
    fi
    
    # Check CRDs (традиционная реализация)
    echo -e "\n📋 Custom Resource Definitions (CRD реализация):"
    kubectl get crd | head -1  # Header
    NETGUARD_CRDS=$(kubectl get crd | grep -i netguard || echo "")
    if [ -n "$NETGUARD_CRDS" ]; then
        echo "$NETGUARD_CRDS"
        
        # Check v1alpha1 specifically (CRD implementation)
        echo -e "\n🔍 Найдены CRD (v1alpha1 - традиционная реализация):"
        kubectl get crd | grep netguard | while read -r line; do
            CRD_NAME=$(echo "$line" | awk '{print $1}')
            echo "  - $CRD_NAME"
        done
    else
        echo "  Netguard CRDs не найдены"
    fi
    
    # Determine which implementation is active
    echo -e "\n🔬 Определение активной реализации:"
    HAS_AGGREGATION=$(kubectl get apiservice v1beta1.netguard.sgroups.io 2>/dev/null && echo "true" || echo "false")
    HAS_CRD=$(kubectl get crd | grep -q netguard && echo "true" || echo "false")
    
    if [ "$HAS_AGGREGATION" = "true" ] && [ "$HAS_CRD" = "true" ]; then
        log_warning "⚠ Обнаружены ОБЕ реализации (Aggregation Layer + CRD)"
        echo "  - v1beta1 (Aggregation Layer): $([ "$V1BETA1_STATUS" = "True" ] && echo "Активен" || echo "Неактивен")"
        echo "  - v1alpha1 (CRD): Присутствует"
        log_info "💡 Рекомендуется использовать только одну реализацию"
    elif [ "$HAS_AGGREGATION" = "true" ]; then
        log_success "✅ Активна Aggregation Layer реализация (v1beta1)"
    elif [ "$HAS_CRD" = "true" ]; then
        log_info "ℹ️ Активна CRD реализация (v1alpha1)"
        log_warning "⚠ Для тестирования Aggregation Layer нужна v1beta1"
    else
        log_warning "❌ Netguard реализации не найдены"
    fi
    
    # Try to discover netguard API resources
    echo -e "\n🎯 Обнаружение netguard API ресурсов:"
    if kubectl api-resources --api-group=netguard.sgroups.io &>/dev/null; then
        log_success "✓ API группа netguard.sgroups.io доступна:"
        kubectl api-resources --api-group=netguard.sgroups.io
        
        # Try to list some resources with version detection
        echo -e "\n📦 Существующие netguard ресурсы:"
        
        # Check v1beta1 resources (Aggregation Layer)
        echo "  🔸 v1beta1 ресурсы (Aggregation Layer):"
        if kubectl api-resources --api-group=netguard.sgroups.io | grep -q v1beta1; then
            kubectl get services.v1beta1.netguard.sgroups.io -A 2>/dev/null | head -5 || echo "    Нет services.v1beta1.netguard.sgroups.io"
            kubectl get addressgroups.v1beta1.netguard.sgroups.io -A 2>/dev/null | head -3 || echo "    Нет addressgroups.v1beta1.netguard.sgroups.io"
        else
            echo "    v1beta1 ресурсы недоступны"
        fi
        
        # Check v1alpha1 resources (CRD)
        echo "  🔸 v1alpha1 ресурсы (CRD):"
        if kubectl api-resources --api-group=netguard.sgroups.io | grep -q v1alpha1; then
            kubectl get services.v1alpha1.netguard.sgroups.io -A 2>/dev/null | head -3 || echo "    Нет services.v1alpha1.netguard.sgroups.io"
        else
            echo "    v1alpha1 ресурсы недоступны"
        fi
    else
        log_warning "❌ API группа netguard.sgroups.io недоступна"
    fi
}

# Analyze configuration issues
analyze_config_issues() {
    log_section "Анализ проблем конфигурации"
    
    local issues_found=0
    
    # Check for namespace inconsistencies
    echo "🔍 Проверка несоответствий namespace'ов:"
    
    # Check if resources are in different namespaces
    NETGUARD_NAMESPACES_LIST=$(kubectl get all -A | grep -i netguard | awk '{print $1}' | sort | uniq || echo "")
    
    if [ -n "$NETGUARD_NAMESPACES_LIST" ]; then
        NAMESPACE_COUNT=$(echo "$NETGUARD_NAMESPACES_LIST" | wc -l)
        if [ "$NAMESPACE_COUNT" -gt 1 ]; then
            log_warning "⚠ Netguard ресурсы разбросаны по $NAMESPACE_COUNT namespace'ам:"
            echo "$NETGUARD_NAMESPACES_LIST"
            issues_found=$((issues_found + 1))
        else
            log_success "✓ Все netguard ресурсы в одном namespace: $NETGUARD_NAMESPACES_LIST"
        fi
    fi
    
    # Check for pods in Error/CrashLoopBackOff states
    echo -e "\n🚨 Проверка проблемных подов:"
    PROBLEM_PODS=$(kubectl get pods -A | grep -i netguard | grep -E "(Error|CrashLoopBackOff|Pending|ImagePullBackOff)" || echo "")
    
    if [ -n "$PROBLEM_PODS" ]; then
        log_warning "⚠ Найдены проблемные netguard поды:"
        echo "$PROBLEM_PODS"
        issues_found=$((issues_found + 1))
    else
        log_success "✓ Нет проблемных netguard подов"
    fi
    
    # Check APIService availability
    echo -e "\n🔌 Проверка доступности APIService:"
    APISERVICE_STATUS=$(kubectl get apiservice v1beta1.netguard.sgroups.io -o jsonpath='{.status.conditions[?(@.type=="Available")].status}' 2>/dev/null || echo "NotFound")
    
    if [ "$APISERVICE_STATUS" = "True" ]; then
        log_success "✓ APIService доступен"
    elif [ "$APISERVICE_STATUS" = "NotFound" ]; then
        log_warning "⚠ APIService не найден"
        issues_found=$((issues_found + 1))
    else
        log_warning "⚠ APIService недоступен (статус: $APISERVICE_STATUS)"
        issues_found=$((issues_found + 1))
    fi
    
    echo -e "\n📊 Итого найдено проблем: $issues_found"
    
    if [ "$issues_found" -eq 0 ]; then
        log_success "🎉 Конфигурация выглядит корректной!"
    else
        log_warning "🔧 Рекомендуется исправить найденные проблемы"
    fi
}

# Show resource usage
show_resource_usage() {
    log_section "Использование ресурсов"
    
    echo "📈 Использование ресурсов кластера:"
    kubectl top nodes 2>/dev/null || echo "  Metrics сервер недоступен"
    
    echo -e "\n📊 Использование ресурсов подами netguard:"
    kubectl top pods -A 2>/dev/null | grep -i netguard || echo "  Metrics недоступны или netguard поды не найдены"
}

# Generate recommendations
generate_recommendations() {
    log_section "Рекомендации"
    
    echo "💡 Рекомендации на основе анализа:"
    
    # Check if any netguard resources exist
    NETGUARD_RESOURCES=$(kubectl get all -A | grep -i netguard || echo "")
    
    if [ -z "$NETGUARD_RESOURCES" ]; then
        echo "1. 🚀 Netguard не развернут. Рекомендуется:"
        echo "   - Запустить: ./scripts/fix-namespaces.sh"
        echo "   - Затем: ./scripts/deploy-complete.sh"
    else
        # Check namespace consistency
        NAMESPACE_COUNT=$(kubectl get all -A | grep -i netguard | awk '{print $1}' | sort | uniq | wc -l)
        if [ "$NAMESPACE_COUNT" -gt 1 ]; then
            echo "1. 🔧 Исправить разброс по namespace'ам:"
            echo "   - Запустить: ./scripts/fix-namespaces.sh"
            echo "   - Затем переразвернуть: ./scripts/deploy-complete.sh"
        fi
        
        # Check if APIService is working
        if ! kubectl api-resources --api-group=netguard.sgroups.io &>/dev/null; then
            echo "2. ⚡ API не работает. Рекомендуется:"
            echo "   - Проверить логи: kubectl logs -n netguard-system deployment/netguard-apiserver"
            echo "   - Переразвернуть: ./scripts/deploy-complete.sh"
        fi
        
        # General testing recommendation
        echo "3. 🧪 Запустить тестирование:"
        echo "   - ./scripts/test-complete.sh"
    fi
    
    echo -e "\n📚 Дополнительные команды для диагностики:"
    echo "   kubectl get all -A | grep netguard"
    echo "   kubectl describe apiservice v1beta1.netguard.sgroups.io"
    echo "   kubectl logs -n netguard-system deployment/netguard-apiserver"
    echo "   kubectl logs -n netguard-system deployment/netguard-backend"
}

# Main function
main() {
    echo "🔍 Анализ текущего состояния Netguard в Kubernetes"
    echo "=================================================="
    echo "Время анализа: $(date)"
    echo ""
    
    check_minikube
    analyze_namespaces
    analyze_workloads
    analyze_services
    analyze_api_resources
    analyze_config_issues
    show_resource_usage
    generate_recommendations
    
    echo -e "\n✅ Анализ завершен!"
}

# Handle script arguments
case "${1:-analyze}" in
    analyze)
        main
        ;;
    namespaces)
        analyze_namespaces
        ;;
    workloads)
        analyze_workloads
        ;;
    services)
        analyze_services
        ;;
    api)
        analyze_api_resources
        ;;
    config)
        analyze_config_issues
        ;;
    recommendations)
        generate_recommendations
        ;;
    help|*)
        echo "Usage: $0 [analyze|namespaces|workloads|services|api|config|recommendations|help]"
        echo ""
        echo "Commands:"
        echo "  analyze        - Полный анализ (по умолчанию)"
        echo "  namespaces     - Анализ только namespace'ов"
        echo "  workloads      - Анализ только deployments и подов"
        echo "  services       - Анализ только сервисов"
        echo "  api            - Анализ только API ресурсов"
        echo "  config         - Анализ только проблем конфигурации"
        echo "  recommendations - Только рекомендации"
        echo "  help           - Показать эту справку"
        ;;
esac 