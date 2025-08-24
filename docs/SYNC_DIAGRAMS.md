# Диаграммы синхронизации с SGROUP

## Обзор

Данный документ содержит диаграммы архитектуры и процессов синхронизации с SGROUP в формате PlantUML. Диаграммы помогают визуализировать взаимодействие компонентов, потоки данных и последовательность операций.

## Архитектурные диаграммы

### 1. Общая архитектура системы синхронизации

```plantuml
@startuml
!theme plain
skinparam backgroundColor #FFFFFF
skinparam componentStyle rectangle

package "Netguard PG Backend" {
    package "API Layer" {
        [Kubernetes API] as K8S_API
        [Aggregated API Server] as AGG_API
        [Admission Controllers] as ADM_CTRL
    }
  
    package "Business Logic Layer" {
        [gRPC Service] as GRPC_SVC
        [Domain Models] as DOMAIN
        [Business Rules] as BIZ_RULES
    }
  
    package "Sync Layer" {
        [Sync Manager] as SYNC_MGR
        [AddressGroup Syncer] as AG_SYNC
        [Network Syncer] as NET_SYNC
        [IEAgAgRule Syncer] as RULE_SYNC
        [SGroup Gateway] as SG_GATEWAY
        [Sync Tracker] as SYNC_TRACK
    }
  
    package "Repository Layer" {
        [Repository Interface] as REPO_IF
        [PostgreSQL Repository] as PG_REPO
        [In-Memory Repository] as MEM_REPO
    }
}

package "External Systems" {
    [SGROUP Service] as SGROUP
    [PostgreSQL Database] as PG_DB
}

' API Layer connections
K8S_API --> AGG_API
AGG_API --> ADM_CTRL
ADM_CTRL --> GRPC_SVC

' Business Logic connections
GRPC_SVC --> DOMAIN
DOMAIN --> BIZ_RULES
BIZ_RULES --> REPO_IF
BIZ_RULES --> SYNC_MGR

' Sync Layer connections
SYNC_MGR --> AG_SYNC
SYNC_MGR --> NET_SYNC
SYNC_MGR --> RULE_SYNC
AG_SYNC --> SG_GATEWAY
NET_SYNC --> SG_GATEWAY
RULE_SYNC --> SG_GATEWAY
SYNC_MGR --> SYNC_TRACK

' Repository connections
REPO_IF --> PG_REPO
REPO_IF --> MEM_REPO
PG_REPO --> PG_DB

' External connections
SG_GATEWAY --> SGROUP : gRPC/TLS

@enduml
```

### 2. Компоненты слоя синхронизации

```plantuml
@startuml
!theme plain
skinparam backgroundColor #FFFFFF
skinparam classAttributeIconSize 0

class SyncManager {
    -syncers: map[SyncSubjectType]EntitySyncer
    -tracker: SyncTracker
    -gateway: SGroupGateway
    +RegisterSyncer(type, syncer)
    +SyncEntity(entity, operation)
    +SyncBatch(entities, operation)
    +Start(ctx)
    +Stop()
}

interface EntitySyncer {
    +Sync(ctx, entity, operation)
    +SyncBatch(ctx, entities, operation)
    +GetSupportedSubjectType()
}

class AddressGroupSyncer {
    -gateway: SGroupGateway
    -logger: Logger
    +Sync(ctx, entity, operation)
    +SyncBatch(ctx, entities, operation)
    +GetSupportedSubjectType()
}

class NetworkSyncer {
    -gateway: SGroupGateway
    -logger: Logger
    +Sync(ctx, entity, operation)
    +SyncBatch(ctx, entities, operation)
    +GetSupportedSubjectType()
}

class IEAgAgRuleSyncer {
    -gateway: SGroupGateway
    -logger: Logger
    +Sync(ctx, entity, operation)
    +SyncBatch(ctx, entities, operation)
    +GetSupportedSubjectType()
}

interface SGroupGateway {
    +Sync(ctx, request)
    +Health(ctx)
    +Connect()
    +Disconnect()
}

class SGroupsClient {
    -client: SGroupsServiceClient
    -config: ClientConfig
    -conn: grpc.ClientConn
    +Sync(ctx, request)
    +Health(ctx)
    +Connect()
    +Disconnect()
    -setupTLS(config)
    -syncWithRetry(ctx, request)
}

interface SyncTracker {
    +Track(type, operation, success)
    +GetStats()
    +ShouldSync(key, operation)
    +ShouldSyncForced(key, operation)
}

class DefaultSyncTracker {
    -stats: map[SyncSubjectType]SyncStats
    -debounceMap: map[string]time.Time
    -mutex: sync.RWMutex
    +Track(type, operation, success)
    +GetStats()
    +ShouldSync(key, operation)
    +ShouldSyncForced(key, operation)
}

SyncManager --> EntitySyncer
SyncManager --> SGroupGateway
SyncManager --> SyncTracker

EntitySyncer <|-- AddressGroupSyncer
EntitySyncer <|-- NetworkSyncer
EntitySyncer <|-- IEAgAgRuleSyncer

AddressGroupSyncer --> SGroupGateway
NetworkSyncer --> SGroupGateway
IEAgAgRuleSyncer --> SGroupGateway

SGroupGateway <|-- SGroupsClient
SyncTracker <|-- DefaultSyncTracker

@enduml
```

## Диаграммы последовательности

### 3. Создание AddressGroup с синхронизацией

```plantuml
@startuml
!theme plain
skinparam backgroundColor #FFFFFF
skinparam sequenceArrowThickness 2
skinparam roundcorner 20

participant "kubectl" as CLI
participant "K8s API Server" as K8S
participant "Aggregated API" as AGG
participant "Admission Controller" as ADM
participant "gRPC Service" as GRPC
participant "Business Logic" as BL
participant "Repository" as REPO
participant "Sync Manager" as SM
participant "AddressGroup Syncer" as AGS
participant "SGroup Gateway" as SGW
participant "SGROUP Service" as SG
participant "PostgreSQL" as DB

CLI->>K8S: POST AddressGroup
K8S->>AGG: Forward request
AGG->>ADM: Validate resource

ADM->>ADM: Schema validation
ADM->>ADM: Business rules validation
ADM-->>AGG: Validation passed

AGG->>GRPC: CreateAddressGroup(proto)
GRPC->>BL: Process creation request
BL->>BL: Apply business rules
BL->>REPO: Create(addressGroup)

REPO->>DB: INSERT INTO address_groups
DB-->>REPO: Success
REPO-->>BL: Created entity

Note over BL,SM: Trigger synchronization
BL->>SM: SyncEntity(addressGroup, Upsert)
SM->>SM: Route to AddressGroupSyncer
SM->>AGS: Sync(addressGroup, Upsert)

AGS->>AGS: Convert to SGROUP format
AGS->>SGW: Sync(syncRequest)
SGW->>SGW: Setup TLS connection
SGW->>SG: gRPC Sync call

SG-->>SGW: Success response
SGW-->>AGS: Sync complete
AGS-->>SM: Success
SM-->>BL: Sync result

BL-->>GRPC: Success response
GRPC-->>AGG: 201 Created
AGG-->>K8S: Success
K8S-->>CLI: AddressGroup created

@enduml
```

### 4. Batch синхронизация Networks

```plantuml
@startuml
!theme plain
skinparam backgroundColor #FFFFFF
skinparam sequenceArrowThickness 2
skinparam roundcorner 20

participant "Business Logic" as BL
participant "Sync Manager" as SM
participant "Network Syncer" as NS
participant "SGroup Gateway" as SGW
participant "SGROUP Service" as SG

BL->>SM: SyncBatch(networks[], Upsert)
SM->>SM: Group by entity type
SM->>NS: SyncBatch(networks[], Upsert)

loop For each network
    NS->>NS: Convert to SGROUP format
end

NS->>NS: Create batch request
NS->>SGW: Sync(batchRequest)

SGW->>SGW: Validate batch size
SGW->>SG: gRPC Batch Sync call

alt Batch sync successful
    SG-->>SGW: Success response
    SGW-->>NS: Batch complete
    NS->>NS: Update metrics
    NS-->>SM: Batch success
    SM-->>BL: All networks synchronized
else Batch sync failed
    SG-->>SGW: Error response
    SGW->>SGW: Initiate retry
    SGW->>SG: Retry batch sync
    SG-->>SGW: Success on retry
    SGW-->>NS: Batch complete after retry
    NS-->>SM: Batch success with retry
    SM-->>BL: All networks synchronized
end

@enduml
```

### 5. Обработка сбоя SGROUP с восстановлением

```plantuml
@startuml
!theme plain
skinparam backgroundColor #FFFFFF
skinparam sequenceArrowThickness 2
skinparam roundcorner 20

participant "Business Logic" as BL
participant "Sync Manager" as SM
participant "SGroup Gateway" as SGW
participant "Sync Tracker" as ST
participant "SGROUP Service" as SG

BL->>SM: SyncEntity(entity, Upsert)
SM->>SGW: Sync(syncRequest)
SGW->>SG: gRPC Sync call

SG-->>SGW: Connection refused
SGW-->>SM: UNAVAILABLE error
SM->>ST: Track(type, operation, false)
SM->>SM: Queue for retry
SM-->>BL: Sync queued

Note over SM,ST: SGROUP is down, operations queued

loop Health check every 30s
    SGW->>SG: Health check
    SG-->>SGW: Connection refused
end

Note over SG: SGROUP service restored

SGW->>SG: Health check
SG-->>SGW: Healthy response
SGW->>SM: SGROUP available

SM->>SM: Process queued operations
loop For each queued operation
    SM->>SGW: Sync(queuedRequest)
    SGW->>SG: gRPC Sync call
    SG-->>SGW: Success response
    SGW-->>SM: Sync complete
    SM->>ST: Track(type, operation, true)
end

SM-->>BL: All queued operations synchronized

@enduml
```

## Диаграммы состояний

### 6. Состояния синхронизации сущности

```plantuml
@startuml
!theme plain
skinparam backgroundColor #FFFFFF

[*] --> Created : Entity created

Created --> Validating : Trigger sync
Validating --> Converting : Validation passed
Validating --> Failed : Validation failed

Converting --> Syncing : Conversion successful
Converting --> Failed : Conversion failed

Syncing --> Synchronized : Sync successful
Syncing --> Retrying : Sync failed (retryable)
Syncing --> Failed : Sync failed (non-retryable)

Retrying --> Syncing : Retry attempt
Retrying --> Failed : Max retries exceeded

Synchronized --> Converting : Entity updated
Synchronized --> Deleting : Entity deleted

Deleting --> Deleted : Delete sync successful
Deleting --> Failed : Delete sync failed

Failed --> Converting : Manual retry
Failed --> [*] : Give up

Synchronized --> [*] : Entity lifecycle complete
Deleted --> [*] : Entity removed

@enduml
```

### 7. Состояния SGROUP соединения

```plantuml
@startuml
!theme plain
skinparam backgroundColor #FFFFFF

[*] --> Disconnected : Initial state

Disconnected --> Connecting : Connect()
Connecting --> Connected : Connection successful
Connecting --> Disconnected : Connection failed

Connected --> Healthy : Health check passed
Connected --> Unhealthy : Health check failed
Connected --> Disconnected : Connection lost

Healthy --> Syncing : Sync operation
Healthy --> Unhealthy : Health check failed

Syncing --> Healthy : Sync successful
Syncing --> Unhealthy : Sync failed

Unhealthy --> Reconnecting : Attempt reconnect
Unhealthy --> Disconnected : Give up reconnecting

Reconnecting --> Connected : Reconnection successful
Reconnecting --> Disconnected : Reconnection failed

Connected --> Disconnected : Disconnect()
Healthy --> Disconnected : Disconnect()
Unhealthy --> Disconnected : Disconnect()

Disconnected --> [*] : Shutdown

@enduml
```

## ### 9. Поток данных при синхронизации

```plantuml
@startuml
!theme plain
skinparam backgroundColor #FFFFFF

package "Data Flow" {
    rectangle "Kubernetes Resource" as k8s_res {
        [AddressGroup YAML]
    }
  
    rectangle "Internal Model" as internal {
        [Domain Model]
    }
  
    rectangle "SGROUP Format" as sgroup_fmt {
        [Protobuf Message]
    }
  
    rectangle "External System" as external {
        [SGROUP Service]
    }
}

k8s_res --> internal : Parse & Validate
internal --> sgroup_fmt : ToSGroupsProto()
sgroup_fmt --> external : gRPC Sync

note right of k8s_res
  apiVersion: netguard.sgroups.io/v1beta1
  kind: AddressGroup
  metadata:
    name: web-servers
    namespace: production
  spec:
    addresses:
      - "10.0.1.10"
      - "10.0.1.11"
end note

note right of internal
  type AddressGroup struct {
    Name      string
    Namespace string
    Addresses []string
  }
end note

note right of sgroup_fmt
  message SyncGroups {
    repeated Group groups = 1;
  }
  
  message Group {
    string name = 1;
    repeated string members = 2;
  }
end note

note right of external
  gRPC Service
  - Groups management
  - Networks management
  - Rules management
end note

@enduml
```

```## Заключение

Представленные диаграммы обеспечивают полное понимание архитектуры и процессов синхронизации с SGROUP:

1. **Архитектурные диаграммы** - показывают структуру компонентов и их взаимосвязи
2. **Диаграммы последовательности** - демонстрируют потоки выполнения операций
3. **Диаграммы состояний** - описывают жизненные циклы сущностей и соединений
4. **Диаграммы развертывания** - показывают физическое размещение компонентов
5. **Диаграммы мониторинга** - иллюстрируют сбор и обработку метрик

Эти диаграммы могут быть использованы для:

- Понимания архитектуры системы
- Планирования разработки
- Документирования процессов
- Обучения новых разработчиков
- Диагностики проблем
