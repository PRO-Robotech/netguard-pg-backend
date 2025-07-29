# gRPC контракт и протоколы Netguard PG Backend

## Обзор gRPC контракта

gRPC контракт определяет протокол взаимодействия между Aggregated API Server и Backend Service. Он обеспечивает типобезопасность, эффективную сериализацию и поддержку потоковой передачи данных.

## Определение сервиса

### Основной сервис NetguardService

```protobuf
syntax = "proto3";
package netguard.v1;
option go_package = "netguard-pg-backend/protos/pkg/api/netguard;netguard";

import "google/protobuf/empty.proto";
import "google/protobuf/timestamp.proto";
import "google/api/annotations.proto";

service NetguardService {
  // CRUD операции для Service
  rpc CreateService(CreateServiceRequest) returns (CreateServiceResponse);
  rpc GetService(GetServiceRequest) returns (GetServiceResponse);
  rpc UpdateService(UpdateServiceRequest) returns (UpdateServiceResponse);
  rpc DeleteService(DeleteServiceRequest) returns (DeleteServiceResponse);
  rpc ListServices(ListServicesRequest) returns (ListServicesResponse);
  
  // CRUD операции для AddressGroup
  rpc CreateAddressGroup(CreateAddressGroupRequest) returns (CreateAddressGroupResponse);
  rpc GetAddressGroup(GetAddressGroupRequest) returns (GetAddressGroupResponse);
  rpc UpdateAddressGroup(UpdateAddressGroupRequest) returns (UpdateAddressGroupResponse);
  rpc DeleteAddressGroup(DeleteAddressGroupRequest) returns (DeleteAddressGroupResponse);
  rpc ListAddressGroups(ListAddressGroupsRequest) returns (ListAddressGroupsResponse);
  
  // CRUD операции для AddressGroupBinding
  rpc CreateAddressGroupBinding(CreateAddressGroupBindingRequest) returns (CreateAddressGroupBindingResponse);
  rpc GetAddressGroupBinding(GetAddressGroupBindingRequest) returns (GetAddressGroupBindingResponse);
  rpc UpdateAddressGroupBinding(UpdateAddressGroupBindingRequest) returns (UpdateAddressGroupBindingResponse);
  rpc DeleteAddressGroupBinding(DeleteAddressGroupBindingRequest) returns (DeleteAddressGroupBindingResponse);
  rpc ListAddressGroupBindings(ListAddressGroupBindingsRequest) returns (ListAddressGroupBindingsResponse);
  
  // CRUD операции для AddressGroupPortMapping
  rpc CreateAddressGroupPortMapping(CreateAddressGroupPortMappingRequest) returns (CreateAddressGroupPortMappingResponse);
  rpc GetAddressGroupPortMapping(GetAddressGroupPortMappingRequest) returns (GetAddressGroupPortMappingResponse);
  rpc UpdateAddressGroupPortMapping(UpdateAddressGroupPortMappingRequest) returns (UpdateAddressGroupPortMappingResponse);
  rpc DeleteAddressGroupPortMapping(DeleteAddressGroupPortMappingRequest) returns (DeleteAddressGroupPortMappingResponse);
  rpc ListAddressGroupPortMappings(ListAddressGroupPortMappingsRequest) returns (ListAddressGroupPortMappingsResponse);
  
  // CRUD операции для RuleS2S
  rpc CreateRuleS2S(CreateRuleS2SRequest) returns (CreateRuleS2SResponse);
  rpc GetRuleS2S(GetRuleS2SRequest) returns (GetRuleS2SResponse);
  rpc UpdateRuleS2S(UpdateRuleS2SRequest) returns (UpdateRuleS2SResponse);
  rpc DeleteRuleS2S(DeleteRuleS2SRequest) returns (DeleteRuleS2SResponse);
  rpc ListRuleS2S(ListRuleS2SRequest) returns (ListRuleS2SResponse);
  
  // CRUD операции для ServiceAlias
  rpc CreateServiceAlias(CreateServiceAliasRequest) returns (CreateServiceAliasResponse);
  rpc GetServiceAlias(GetServiceAliasRequest) returns (GetServiceAliasResponse);
  rpc UpdateServiceAlias(UpdateServiceAliasRequest) returns (UpdateServiceAliasResponse);
  rpc DeleteServiceAlias(DeleteServiceAliasRequest) returns (DeleteServiceAliasResponse);
  rpc ListServiceAliases(ListServiceAliasesRequest) returns (ListServiceAliasesResponse);
  
  // CRUD операции для AddressGroupBindingPolicy
  rpc CreateAddressGroupBindingPolicy(CreateAddressGroupBindingPolicyRequest) returns (CreateAddressGroupBindingPolicyResponse);
  rpc GetAddressGroupBindingPolicy(GetAddressGroupBindingPolicyRequest) returns (GetAddressGroupBindingPolicyResponse);
  rpc UpdateAddressGroupBindingPolicy(UpdateAddressGroupBindingPolicyRequest) returns (UpdateAddressGroupBindingPolicyResponse);
  rpc DeleteAddressGroupBindingPolicy(DeleteAddressGroupBindingPolicyRequest) returns (DeleteAddressGroupBindingPolicyResponse);
  rpc ListAddressGroupBindingPolicies(ListAddressGroupBindingPoliciesRequest) returns (ListAddressGroupBindingPoliciesResponse);
  
  // CRUD операции для IEAgAgRule
  rpc CreateIEAgAgRule(CreateIEAgAgRuleRequest) returns (CreateIEAgAgRuleResponse);
  rpc GetIEAgAgRule(GetIEAgAgRuleRequest) returns (GetIEAgAgRuleResponse);
  rpc UpdateIEAgAgRule(UpdateIEAgAgRuleRequest) returns (UpdateIEAgAgRuleResponse);
  rpc DeleteIEAgAgRule(DeleteIEAgAgRuleRequest) returns (DeleteIEAgAgRuleResponse);
  rpc ListIEAgAgRules(ListIEAgAgRulesRequest) returns (ListIEAgAgRulesResponse);
  
  // Синхронизация
  rpc Sync(SyncRequest) returns (google.protobuf.Empty);
  rpc SyncStatus(google.protobuf.Empty) returns (SyncStatusResponse);
  
  // Health Check
  rpc HealthCheck(google.protobuf.Empty) returns (HealthCheckResponse);
}
```

## Типы сообщений

### Базовые типы

```protobuf
// Идентификатор ресурса
message ResourceIdentifier {
  string name = 1;
  string namespace = 2;
}

// Метаданные ресурса
message Meta {
  string uid = 1;
  string resource_version = 2;
  int64 generation = 3;
  google.protobuf.Timestamp creation_ts = 4;
  map<string,string> labels = 5;
  map<string,string> annotations = 6;
  repeated Condition conditions = 7;
  int64 observed_generation = 8;
}

// Условие Kubernetes
message Condition {
  string type = 1;
  string status = 2;
  int64 observed_generation = 3;
  google.protobuf.Timestamp last_transition_time = 4;
  string reason = 5;
  string message = 6;
}

// Ошибка
message Error {
  string code = 1;
  string message = 2;
  string details = 3;
  repeated string field_errors = 4;
}

// Код ошибки
enum ErrorCode {
  UNKNOWN = 0;
  VALIDATION_ERROR = 1;
  NOT_FOUND = 2;
  ALREADY_EXISTS = 3;
  CONFLICT = 4;
  INTERNAL_ERROR = 5;
  PERMISSION_DENIED = 6;
  INVALID_ARGUMENT = 7;
}
```

### Доменные сущности

```protobuf
// Сервис
message Service {
  ResourceIdentifier self_ref = 1;
  string description = 3;
  repeated IngressPort ingress_ports = 4;
  repeated AddressGroupRef address_groups = 5;
  Meta meta = 6;
}

// Входящий порт
message IngressPort {
  Networks.NetIP.Transport protocol = 1;
  string port = 2;
  string description = 3;
}

// Группа адресов
message AddressGroup {
  ResourceIdentifier self_ref = 1;
  repeated NetworkItem networks = 2;
  RuleAction action = 3;
  string description = 4;
  Meta meta = 5;
}

// Элемент сети
message NetworkItem {
  string cidr = 1;
  string description = 2;
}

// Привязка группы адресов
message AddressGroupBinding {
  ResourceIdentifier self_ref = 1;
  ServiceRef service_ref = 2;
  AddressGroupRef address_group_ref = 3;
  Meta meta = 4;
}

// Маппинг портов группы адресов
message AddressGroupPortMapping {
  ResourceIdentifier self_ref = 1;
  repeated ProtocolPorts access_ports = 2;
  Meta meta = 3;
}

// Протокол и порты
message ProtocolPorts {
  Networks.NetIP.Transport protocol = 1;
  repeated PortRange ports = 2;
}

// Диапазон портов
message PortRange {
  string from = 1;
  string to = 2;
}

// Правило S2S
message RuleS2S {
  ResourceIdentifier self_ref = 1;
  Traffic traffic = 2;
  ServiceRef service_local_ref = 3;
  ServiceRef service_ref = 4;
  Meta meta = 5;
}

// Алиас сервиса
message ServiceAlias {
  ResourceIdentifier self_ref = 1;
  ServiceRef service_ref = 2;
  Meta meta = 3;
}

// Политика привязки группы адресов
message AddressGroupBindingPolicy {
  ResourceIdentifier self_ref = 1;
  ServiceRef service_ref = 2;
  AddressGroupRef address_group_ref = 3;
  Meta meta = 4;
}

// Правило IEAgAg
message IEAgAgRule {
  ResourceIdentifier self_ref = 1;
  Traffic traffic = 2;
  ServiceRef service_local_ref = 3;
  AddressGroupRef address_group_ref = 4;
  Meta meta = 5;
}

// Ссылки
message ServiceRef {
  ResourceIdentifier identifier = 1;
}

message AddressGroupRef {
  ResourceIdentifier identifier = 1;
}

// Перечисления
enum Traffic {
  Ingress = 0;
  Egress = 1;
}

enum RuleAction {
  UNDEFINED = 0;
  ACCEPT = 1;
  DROP = 2;
}
```

## Запросы и ответы

### CRUD операции для Service

```protobuf
// Создание Service
message CreateServiceRequest {
  Service service = 1;
}

message CreateServiceResponse {
  Service service = 1;
  Error error = 2;
}

// Получение Service
message GetServiceRequest {
  string name = 1;
  string namespace = 2;
}

message GetServiceResponse {
  Service service = 1;
  Error error = 2;
}

// Обновление Service
message UpdateServiceRequest {
  Service service = 1;
}

message UpdateServiceResponse {
  Service service = 1;
  Error error = 2;
}

// Удаление Service
message DeleteServiceRequest {
  string name = 1;
  string namespace = 2;
}

message DeleteServiceResponse {
  Error error = 1;
}

// Список Service
message ListServicesRequest {
  string namespace = 1;
  map<string,string> labels = 2;
  int32 limit = 3;
  string continue_token = 4;
}

message ListServicesResponse {
  repeated Service services = 1;
  string continue_token = 2;
  Error error = 3;
}
```

### Синхронизация

```protobuf
// Запрос синхронизации
message SyncRequest {
  SyncOp operation = 1;
  oneof sync_data {
    SyncServices services = 2;
    SyncAddressGroups address_groups = 3;
    SyncAddressGroupBindings address_group_bindings = 4;
    SyncAddressGroupPortMappings address_group_port_mappings = 5;
    SyncRuleS2S rules_s2s = 6;
    SyncServiceAliases service_aliases = 7;
    SyncAddressGroupBindingPolicies address_group_binding_policies = 8;
    SyncIEAgAgRules ieagag_rules = 9;
  }
}

// Операция синхронизации
enum SyncOp {
  CREATE = 0;
  UPDATE = 1;
  DELETE = 2;
}

// Данные для синхронизации
message SyncServices {
  repeated Service services = 1;
}

message SyncAddressGroups {
  repeated AddressGroup address_groups = 1;
}

message SyncAddressGroupBindings {
  repeated AddressGroupBinding address_group_bindings = 1;
}

message SyncAddressGroupPortMappings {
  repeated AddressGroupPortMapping address_group_port_mappings = 1;
}

message SyncRuleS2S {
  repeated RuleS2S rules_s2s = 1;
}

message SyncServiceAliases {
  repeated ServiceAlias service_aliases = 1;
}

message SyncAddressGroupBindingPolicies {
  repeated AddressGroupBindingPolicy address_group_binding_policies = 1;
}

message SyncIEAgAgRules {
  repeated IEAgAgRule ieagag_rules = 1;
}

// Статус синхронизации
message SyncStatusResponse {
  bool is_syncing = 1;
  int32 pending_operations = 2;
  google.protobuf.Timestamp last_sync_time = 3;
  repeated string errors = 4;
}
```

### Health Check

```protobuf
message HealthCheckResponse {
  string status = 1;  // "healthy", "unhealthy", "degraded"
  map<string,string> details = 2;
  repeated string errors = 3;
}
```

## Схема взаимодействия

```plantuml
@startuml
!theme plain
skinparam backgroundColor #FFFFFF
skinparam sequenceArrowThickness 2
skinparam roundcorner 20
skinparam maxmessagesize 60

participant Client as gRPC Client
participant Server as gRPC Server
participant BL as Business Logic
participant REPO as Repository

Client->>Server: CreateServiceRequest
Server->>BL: Validate and process
BL->>REPO: Create entity
REPO-->>BL: Created entity
BL-->>Server: Success response
Server-->>Client: CreateServiceResponse

Client->>Server: GetServiceRequest
Server->>BL: Process request
BL->>REPO: Get entity
REPO-->>BL: Entity data
BL-->>Server: Success response
Server-->>Client: GetServiceResponse

Client->>Server: SyncRequest
Server->>BL: Process sync
BL->>REPO: Batch operations
REPO-->>BL: Sync result
BL-->>Server: Success response
Server-->>Client: Empty response
@enduml
```

## Обработка ошибок

### Типы ошибок

```protobuf
enum ErrorCode {
  UNKNOWN = 0;
  VALIDATION_ERROR = 1;      // Ошибка валидации данных
  NOT_FOUND = 2;            // Ресурс не найден
  ALREADY_EXISTS = 3;       // Ресурс уже существует
  CONFLICT = 4;             // Конфликт данных
  INTERNAL_ERROR = 5;       // Внутренняя ошибка сервера
  PERMISSION_DENIED = 6;    // Отказано в доступе
  INVALID_ARGUMENT = 7;     // Неверный аргумент
}
```

### Примеры ошибок

```protobuf
// Ошибка валидации
{
  "code": "VALIDATION_ERROR",
  "message": "Service validation failed",
  "details": "Port 80 is already in use by another service",
  "field_errors": [
    "spec.ingressPorts[0].port: Port 80 conflicts with existing service"
  ]
}

// Ошибка "не найдено"
{
  "code": "NOT_FOUND",
  "message": "Service not found",
  "details": "Service 'web-service' in namespace 'default' does not exist"
}

// Ошибка конфликта
{
  "code": "CONFLICT",
  "message": "Resource conflict",
  "details": "Service with name 'web-service' already exists in namespace 'default'"
}
```

## Метрики и мониторинг

### Метрики gRPC

```go
// Метрики для Prometheus
var (
    grpcRequestsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "netguard_grpc_requests_total",
            Help: "Total number of gRPC requests",
        },
        []string{"method", "status"},
    )
    
    grpcRequestDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "netguard_grpc_request_duration_seconds",
            Help: "Duration of gRPC requests",
        },
        []string{"method"},
    )
    
    grpcErrorsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "netguard_grpc_errors_total",
            Help: "Total number of gRPC errors",
        },
        []string{"method", "error_code"},
    )
)
```

### Interceptors

```go
// Логирование запросов
func LoggingInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
    start := time.Now()
    
    // Логирование входящего запроса
    log.Printf("gRPC request: %s", info.FullMethod)
    
    resp, err := handler(ctx, req)
    
    // Логирование результата
    duration := time.Since(start)
    if err != nil {
        log.Printf("gRPC error: %s, duration: %v", err, duration)
    } else {
        log.Printf("gRPC success: %s, duration: %v", info.FullMethod, duration)
    }
    
    return resp, err
}

// Метрики
func MetricsInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
    start := time.Now()
    
    resp, err := handler(ctx, req)
    
    duration := time.Since(start).Seconds()
    status := "success"
    if err != nil {
        status = "error"
    }
    
    grpcRequestsTotal.WithLabelValues(info.FullMethod, status).Inc()
    grpcRequestDuration.WithLabelValues(info.FullMethod).Observe(duration)
    
    if err != nil {
        grpcErrorsTotal.WithLabelValues(info.FullMethod, getErrorCode(err)).Inc()
    }
    
    return resp, err
}
```

## Конфигурация gRPC

### Сервер

```go
func NewGRPCServer(businessLogic BusinessLogic) *grpc.Server {
    server := grpc.NewServer(
        grpc.UnaryInterceptor(grpc.ChainUnaryInterceptor(
            LoggingInterceptor,
            MetricsInterceptor,
            RecoveryInterceptor,
        )),
        grpc.MaxRecvMsgSize(10*1024*1024), // 10MB
        grpc.MaxSendMsgSize(10*1024*1024), // 10MB
    )
    
    // Регистрация сервиса
    netguard.RegisterNetguardServiceServer(server, &NetguardServiceImpl{
        businessLogic: businessLogic,
    })
    
    return server
}
```

### Клиент

```go
func NewGRPCClient(address string) (netguard.NetguardServiceClient, error) {
    conn, err := grpc.Dial(address,
        grpc.WithInsecure(),
        grpc.WithDefaultCallOptions(
            grpc.MaxCallRecvMsgSize(10*1024*1024), // 10MB
            grpc.MaxCallSendMsgSize(10*1024*1024), // 10MB
        ),
        grpc.WithUnaryInterceptor(ClientLoggingInterceptor),
    )
    if err != nil {
        return nil, fmt.Errorf("failed to connect: %w", err)
    }
    
    return netguard.NewNetguardServiceClient(conn), nil
}
``` 