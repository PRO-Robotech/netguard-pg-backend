#!/bin/bash

# Complete testing script for Netguard Platform
# Комплексное тестирование netguard платформы

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
NAMESPACE="${NAMESPACE:-netguard-system}"

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

# Test counters
TESTS_TOTAL=0
TESTS_PASSED=0
TESTS_FAILED=0

run_test() {
    local test_name="$1"
    local test_command="$2"
    
    TESTS_TOTAL=$((TESTS_TOTAL + 1))
    
    log_info "🧪 Тест: $test_name"
    
    if eval "$test_command"; then
        log_success "✅ PASSED: $test_name"
        TESTS_PASSED=$((TESTS_PASSED + 1))
        return 0
    else
        log_error "❌ FAILED: $test_name"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        return 1
    fi
}

# Test 1: Check namespace exists
test_namespace() {
    kubectl get namespace "$NAMESPACE" &>/dev/null
}

# Test 2: Check deployments are ready
test_deployments() {
    local ready_deployments=0
    local total_deployments=0
    
    # Count deployments
    total_deployments=$(kubectl get deployments -n "$NAMESPACE" --no-headers | wc -l)
    
    if [ "$total_deployments" -eq 0 ]; then
        log_error "Нет развертываний в namespace $NAMESPACE"
        return 1
    fi
    
    # Check each deployment
    while IFS= read -r line; do
        local name=$(echo "$line" | awk '{print $1}')
        local ready=$(echo "$line" | awk '{print $2}' | cut -d'/' -f1)
        local desired=$(echo "$line" | awk '{print $2}' | cut -d'/' -f2)
        
        if [ "$ready" -eq "$desired" ] && [ "$desired" -gt 0 ]; then
            ready_deployments=$((ready_deployments + 1))
            log_info "  ✓ $name: $ready/$desired готов"
        else
            log_warning "  ⚠ $name: $ready/$desired не готов"
        fi
    done < <(kubectl get deployments -n "$NAMESPACE" --no-headers)
    
    [ "$ready_deployments" -eq "$total_deployments" ]
}

# Test 3: Check all pods are running
test_pods() {
    local running_pods=0
    local total_pods=0
    
    # Get pod statuses
    while IFS= read -r line; do
        local name=$(echo "$line" | awk '{print $1}')
        local status=$(echo "$line" | awk '{print $3}')
        
        total_pods=$((total_pods + 1))
        
        if [ "$status" = "Running" ]; then
            running_pods=$((running_pods + 1))
            log_info "  ✓ $name: $status"
        else
            log_warning "  ⚠ $name: $status"
        fi
    done < <(kubectl get pods -n "$NAMESPACE" --no-headers)
    
    if [ "$total_pods" -eq 0 ]; then
        log_error "Нет подов в namespace $NAMESPACE"
        return 1
    fi
    
    [ "$running_pods" -eq "$total_pods" ]
}

# Test 4: Check services are accessible
test_services() {
    local services_ok=0
    local total_services=0
    
    while IFS= read -r line; do
        local name=$(echo "$line" | awk '{print $1}')
        local type=$(echo "$line" | awk '{print $2}')
        local cluster_ip=$(echo "$line" | awk '{print $3}')
        
        total_services=$((total_services + 1))
        
        if [ "$cluster_ip" != "<none>" ] && [ "$cluster_ip" != "None" ]; then
            services_ok=$((services_ok + 1))
            log_info "  ✓ $name ($type): $cluster_ip"
        else
            log_warning "  ⚠ $name ($type): No ClusterIP"
        fi
    done < <(kubectl get services -n "$NAMESPACE" --no-headers)
    
    if [ "$total_services" -eq 0 ]; then
        log_error "Нет сервисов в namespace $NAMESPACE"
        return 1
    fi
    
    [ "$services_ok" -eq "$total_services" ]
}

# Test 5: Check APIService registration (Aggregation Layer v1beta1)
test_apiservice() {
    local apiservice_status
    apiservice_status=$(kubectl get apiservice v1beta1.netguard.sgroups.io -o jsonpath='{.status.conditions[?(@.type=="Available")].status}' 2>/dev/null || echo "NotFound")
    
    if [ "$apiservice_status" = "True" ]; then
        log_info "  ✓ v1beta1.netguard.sgroups.io (Aggregation Layer) доступен"
        return 0
    else
        log_warning "  ⚠ v1beta1.netguard.sgroups.io статус: $apiservice_status"
        
        # Check if CRD version exists instead
        if kubectl get crd | grep -q netguard; then
            log_info "  ℹ️ Найдена CRD реализация (v1alpha1), но тестируем Aggregation Layer (v1beta1)"
        fi
        return 1
    fi
}

# Test 6: Check API resources are discoverable (focus on v1beta1)
test_api_discovery() {
    if kubectl api-resources --api-group=netguard.sgroups.io &>/dev/null; then
        local total_resources
        local v1beta1_resources
        local v1alpha1_resources
        
        total_resources=$(kubectl api-resources --api-group=netguard.sgroups.io --no-headers | wc -l)
        v1beta1_resources=$(kubectl api-resources --api-group=netguard.sgroups.io --no-headers | grep -c "v1beta1" || echo "0")
        v1alpha1_resources=$(kubectl api-resources --api-group=netguard.sgroups.io --no-headers | grep -c "v1alpha1" || echo "0")
        
        log_info "  ✓ Обнаружено $total_resources API ресурсов (v1beta1: $v1beta1_resources, v1alpha1: $v1alpha1_resources)"
        
        if [ "$v1beta1_resources" -gt 0 ]; then
            log_info "  ✓ v1beta1 ресурсы (Aggregation Layer) доступны"
            return 0
        else
            log_warning "  ⚠ v1beta1 ресурсы (Aggregation Layer) не найдены"
            if [ "$v1alpha1_resources" -gt 0 ]; then
                log_info "  ℹ️ Найдены только v1alpha1 ресурсы (CRD реализация)"
            fi
            return 1
        fi
    else
        log_warning "  ⚠ API группа netguard.sgroups.io недоступна"
        return 1
    fi
}

# Test 7: Test CRUD operations
test_crud_operations() {
    local test_resource_name="test-crud-service-$(date +%s)"
    
    # Create (using v1beta1 for Aggregation Layer)
    log_info "  Создание тестового ресурса: $test_resource_name (v1beta1 - Aggregation Layer)"
    if ! cat <<EOF | kubectl apply -f - &>/dev/null
apiVersion: netguard.sgroups.io/v1beta1
kind: Service
metadata:
  name: $test_resource_name
  namespace: $NAMESPACE
spec:
  description: "CRUD test service for Aggregation Layer testing"
  ingressPorts:
  - protocol: TCP
    port: "80"
    description: "HTTP port for testing"
  - protocol: TCP
    port: "443"
    description: "HTTPS port for testing"
EOF
    then
        log_error "    ❌ Не удалось создать ресурс"
        return 1
    fi
    
    # Read (using explicit v1beta1 version)
    log_info "  Чтение созданного ресурса (v1beta1)"
    if ! kubectl get services.v1beta1.netguard.sgroups.io "$test_resource_name" -n "$NAMESPACE" &>/dev/null; then
        log_error "    ❌ Не удалось прочитать v1beta1 ресурс"
        return 1
    fi
    
    # Update (patch) - using v1beta1
    log_info "  Обновление ресурса (v1beta1)"
    if ! kubectl patch services.v1beta1.netguard.sgroups.io "$test_resource_name" -n "$NAMESPACE" --type=merge -p '{"spec":{"description":"Updated CRUD test service for Aggregation Layer"}}' &>/dev/null; then
        log_error "    ❌ Не удалось обновить v1beta1 ресурс"
        return 1
    fi
    
    # Verify update
    local updated_desc
    updated_desc=$(kubectl get services.v1beta1.netguard.sgroups.io "$test_resource_name" -n "$NAMESPACE" -o jsonpath='{.spec.description}' 2>/dev/null || echo "")
    if [ "$updated_desc" != "Updated CRUD test service for Aggregation Layer" ]; then
        log_error "    ❌ Обновление не применилось"
        return 1
    fi
    
    # Delete (using v1beta1)
    log_info "  Удаление тестового ресурса (v1beta1)"
    if ! kubectl delete services.v1beta1.netguard.sgroups.io "$test_resource_name" -n "$NAMESPACE" &>/dev/null; then
        log_error "    ❌ Не удалось удалить v1beta1 ресурс"
        return 1
    fi
    
    # Verify deletion
    if kubectl get services.v1beta1.netguard.sgroups.io "$test_resource_name" -n "$NAMESPACE" &>/dev/null; then
        log_error "    ❌ v1beta1 ресурс не был удален"
        return 1
    fi
    
    log_info "  ✓ Все CRUD операции выполнены успешно"
    return 0
}

# Test 8: Check backend connectivity
test_backend_connectivity() {
    local backend_pod
    backend_pod=$(kubectl get pods -n "$NAMESPACE" -l app=netguard-backend --no-headers | head -1 | awk '{print $1}')
    
    if [ -z "$backend_pod" ]; then
        log_error "  ❌ Backend под не найден"
        return 1
    fi
    
    # Test gRPC port
    if kubectl exec -n "$NAMESPACE" "$backend_pod" -- nc -z localhost 9090 &>/dev/null; then
        log_info "  ✓ Backend gRPC порт (9090) доступен"
    else
        log_warning "  ⚠ Backend gRPC порт недоступен"
        return 1
    fi
    
    # Test HTTP port
    if kubectl exec -n "$NAMESPACE" "$backend_pod" -- nc -z localhost 8080 &>/dev/null; then
        log_info "  ✓ Backend HTTP порт (8080) доступен"
    else
        log_warning "  ⚠ Backend HTTP порт недоступен"
        return 1
    fi
    
    return 0
}

# Test 9: Check API server health endpoints
test_apiserver_health() {
    local apiserver_pod
    apiserver_pod=$(kubectl get pods -n "$NAMESPACE" -l app=netguard-apiserver --no-headers | head -1 | awk '{print $1}')
    
    if [ -z "$apiserver_pod" ]; then
        log_error "  ❌ API Server под не найден"
        return 1
    fi
    
    # Test health endpoints
    if kubectl exec -n "$NAMESPACE" "$apiserver_pod" -- wget -q -O- http://localhost:8080/healthz &>/dev/null; then
        log_info "  ✓ Health endpoint доступен"
    else
        log_warning "  ⚠ Health endpoint недоступен"
        return 1
    fi
    
    if kubectl exec -n "$NAMESPACE" "$apiserver_pod" -- wget -q -O- http://localhost:8080/readyz &>/dev/null; then
        log_info "  ✓ Readiness endpoint доступен"
    else
        log_warning "  ⚠ Readiness endpoint недоступен"
        return 1
    fi
    
    return 0
}

# Test 10: Check logs for errors
test_logs_for_errors() {
    local error_count=0
    
    # Check API server logs
    local apiserver_errors
    apiserver_errors=$(kubectl logs -n "$NAMESPACE" deployment/netguard-apiserver --tail=100 | grep -i error | wc -l)
    
    if [ "$apiserver_errors" -gt 0 ]; then
        log_warning "  ⚠ Найдено $apiserver_errors ошибок в логах API Server"
        error_count=$((error_count + apiserver_errors))
    else
        log_info "  ✓ Нет ошибок в логах API Server"
    fi
    
    # Check backend logs
    local backend_errors
    backend_errors=$(kubectl logs -n "$NAMESPACE" deployment/netguard-backend --tail=100 | grep -i error | wc -l)
    
    if [ "$backend_errors" -gt 0 ]; then
        log_warning "  ⚠ Найдено $backend_errors ошибок в логах Backend"
        error_count=$((error_count + backend_errors))
    else
        log_info "  ✓ Нет ошибок в логах Backend"
    fi
    
    # Allow some errors but not too many
    [ "$error_count" -lt 5 ]
}

# Show detailed status
show_detailed_status() {
    echo -e "\n📊 Детальный статус системы:"
    echo "============================"
    
    echo -e "\n🏗️ Развертывания:"
    kubectl get deployments -n "$NAMESPACE" -o wide
    
    echo -e "\n🟢 Поды:"
    kubectl get pods -n "$NAMESPACE" -o wide
    
    echo -e "\n🔌 Сервисы:"
    kubectl get services -n "$NAMESPACE" -o wide
    
    echo -e "\n🔗 APIService:"
    kubectl get apiservice v1beta1.netguard.sgroups.io -o wide
    
    echo -e "\n🎯 Доступные API ресурсы:"
    kubectl api-resources --api-group=netguard.sgroups.io 2>/dev/null || echo "API группа недоступна"
    
    echo -e "\n📈 Использование ресурсов:"
    kubectl top pods -n "$NAMESPACE" 2>/dev/null || echo "Metrics недоступны"
}

# Show recent logs
show_recent_logs() {
    echo -e "\n📝 Последние логи:"
    echo "=================="
    
    echo -e "\n🔸 API Server (последние 10 строк):"
    kubectl logs -n "$NAMESPACE" deployment/netguard-apiserver --tail=10 || echo "Логи недоступны"
    
    echo -e "\n🔸 Backend (последние 10 строк):"
    kubectl logs -n "$NAMESPACE" deployment/netguard-backend --tail=10 || echo "Логи недоступны"
}

# Main testing function
run_all_tests() {
    echo "🧪 Комплексное тестирование Netguard Platform"
    echo "============================================="
    echo "Namespace: $NAMESPACE"
    echo ""
    
    # Basic infrastructure tests
    run_test "Namespace существует" "test_namespace"
    run_test "Развертывания готовы" "test_deployments"
    run_test "Поды запущены" "test_pods"
    run_test "Сервисы доступны" "test_services"
    
    # API tests
    run_test "APIService зарегистрирован" "test_apiservice"
    run_test "API ресурсы обнаруживаются" "test_api_discovery"
    run_test "CRUD операции работают" "test_crud_operations"
    
    # Connectivity tests
    run_test "Backend доступен" "test_backend_connectivity"
    run_test "API Server health endpoints" "test_apiserver_health"
    
    # Quality tests
    run_test "Логи без критических ошибок" "test_logs_for_errors"
    
    echo ""
    echo "📊 Результаты тестирования:"
    echo "=========================="
    echo "Общее количество тестов: $TESTS_TOTAL"
    echo "Прошло успешно: $TESTS_PASSED"
    echo "Провалилось: $TESTS_FAILED"
    
    if [ "$TESTS_FAILED" -eq 0 ]; then
        log_success "🎉 Все тесты прошли успешно!"
        echo ""
        echo "✅ Netguard Platform полностью функциональна и готова к использованию"
        return 0
    else
        log_warning "⚠️ Некоторые тесты провалились ($TESTS_FAILED из $TESTS_TOTAL)"
        echo ""
        echo "🔧 Рекомендуется проверить логи и конфигурацию"
        return 1
    fi
}

# Performance test
run_performance_test() {
    log_info "🚀 Запуск нагрузочного теста..."
    
    local start_time=$(date +%s)
    local operations=10
    local successful_ops=0
    
    for i in $(seq 1 $operations); do
        local resource_name="perf-test-$i"
        
        if cat <<EOF | kubectl apply -f - &>/dev/null
apiVersion: netguard.sgroups.io/v1beta1
kind: Service
metadata:
  name: $resource_name
  namespace: $NAMESPACE
spec:
  description: "Performance test service $i (Aggregation Layer v1beta1)"
  ingressPorts:
  - protocol: TCP
    port: "$(($i + 8000))"
    description: "Test port $i"
EOF
        then
            if kubectl delete services.v1beta1.netguard.sgroups.io "$resource_name" -n "$NAMESPACE" &>/dev/null; then
                successful_ops=$((successful_ops + 1))
            fi
        fi
    done
    
    local end_time=$(date +%s)
    local duration=$((end_time - start_time))
    
    echo "📈 Результаты нагрузочного теста:"
    echo "  Операций: $operations"
    echo "  Успешных: $successful_ops"
    echo "  Время: ${duration}s"
    echo "  Скорость: $(echo "scale=2; $successful_ops / $duration" | bc) ops/sec"
}

# Main script logic
case "${1:-all}" in
    all)
        run_all_tests
        echo ""
        show_detailed_status
        ;;
    quick)
        run_test "Namespace существует" "test_namespace"
        run_test "Развертывания готовы" "test_deployments"
        run_test "Поды запущены" "test_pods"
        run_test "CRUD операции работают" "test_crud_operations"
        ;;
    performance|perf)
        run_performance_test
        ;;
    status)
        show_detailed_status
        ;;
    logs)
        show_recent_logs
        ;;
    help|*)
        echo "Usage: $0 [all|quick|performance|status|logs|help]"
        echo ""
        echo "Commands:"
        echo "  all         - Полное тестирование (по умолчанию)"
        echo "  quick       - Быстрая проверка основных функций"
        echo "  performance - Нагрузочное тестирование"
        echo "  status      - Детальный статус системы"
        echo "  logs        - Показать последние логи"
        echo "  help        - Показать эту справку"
        ;;
esac 