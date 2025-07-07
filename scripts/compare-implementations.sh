#!/bin/bash

# Script to compare CRD (v1alpha1) and Aggregation Layer (v1beta1) implementations
# Сравнение CRD (v1alpha1) и Aggregation Layer (v1beta1) реализаций

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
MAGENTA='\033[0;35m'
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

log_implementation() {
    echo -e "${MAGENTA}$1${NC}"
}

# Check implementation availability
check_implementations() {
    log_section "Проверка доступности реализаций"
    
    # Check Aggregation Layer (v1beta1)
    log_implementation "🔸 Aggregation Layer (v1beta1):"
    V1BETA1_APISERVICE=$(kubectl get apiservice v1beta1.netguard.sgroups.io 2>/dev/null || echo "")
    if [ -n "$V1BETA1_APISERVICE" ]; then
        V1BETA1_STATUS=$(kubectl get apiservice v1beta1.netguard.sgroups.io -o jsonpath='{.status.conditions[?(@.type=="Available")].status}' 2>/dev/null || echo "Unknown")
        if [ "$V1BETA1_STATUS" = "True" ]; then
            log_success "✅ APIService v1beta1.netguard.sgroups.io доступен"
            AGGREGATION_AVAILABLE=true
        else
            log_warning "⚠ APIService v1beta1.netguard.sgroups.io недоступен (статус: $V1BETA1_STATUS)"
            AGGREGATION_AVAILABLE=false
        fi
    else
        log_warning "❌ APIService v1beta1.netguard.sgroups.io не найден"
        AGGREGATION_AVAILABLE=false
    fi
    
    # Check CRD implementation (v1alpha1)
    log_implementation "🔸 CRD реализация (v1alpha1):"
    CRD_COUNT=$(kubectl get crd | grep -c netguard || echo "0")
    if [ "$CRD_COUNT" -gt 0 ]; then
        log_success "✅ Найдено $CRD_COUNT netguard CRD"
        CRD_AVAILABLE=true
        
        echo "  Найденные CRD:"
        kubectl get crd | grep netguard | while read -r line; do
            CRD_NAME=$(echo "$line" | awk '{print $1}')
            echo "    - $CRD_NAME"
        done
    else
        log_warning "❌ Netguard CRD не найдены"
        CRD_AVAILABLE=false
    fi
}

# Compare API resources
compare_api_resources() {
    log_section "Сравнение API ресурсов"
    
    if kubectl api-resources --api-group=netguard.sgroups.io &>/dev/null; then
        echo "📋 Доступные API ресурсы в группе netguard.sgroups.io:"
        kubectl api-resources --api-group=netguard.sgroups.io
        
        # Count resources by version
        V1ALPHA1_COUNT=$(kubectl api-resources --api-group=netguard.sgroups.io --no-headers | grep -c "v1alpha1" || echo "0")
        V1BETA1_COUNT=$(kubectl api-resources --api-group=netguard.sgroups.io --no-headers | grep -c "v1beta1" || echo "0")
        
        echo -e "\n📊 Статистика по версиям:"
        echo "  - v1alpha1 (CRD): $V1ALPHA1_COUNT ресурсов"
        echo "  - v1beta1 (Aggregation): $V1BETA1_COUNT ресурсов"
        
        # List resources by version
        if [ "$V1BETA1_COUNT" -gt 0 ]; then
            echo -e "\n🔸 v1beta1 ресурсы (Aggregation Layer):"
            kubectl api-resources --api-group=netguard.sgroups.io --no-headers | grep "v1beta1" | while read -r line; do
                RESOURCE_NAME=$(echo "$line" | awk '{print $1}')
                echo "    - $RESOURCE_NAME"
            done
        fi
        
        if [ "$V1ALPHA1_COUNT" -gt 0 ]; then
            echo -e "\n🔸 v1alpha1 ресурсы (CRD):"
            kubectl api-resources --api-group=netguard.sgroups.io --no-headers | grep "v1alpha1" | while read -r line; do
                RESOURCE_NAME=$(echo "$line" | awk '{print $1}')
                echo "    - $RESOURCE_NAME"
            done
        fi
    else
        log_warning "❌ API группа netguard.sgroups.io недоступна"
    fi
}

# Test CRUD operations for both versions
test_crud_comparison() {
    log_section "Тестирование CRUD операций"
    
    local test_timestamp=$(date +%s)
    local namespace="netguard-system"
    
    # Test v1beta1 (Aggregation Layer)
    if [ "$AGGREGATION_AVAILABLE" = true ]; then
        log_implementation "🔸 Тестирование v1beta1 (Aggregation Layer):"
        test_v1beta1_crud "$test_timestamp" "$namespace"
    else
        log_warning "⚠ v1beta1 недоступен для тестирования"
    fi
    
    # Test v1alpha1 (CRD)
    if [ "$CRD_AVAILABLE" = true ]; then
        log_implementation "🔸 Тестирование v1alpha1 (CRD):"
        test_v1alpha1_crud "$test_timestamp" "$namespace"
    else
        log_warning "⚠ v1alpha1 недоступен для тестирования"
    fi
}

# Test v1beta1 CRUD
test_v1beta1_crud() {
    local timestamp="$1"
    local namespace="$2"
    local resource_name="test-v1beta1-$timestamp"
    
    # Create
    if cat <<EOF | kubectl apply -f - &>/dev/null
apiVersion: netguard.sgroups.io/v1beta1
kind: Service
metadata:
  name: $resource_name
  namespace: $namespace
spec:
  description: "Test service for v1beta1 (Aggregation Layer)"
  ingressPorts:
  - protocol: TCP
    port: "8080"
    description: "Test port"
EOF
    then
        log_success "  ✅ CREATE: v1beta1 ресурс создан"
        
        # Read
        if kubectl get services.v1beta1.netguard.sgroups.io "$resource_name" -n "$namespace" &>/dev/null; then
            log_success "  ✅ READ: v1beta1 ресурс прочитан"
            
            # Update
            if kubectl patch services.v1beta1.netguard.sgroups.io "$resource_name" -n "$namespace" --type=merge -p '{"spec":{"description":"Updated v1beta1 service"}}' &>/dev/null; then
                log_success "  ✅ UPDATE: v1beta1 ресурс обновлен"
                
                # Delete
                if kubectl delete services.v1beta1.netguard.sgroups.io "$resource_name" -n "$namespace" &>/dev/null; then
                    log_success "  ✅ DELETE: v1beta1 ресурс удален"
                else
                    log_error "  ❌ DELETE: Не удалось удалить v1beta1 ресурс"
                fi
            else
                log_error "  ❌ UPDATE: Не удалось обновить v1beta1 ресурс"
            fi
        else
            log_error "  ❌ READ: Не удалось прочитать v1beta1 ресурс"
        fi
    else
        log_error "  ❌ CREATE: Не удалось создать v1beta1 ресурс"
    fi
}

# Test v1alpha1 CRUD
test_v1alpha1_crud() {
    local timestamp="$1"
    local namespace="$2"
    local resource_name="test-v1alpha1-$timestamp"
    
    # Create
    if cat <<EOF | kubectl apply -f - &>/dev/null
apiVersion: netguard.sgroups.io/v1alpha1
kind: Service
metadata:
  name: $resource_name
  namespace: $namespace
spec:
  description: "Test service for v1alpha1 (CRD)"
  ingressPorts:
  - protocol: TCP
    port: "8080"
    description: "Test port"
EOF
    then
        log_success "  ✅ CREATE: v1alpha1 ресурс создан"
        
        # Read
        if kubectl get services.v1alpha1.netguard.sgroups.io "$resource_name" -n "$namespace" &>/dev/null; then
            log_success "  ✅ READ: v1alpha1 ресурс прочитан"
            
            # Update
            if kubectl patch services.v1alpha1.netguard.sgroups.io "$resource_name" -n "$namespace" --type=merge -p '{"spec":{"description":"Updated v1alpha1 service"}}' &>/dev/null; then
                log_success "  ✅ UPDATE: v1alpha1 ресурс обновлен"
                
                # Delete
                if kubectl delete services.v1alpha1.netguard.sgroups.io "$resource_name" -n "$namespace" &>/dev/null; then
                    log_success "  ✅ DELETE: v1alpha1 ресурс удален"
                else
                    log_error "  ❌ DELETE: Не удалось удалить v1alpha1 ресурс"
                fi
            else
                log_error "  ❌ UPDATE: Не удалось обновить v1alpha1 ресурс"
            fi
        else
            log_error "  ❌ READ: Не удалось прочитать v1alpha1 ресурс"
        fi
    else
        log_error "  ❌ CREATE: Не удалось создать v1alpha1 ресурс"
    fi
}

# Show existing resources for both versions
show_existing_resources() {
    log_section "Существующие ресурсы"
    
    # v1beta1 resources
    if [ "$AGGREGATION_AVAILABLE" = true ]; then
        log_implementation "🔸 v1beta1 ресурсы (Aggregation Layer):"
        
        local v1beta1_services=$(kubectl get services.v1beta1.netguard.sgroups.io -A 2>/dev/null | tail -n +2 | wc -l || echo "0")
        if [ "$v1beta1_services" -gt 0 ]; then
            echo "  📦 Services ($v1beta1_services):"
            kubectl get services.v1beta1.netguard.sgroups.io -A | head -6
        else
            echo "  📦 Services: нет"
        fi
        
        local v1beta1_addressgroups=$(kubectl get addressgroups.v1beta1.netguard.sgroups.io -A 2>/dev/null | tail -n +2 | wc -l || echo "0")
        if [ "$v1beta1_addressgroups" -gt 0 ]; then
            echo "  📦 AddressGroups ($v1beta1_addressgroups):"
            kubectl get addressgroups.v1beta1.netguard.sgroups.io -A | head -4
        else
            echo "  📦 AddressGroups: нет"
        fi
    fi
    
    # v1alpha1 resources
    if [ "$CRD_AVAILABLE" = true ]; then
        log_implementation "🔸 v1alpha1 ресурсы (CRD):"
        
        local v1alpha1_services=$(kubectl get services.v1alpha1.netguard.sgroups.io -A 2>/dev/null | tail -n +2 | wc -l || echo "0")
        if [ "$v1alpha1_services" -gt 0 ]; then
            echo "  📦 Services ($v1alpha1_services):"
            kubectl get services.v1alpha1.netguard.sgroups.io -A | head -6
        else
            echo "  📦 Services: нет"
        fi
        
        local v1alpha1_addressgroups=$(kubectl get addressgroups.v1alpha1.netguard.sgroups.io -A 2>/dev/null | tail -n +2 | wc -l || echo "0")
        if [ "$v1alpha1_addressgroups" -gt 0 ]; then
            echo "  📦 AddressGroups ($v1alpha1_addressgroups):"
            kubectl get addressgroups.v1alpha1.netguard.sgroups.io -A | head -4
        else
            echo "  📦 AddressGroups: нет"
        fi
    fi
}

# Performance comparison
performance_comparison() {
    log_section "Сравнение производительности"
    
    if [ "$AGGREGATION_AVAILABLE" = true ] && [ "$CRD_AVAILABLE" = true ]; then
        log_info "Запуск быстрого теста производительности..."
        
        # Test v1beta1 performance
        log_implementation "🔸 v1beta1 (Aggregation Layer):"
        test_performance_v1beta1
        
        # Test v1alpha1 performance
        log_implementation "🔸 v1alpha1 (CRD):"
        test_performance_v1alpha1
    else
        log_warning "⚠ Для сравнения производительности нужны обе реализации"
    fi
}

test_performance_v1beta1() {
    local start_time=$(date +%s%N)
    local operations=5
    local successful=0
    
    for i in $(seq 1 $operations); do
        if cat <<EOF | kubectl apply -f - &>/dev/null
apiVersion: netguard.sgroups.io/v1beta1
kind: Service
metadata:
  name: perf-v1beta1-$i
  namespace: netguard-system
spec:
  description: "Performance test v1beta1 $i"
  ingressPorts:
  - protocol: TCP
    port: "$((8000 + i))"
EOF
        then
            if kubectl delete services.v1beta1.netguard.sgroups.io "perf-v1beta1-$i" -n netguard-system &>/dev/null; then
                successful=$((successful + 1))
            fi
        fi
    done
    
    local end_time=$(date +%s%N)
    local duration_ms=$(( (end_time - start_time) / 1000000 ))
    
    echo "  📊 Результат: $successful/$operations операций за ${duration_ms}ms"
}

test_performance_v1alpha1() {
    local start_time=$(date +%s%N)
    local operations=5
    local successful=0
    
    for i in $(seq 1 $operations); do
        if cat <<EOF | kubectl apply -f - &>/dev/null
apiVersion: netguard.sgroups.io/v1alpha1
kind: Service
metadata:
  name: perf-v1alpha1-$i
  namespace: netguard-system
spec:
  description: "Performance test v1alpha1 $i"
  ingressPorts:
  - protocol: TCP
    port: "$((8000 + i))"
EOF
        then
            if kubectl delete services.v1alpha1.netguard.sgroups.io "perf-v1alpha1-$i" -n netguard-system &>/dev/null; then
                successful=$((successful + 1))
            fi
        fi
    done
    
    local end_time=$(date +%s%N)
    local duration_ms=$(( (end_time - start_time) / 1000000 ))
    
    echo "  📊 Результат: $successful/$operations операций за ${duration_ms}ms"
}

# Generate recommendations
generate_recommendations() {
    log_section "Рекомендации"
    
    echo "💡 Рекомендации на основе анализа:"
    
    if [ "$AGGREGATION_AVAILABLE" = true ] && [ "$CRD_AVAILABLE" = true ]; then
        log_warning "⚠ Обнаружены ОБЕ реализации одновременно"
        echo "1. 🔧 Рекомендуется использовать только одну реализацию"
        echo "2. 🎯 Для тестирования Aggregation Layer фокусируйтесь на v1beta1"
        echo "3. 🧹 Рассмотрите удаление неиспользуемой реализации"
        echo ""
        echo "📚 Различия реализаций:"
        echo "  - v1alpha1 (CRD): Традиционная K8s реализация через CustomResourceDefinitions"
        echo "  - v1beta1 (Aggregation): Расширение API Server через Aggregation Layer"
        echo ""
        echo "🔧 Команды для очистки:"
        echo "  # Удалить CRD реализацию:"
        echo "  kubectl delete crd \$(kubectl get crd | grep netguard | awk '{print \$1}')"
        echo "  # Удалить Aggregation Layer:"
        echo "  kubectl delete apiservice v1beta1.netguard.sgroups.io"
        
    elif [ "$AGGREGATION_AVAILABLE" = true ]; then
        log_success "✅ Активна только Aggregation Layer реализация (v1beta1)"
        echo "1. 🎯 Используйте скрипты с фокусом на v1beta1"
        echo "2. 🧪 Запустите: ./test-complete.sh для полного тестирования"
        
    elif [ "$CRD_AVAILABLE" = true ]; then
        log_info "ℹ️ Активна только CRD реализация (v1alpha1)"
        echo "1. ⚡ Для тестирования Aggregation Layer развертывайте v1beta1"
        echo "2. 🚀 Запустите: ./deploy-complete.sh для развертывания Aggregation Layer"
        
    else
        log_warning "❌ Netguard реализации не найдены"
        echo "1. 🚀 Развертывайте Aggregation Layer: ./deploy-complete.sh"
        echo "2. 🔧 Проверьте конфигурацию: ./analyze-current-state.sh"
    fi
    
    echo ""
    echo "📝 Полезные команды:"
    echo "  ./analyze-current-state.sh     # Анализ текущего состояния"
    echo "  ./deploy-complete.sh           # Развертывание Aggregation Layer"
    echo "  ./test-complete.sh             # Тестирование v1beta1"
    echo "  ./compare-implementations.sh   # Это сравнение (повторно)"
}

# Main function
main() {
    echo "🔍 Сравнение реализаций Netguard"
    echo "================================"
    echo "v1alpha1 (CRD) vs v1beta1 (Aggregation Layer)"
    echo "Время анализа: $(date)"
    echo ""
    
    check_implementations
    compare_api_resources
    show_existing_resources
    test_crud_comparison
    performance_comparison
    generate_recommendations
    
    echo -e "\n✅ Сравнение завершено!"
}

# Handle script arguments
case "${1:-compare}" in
    compare)
        main
        ;;
    check)
        check_implementations
        ;;
    api)
        compare_api_resources
        ;;
    resources)
        show_existing_resources
        ;;
    crud)
        check_implementations
        test_crud_comparison
        ;;
    performance)
        check_implementations
        performance_comparison
        ;;
    recommendations)
        check_implementations
        generate_recommendations
        ;;
    help|*)
        echo "Usage: $0 [compare|check|api|resources|crud|performance|recommendations|help]"
        echo ""
        echo "Commands:"
        echo "  compare        - Полное сравнение (по умолчанию)"
        echo "  check          - Только проверка доступности реализаций"
        echo "  api            - Только сравнение API ресурсов"
        echo "  resources      - Только показ существующих ресурсов"
        echo "  crud           - Только тестирование CRUD операций"
        echo "  performance    - Только сравнение производительности"
        echo "  recommendations - Только рекомендации"
        echo "  help           - Показать эту справку"
        ;;
esac 