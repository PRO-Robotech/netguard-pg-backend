# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**Netguard PG Backend** is a network security resource management service implementing the [sgroups-k8s-netguard](https://github.com/PRO-robotech/sgroups-k8s-netguard) specification. It provides a gRPC-based backend for storing and managing network security resources with synchronization to external SGROUP systems.

The project consists of multiple components:
- **Backend Server**: Core gRPC/HTTP service (port 9090/8080)
- **Kubernetes API Server**: Aggregated API server for K8s integration
- **Webhook Server**: Admission webhook for validation
- **OutboxWorker**: Asynchronous SGROUP synchronization worker
- **Reverse Sync System**: Bi-directional SGROUP ↔ NETGUARD synchronization

**CRITICAL**: Russian-speaking project. All documentation, comments, and communication may be in Russian.

## Context7 Integration (MANDATORY)

**ALWAYS use Context7** when you need:
- Code generation with specific libraries
- Setup or configuration steps for frameworks/tools
- Library/API documentation
- Checking latest API signatures or examples

**How to use Context7**:
1. First call `mcp__context7__resolve-library-id` to get the Context7-compatible library ID
2. Then call `mcp__context7__get-library-docs` with that library ID to get documentation

**Example libraries you may need**:
- PostgreSQL driver: `pgx`
- gRPC: `grpc-go`
- Kubernetes client: `k8s.io/client-go`
- Goose migrations: `pressly/goose`

**DO NOT** guess API signatures or configuration formats. Always consult Context7 first.

## Testing with TestSprite (MANDATORY)

**CRITICAL**: When testing is required, ALWAYS use **TestSprite MCP tools**.

**Available TestSprite Tools**:
- `mcp__TestSprite__testsprite_bootstrap_tests` - Initialize testing environment
- `mcp__TestSprite__testsprite_generate_code_summary` - Analyze codebase
- `mcp__TestSprite__testsprite_generate_standardized_prd` - Generate PRD
- `mcp__TestSprite__testsprite_generate_frontend_test_plan` - Frontend test plan
- `mcp__TestSprite__testsprite_generate_backend_test_plan` - Backend test plan (USE THIS)
- `mcp__TestSprite__testsprite_generate_code_and_execute` - Generate and execute tests

**Workflow for Backend Testing**:
1. Bootstrap: `testsprite_bootstrap_tests` (type: "backend", localPort: 8080)
2. Generate test plan: `testsprite_generate_backend_test_plan`
3. Execute tests: `testsprite_generate_code_and_execute`

**DO NOT** write manual tests without TestSprite when comprehensive testing is required.

## Deployment Context (MANDATORY)

**CRITICAL**: Read `.claude/deployment-context.md` before ANY deployment activities.

**Key deployment rules**:
1. **ALWAYS** use `./scripts/deploy-local.sh` for deployments
2. **NEVER** manually use `kubectl set image` with `:local-dev` tag
3. **ALWAYS** verify image hashes after deployment (3-level check)
4. **ALWAYS** verify migration version in database
5. **ALWAYS** test functionality after deployment

**Cluster Info**:
- Name: `incloud`
- Namespace: `incloud-sgroups`
- Context: `incloud`
- PostgreSQL: `netguard-postgresql-0` pod

**Database Connection** (for direct queries):
```bash
kubectl exec netguard-postgresql-0 -n incloud-sgroups -- \
  env PGPASSWORD=netguard psql -U netguard -d netguard -c "YOUR SQL HERE"
```

## Minikube Deployment Skill (RECOMMENDED)

**NEW**: Standardized deployment workflow available as skill!

### What is the Skill?

The `deploy-minikube` skill provides a comprehensive, reliable workflow for deploying all Netguard components to Minikube cluster (incloud context).

**Key Benefits:**
- ✅ Standardized workflow - no improvisation
- ✅ Comprehensive verification - all aspects checked
- ✅ Delete + LocalDev strategy - reliable cache bypass
- ✅ Support for all components - backend, apiserver, webhook
- ✅ Automated deployment - build, deploy, verify in one command

### How to Use

**Option 1: Via Skill (in agent/conversation)**
```
Use skill: deploy-minikube
Component: backend
```

**Option 2: Via Slash Command**
```bash
/deploy backend
/deploy apiserver
/deploy webhook
/deploy backend+apiserver
/deploy all
```

**Option 3: Direct Script (from terminal)**
```bash
./scripts/deploy-local.sh backend
./scripts/deploy-local.sh apiserver
./scripts/deploy-local.sh webhook
./scripts/deploy-local.sh backend+apiserver
./scripts/deploy-local.sh all
```

**Option 4: Via Makefile**
```bash
make minikube-backend
make minikube-apiserver
make minikube-webhook
make minikube-all
make minikube-help
```

### Available Components

1. **backend** - Backend service + goose migrations (most common)
2. **apiserver** - Kubernetes Aggregated API Server
3. **webhook** - Admission webhook for validation
4. **backend+apiserver** - Deploy both backend and API server
5. **all** - Deploy all components (backend + apiserver + webhook + goose)

### What the Skill Does Automatically

1. **Build** images with `:local-dev` tags
2. **Delete** old images from Minikube (cache bypass!)
3. **Load** new images into Minikube
4. **Update** deployments with new images
5. **Force** rollout restart
6. **Monitor** rollout progress and migrations
7. **Verify** deployment success:
   - Image hash verification (3-level: local → cluster → pod)
   - Migration version check (for backend)
   - Pod status check (Running, Ready)
   - Logs check (no errors)
   - Functional test (create resource, check outbox)
8. **Report** results with verification summary

### Deployment Strategy (v3.0)

**Delete + LocalDev Strategy** - The ONLY correct approach:
- Use consistent `:local-dev` tags (no timestamp tags!)
- Delete old image before loading new (reliable cache bypass)
- No image accumulation in Minikube
- Simple, predictable workflow

### Documentation

- **Skill Definition:** `.claude/skills/deploy-minikube.md`
- **Slash Command:** `.claude/commands/deploy.md`
- **Full Workflow:** `.claude/minikube-deploy-standard.md`
- **Deployment Context:** `.claude/deployment-context.md`
- **Deploy Script:** `scripts/deploy-local.sh`

### When to Use This Skill

- Deploying code changes to Minikube
- Applying new database migrations
- Updating API server or webhook
- Testing changes in Minikube environment
- Full system deployment/refresh

### Example Usage

**Deploy backend after code changes:**
```bash
# User: "Deploy my backend changes to minikube"
# Claude: Uses deploy-minikube skill → backend component
# Result: Code built, deployed, verified automatically
```

**Deploy all components:**
```bash
# User: "Deploy all components to minikube"
# Claude: Uses deploy-minikube skill → all components
# Result: Full system deployment with verification
```

**Deploy with new migration:**
```bash
# User: "Deploy backend with migration 38 to minikube"
# Claude: Uses deploy-minikube skill with --expected-version 38
# Result: Backend deployed, migration 38 verified in database
```

**IMPORTANT:** DevOps/SRE agent is configured to use this skill automatically for all Minikube deployments!

## QA Testing Resources (MANDATORY)

**CRITICAL**: When creating test manifests, read `.claude/qa-test-resources.md` first.

**API Version**: `netguard.sgroups.io/v1beta1` (ALWAYS use this exactly!)
**Namespace**: `incloud-sgroups` (ALWAYS use this!)
**Architecture**: Kubernetes Aggregation Layer (NOT CRDs!)

**Available Resources**:
- Host (requires `spec.uuid` in UUID format)
- Network (requires `spec.cidr` in CIDR notation)
- AddressGroup (requires `spec.defaultAction: ACCEPT|DROP`)
- HostBinding (binds Host to AddressGroup)
- NetworkBinding (binds Network to AddressGroup)

**Example**:
```yaml
apiVersion: netguard.sgroups.io/v1beta1
kind: Host
metadata:
  name: test-host-001
  namespace: incloud-sgroups
spec:
  uuid: "550e8400-e29b-41d4-a716-446655440001"
```

**NEVER**:
- Guess API signatures
- Use `apiVersion: netguard.io/v1` (wrong group!)
- Use `apiVersion: netguard.sgroups.io/v1` (wrong version!)
- Use namespace other than `incloud-sgroups`

## Development Commands

### Local Development

```bash
# Start with in-memory database (fast development)
make run
go run cmd/server/main.go --memory

# Start with PostgreSQL (production-like)
make run-pg
go run cmd/server/main.go --pg-uri="postgres://postgres:postgres@localhost:5432/netguard?sslmode=disable"

# Build server binary
make build
go build -o bin/netguard-server cmd/server/main.go
```

### Testing

```bash
# Run all tests
make test
go test ./...

# Run specific test types
make test-unit          # Domain layer tests
make test-integration   # Repository tests (mem + pg)
make test-e2e          # API and application tests

# Run with coverage
make test-coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Run single test
go test -v -run TestSpecificFunction ./path/to/package

# PostgreSQL integration tests (requires running PostgreSQL)
TEST_PG_URI="postgres://postgres:postgres@localhost:5432/netguard_test?sslmode=disable" go test -v ./internal/infrastructure/repositories/pg/...
```

### PostgreSQL Development

```bash
# Start PostgreSQL container
make pg-setup

# Stop PostgreSQL container
make pg-stop

# Run migrations manually
make pg-migrate

# Open PostgreSQL shell
make pg-shell

# Reset database (stop, setup, migrate)
make pg-reset

# Check connection status
make pg-status
```

### Database Migrations

The project uses [Goose](https://github.com/pressly/goose) for database migrations.

```bash
# Run migrations (using Goose)
make netguard-pg-migrations PG_URI="postgres://user:password@host:5432/dbname"

# Install Goose locally
make .install-goose

# Check migration status
./bin/goose -dir migrations postgres "$PG_URI" status

# Apply migrations manually
./bin/goose -dir migrations postgres "$PG_URI" up

# Rollback last migration
./bin/goose -dir migrations postgres "$PG_URI" down
```

**IMPORTANT**: In production, migrations run in a separate Kubernetes Job (netguard-migrations) before the backend starts. The `--migrate` flag is deprecated but kept for local development compatibility.

### Kubernetes Deployment

```bash
# Local development with ArgoCD integration
make dev-start          # Suspend ArgoCD, deploy local images
make dev-stop           # Restore ArgoCD control
make dev-status         # Show deployment status
make dev-logs           # Follow backend logs

# Fast component-only deployments (no ArgoCD suspension)
make dev-backend-fast   # Deploy only backend
make dev-apiserver-fast # Deploy only API server
make dev-webhook-fast   # Deploy only webhook

# Manual local deployment (RECOMMENDED for all deployments)
./scripts/deploy-local.sh backend        # Backend only
./scripts/deploy-local.sh apiserver      # API Server only
./scripts/deploy-local.sh webhook        # Webhook only
./scripts/deploy-local.sh all            # All services

# Deploy to Minikube/Kind
make deploy-memory      # In-memory backend (fast)
make deploy-postgresql  # PostgreSQL backend (production)

# Check deployment status
make status-deployment
kubectl get pods,svc,deployment -n incloud-sgroups

# View logs
make logs-backend
make logs-postgresql
kubectl logs -f deployment/netguard-backend -n incloud-sgroups
```

**CRITICAL Deployment Rules** (from deployment-context.md):
1. ALWAYS use `./scripts/deploy-local.sh` (handles unique image tags automatically)
2. NEVER reuse `:local-dev` tag for deployment (causes cache issues)
3. ALWAYS verify migration version in database after deployment
4. ALWAYS check pod image hashes match local image
5. ALWAYS test functionality after deployment (create resource, check outbox)

### Docker

```bash
# Build images
make docker-build              # Backend
make docker-build-k8s-apiserver # API server
make docker-build-webhook      # Webhook
make docker-build-goose        # Migration runner

# Run with Docker Compose
make docker-compose-up
make docker-compose-down
```

## Architecture

### Clean Architecture Layers

The project follows **Clean Architecture** (Hexagonal Architecture) principles:

```
internal/
├── domain/              # Core business logic (innermost layer)
│   ├── models/         # Domain entities (Host, Network, AddressGroup, etc.)
│   ├── ports/          # Interfaces (Repository, SyncManager)
│   └── registry/       # Resource registry pattern
├── application/         # Use cases and orchestration
│   ├── services/       # Application services (NetguardFacade, ConditionManager)
│   └── validation/     # Business validation rules
├── infrastructure/      # External concerns (outermost layer)
│   └── repositories/   # Data access implementations
│       ├── pg/         # PostgreSQL implementation
│       └── mem/        # In-memory implementation
├── api/                # API layer (gRPC/HTTP)
│   └── netguard/       # gRPC service implementation
└── sync/               # SGROUP synchronization system
    ├── manager/        # Sync orchestration
    ├── syncers/        # Resource-specific syncers
    ├── worker/         # OutboxWorker (async sync)
    └── clients/        # SGROUP client
```

**Key Principles**:
- **Domain** layer has NO dependencies on other layers
- **Application** layer depends only on Domain
- **Infrastructure** implements Domain interfaces
- **API** layer orchestrates Application services

### Critical Components

#### 1. OutboxWorker (Async Synchronization)

**Location**: `internal/sync/worker/`

The OutboxWorker implements the **Transactional Outbox Pattern** for reliable SGROUP synchronization:

- Polls `sync_outbox` table for pending entries
- Processes sync operations asynchronously
- Retries with exponential backoff (configurable retry limits for validation/temporary/network errors)
- Updates sync status in `sync_status` field (JSONB)
- Exposes `/healthz/worker` endpoint and Prometheus metrics

**Configuration**: Environment variables (see `.env.example`)
- `OUTBOX_WORKER_ENABLED`: Enable/disable worker
- `OUTBOX_WORKER_POLL_INTERVAL`: Polling frequency (default: 5s)
- `OUTBOX_WORKER_BATCH_SIZE`: Entries per batch (default: 10)
- `OUTBOX_MAX_ATTEMPTS_VALIDATION`: Retry limit for validation errors (default: 3)
- `OUTBOX_MAX_ATTEMPTS_TEMPORARY`: Retry limit for temporary errors (default: 20)
- `OUTBOX_MAX_ATTEMPTS_NETWORK`: Retry limit for network errors (default: 100)

**Database Triggers**: All resource changes (Host, Network, AddressGroup, Service, HostBinding, NetworkBinding) automatically insert outbox entries via PostgreSQL triggers.

#### 2. Condition Manager (Ready Status)

**Location**: `internal/application/services/condition_manager.go`

Manages resource readiness based on **declarative conditions**:

```go
// Example: Host is ready when its Network exists
type HostCondition struct {
    Type: "NetworkReady"
    Check: func(host) bool {
        return networkExists(host.Spec.Network)
    }
}
```

**Auto-sync Trigger**: PostgreSQL triggers automatically manage sync operations when resources are created, updated, or deleted.

**Important**: The `SyncReady` flag in `spec` JSONB controls whether OutboxWorker processes the entry.

#### 3. Resource Registry Pattern

**Location**: `internal/domain/registry/`

Centralizes resource metadata and type mappings:

```go
// Lookup resource info by table name
info := registry.GetResourceByTableName("hosts")
// Returns: ResourceInfo with GVK, syncers, dependencies, etc.
```

**Used by**:
- Database triggers (determine sync_subject_type)
- OutboxWorker (route to correct syncer)
- API layer (validate resource types)

**CRITICAL**: Call `registry.ValidateRegistry()` at startup (already done in `cmd/server/main.go`)

#### 4. SGROUP Synchronization

**Two-way sync**:
- **Forward Sync** (NETGUARD → SGROUP): Via OutboxWorker + database triggers
- **Reverse Sync** (SGROUP → NETGUARD): Via `ReverseSyncSystem` (monitors SGROUP changes)

**Syncers** (`internal/sync/syncers/`):
- `HostSyncer`: Syncs Host resources
- `AddressGroupSyncer`: Syncs AddressGroup as SGROUP Groups
- `NetworkSyncer`: Syncs Network resources
- `ServiceSyncer`: Syncs Service resources
- `IEAgAgRuleSyncer`: Syncs inter-group rules

Each syncer implements `ResourceSyncer` interface with `Sync()`, `Delete()`, and `GetSubjectType()` methods.

### Database Schema

**Key Tables**:
- `hosts`, `networks`, `address_groups`, `services`: Resource tables
- `host_bindings`, `network_bindings`: Relationship tables
- `sync_outbox`: Outbox pattern table (feeds OutboxWorker)
- `netguard_db_ver`: Goose migration version tracking (DO NOT modify manually)

**Critical Columns**:
- `spec` (JSONB): Contains `sync_ready` flag (controls sync eligibility)
- `sync_status` (JSONB): Sync state tracking (last_sync_time, attempts, error)
- `namespace_name`: Full namespaced name (format: `namespace/name`)

**Important Migrations**:
- `024`: CIDR overlap prevention (GIST exclusion constraint)
- `025`: sync_outbox table creation and base types (sync_operation, target_system, outbox_status)
- `026`: Host resource triggers (INSERT/UPDATE/DELETE → sync_outbox)
- `027`: HostBinding/NetworkBinding triggers (process resource changes)
- `028`: AddressGroup binding change triggers (hosts/networks spec updates)

### Configuration

**Config File**: `config/config.yaml`

**Key Sections**:
- `settings.grpc-addr`: gRPC server address (default: `:9090`)
- `settings.http-addr`: HTTP server address (default: `:8080`)
- `sync.enabled`: Enable SGROUP synchronization
- `sync.sgroups.grpc_address`: SGROUP server endpoint
- `reverse_sync`: Reverse sync configuration (SGROUP → NETGUARD)

**Environment Overrides**: All config values can be overridden via environment variables (see `.env.example`)

## Testing Guidelines

### Unit Tests

Focus on **domain logic** without external dependencies:

```bash
go test -v ./internal/domain/...
go test -v ./internal/application/...
```

### Integration Tests

**Location**: `internal/sync/integration/`, `test/e2e/`

Integration tests use **testcontainers** for PostgreSQL:

```go
// Pattern: Create real PostgreSQL container
pool, resource := setupTestPostgres(t)
defer teardownTestPostgres(pool, resource)

// Run migrations
runMigrations(t, pool)

// Test against real database
```

**Running**:
```bash
# Requires Docker
go test -v ./internal/sync/integration/...
go test -v ./test/e2e/...
```

### E2E Tests with TestSprite

**CRITICAL**: For comprehensive E2E testing, use **TestSprite**.

**Workflow**:
1. Bootstrap testing environment:
   ```
   testsprite_bootstrap_tests(
     type: "backend",
     localPort: 8080,
     projectPath: "/Users/zhd/Projects/newPro/netguard-pg-backend",
     testScope: "codebase"
   )
   ```

2. Generate backend test plan:
   ```
   testsprite_generate_backend_test_plan(
     projectPath: "/Users/zhd/Projects/newPro/netguard-pg-backend"
   )
   ```

3. Execute tests:
   ```
   testsprite_generate_code_and_execute(
     projectName: "netguard-pg-backend",
     projectPath: "/Users/zhd/Projects/newPro/netguard-pg-backend",
     testIds: [],  # Empty = all tests
     additionalInstruction: ""
   )
   ```

**DO NOT** manually write comprehensive E2E tests when TestSprite is available.

### Test Manifest Creation

@KUBECTL_NETGUARD_QUICK_REFERENCE.md - документ для понимания того как через kubectl работать с сущностями Netguard!


**ALWAYS** consult `.claude/qa-test-resources.md` before creating test YAML files.

**Template** (Host example):
```yaml
apiVersion: netguard.sgroups.io/v1beta1
kind: Host
metadata:
  name: test-host-001
  namespace: incloud-sgroups
spec:
  uuid: "550e8400-e29b-41d4-a716-446655440001"
```

**Save test manifests** in: `test/e2e/testdata/`

## Common Development Scenarios

### Adding a New Resource Type

1. **Define domain model** in `internal/domain/models/`
2. **Add to Resource Registry** in `internal/domain/registry/registry.go`
3. **Create migration** in `migrations/` (table + triggers)
4. **Implement syncer** in `internal/sync/syncers/`
5. **Register syncer** in `cmd/server/main.go` (`setupSyncManager()`)
6. **Add API endpoint** in `internal/api/netguard/`
7. **Write tests** (unit + integration + TestSprite)

**IMPORTANT**: Use Context7 to check API patterns for gRPC/K8s integration.

### Debugging Sync Issues

1. **Check OutboxWorker health**: `curl http://localhost:8080/healthz/worker`
2. **Query outbox table**:
   ```sql
   SELECT * FROM sync_outbox WHERE status != 'completed' ORDER BY created_at DESC;
   ```
3. **Check sync_status**:
   ```sql
   SELECT namespace_name, spec->'sync_ready', sync_status FROM hosts;
   ```
4. **View worker logs**: Look for `[OutboxWorker]` prefix
5. **Check Prometheus metrics**: `curl http://localhost:8080/metrics | grep outbox`

### Debugging Deployment Issues

**Follow deployment-context.md verification steps**:

1. **Image Hash Verification**:
   ```bash
   # Local image
   docker images netguard/pg-backend:deploy-* --format "{{.ID}}" | head -1

   # Pod image
   kubectl get pod -n incloud-sgroups -l app=netguard-backend \
     -o jsonpath='{.items[0].status.containerStatuses[0].imageID}'
   ```

2. **Migration Version Verification**:
   ```bash
   kubectl exec netguard-postgresql-0 -n incloud-sgroups -- \
     env PGPASSWORD=netguard psql -U netguard -d netguard \
     -c "SELECT version_id FROM netguard_db_ver ORDER BY id DESC LIMIT 1;"
   ```

3. **Functionality Test**:
   ```bash
   # Create test resource
   kubectl apply -f test/e2e/testdata/cloud233/hosts/test-host-001.yaml

   # Check outbox entry
   kubectl exec netguard-postgresql-0 -n incloud-sgroups -- \
     env PGPASSWORD=netguard psql -U netguard -d netguard \
     -c "SELECT resource_type, resource_namespace, resource_name, status FROM sync_outbox ORDER BY created_at DESC LIMIT 1;"

   # Cleanup
   kubectl delete -f test/e2e/testdata/cloud233/hosts/test-host-001.yaml
   ```

### Troubleshooting Migration Issues

1. **Check current version**: `./bin/goose -dir migrations postgres "$PG_URI" status`
2. **Verify applied migrations**: `SELECT * FROM netguard_db_ver ORDER BY id DESC;`
3. **Manual rollback**: `./bin/goose -dir migrations postgres "$PG_URI" down`
4. **Reset database**: `make pg-reset` (destroys all data!)

### Local Development with Real SGROUP

1. **Deploy SGROUP locally**: Follow sgroups-k8s deployment guide
2. **Configure sync**: Update `config/config.yaml` with SGROUP endpoint
3. **Enable TLS** (if required): Set TLS certificates in config
4. **Test connection**: `curl http://localhost:8080/health/sgroup`

## Important Constraints

### CIDR Overlap Prevention

**Migration 024** enforces CIDR uniqueness using PostgreSQL GIST index:

```sql
-- Networks cannot have overlapping CIDR blocks
EXCLUDE USING GIST (cidr inet_ops WITH &&)
```

**Before applying migration 024**:
- Check existing data: `SELECT * FROM networks WHERE cidr && other_cidr;`
- Remove overlaps or abort migration

**Error handling**: API returns `ConflictError` with details about overlapping CIDRs.

### Sync-First Deletion

Resources with `spec.sync_ready = true` MUST sync deletion to SGROUP before database removal:

- **Delete triggers** insert "delete" entries into `sync_outbox`
- **OutboxWorker** processes delete operations
- **Only after successful SGROUP sync** is the resource removed from database

**Important**: Never manually delete from resource tables if `spec.sync_ready = true`. Always use the API or kubectl delete.

### Namespace Naming

All resources use **fully qualified names**: `namespace/name`

- Stored in `namespace_name` column (unique constraint)
- Triggers automatically compute from `namespace` + `name` columns
- **Never modify `namespace_name` directly** - use `namespace` and `name` columns

## Agent System

This project uses specialized Claude Code agents for different roles:

**Available Agents** (in `.claude/agents/`):
- `backend-dev` - Backend development
- `devops-sre` - Deployment and infrastructure
- `migration-expert` - Database migrations
- `db-expert` - Database design and optimization
- `orm-expert` - GORM models and repository patterns
- `qa-engineer` - Testing and quality assurance
- `architect` - Architecture design
- `tech-lead` - Task coordination and routing

**Agent Usage**:
- Use Task tool to delegate to specialized agents
- Each agent has specific responsibilities defined in their .md file
- Agents follow Definition of Done (DoD) standards in `.claude/docs/dod/`

**Example**:
```
"Deploy latest migrations to minikube cluster"
→ Use Task tool with devops-sre agent
→ Agent follows deployment-context.md workflow
→ Returns deployment verification results
```

## Project Dependencies

**Key Libraries**:
- `google.golang.org/grpc`: gRPC server/client
- `github.com/grpc-ecosystem/grpc-gateway/v2`: HTTP/gRPC gateway
- `github.com/jackc/pgx/v5`: PostgreSQL driver
- `github.com/pressly/goose/v3`: Database migrations
- `k8s.io/apiserver`: Kubernetes aggregated API server
- `k8s.io/client-go`: Kubernetes client
- `github.com/testcontainers/testcontainers-go`: Integration testing

**Go Version**: 1.24.0 (specified in `go.mod`)

**IMPORTANT**: When working with these libraries, use Context7 to get latest documentation:
```
1. resolve-library-id(libraryName: "pgx")
2. get-library-docs(context7CompatibleLibraryID: "<id from step 1>")
```

## Documentation Structure

### Deployment Documentation
- `.claude/deployment-context.md` - **CRITICAL**: Deployment workflows, verification steps
- `DEPLOYMENT_STRATEGY.md` - Multi-service deployment strategy

### QA Documentation
- `.claude/qa-test-resources.md` - **CRITICAL**: API signatures for testing

### Agent Documentation
- `.claude/agents/*.md` - Agent role definitions
- `.claude/docs/dod/*.md` - Definition of Done standards

### Feature Documentation
- `docs/features/` - Feature specifications (CIDR validation, etc.)
- `docs/` - API layers, sync operations, architecture

### Migration Documentation
- `migrations/*_NOTES.md` - Migration-specific notes
- `migrations/*_ACCEPTANCE_CHECKLIST.md` - Migration verification checklists

## Performance Considerations

- **Batch Size**: OutboxWorker processes entries in batches (default: 10). Increase for high throughput.
- **Poll Interval**: Default 5s polling. Reduce for lower latency, increase for lower DB load.
- **SGROUP Timeout**: Default 4s per sync operation. Adjust based on SGROUP performance.
- **Database Indexes**: All foreign keys and `namespace_name` columns are indexed.
- **Connection Pooling**: pgx pool configured with sensible defaults in `pg.Registry`.

## Security Notes

- **TLS Configuration**: Set `sync.sgroups.tls.enabled: true` for production
- **Certificate Verification**: Use `verify: "verify"` mode (not `skip`) in production
- **Database Credentials**: Never commit to git. Use environment variables or Kubernetes secrets.
- **API Authentication**: Currently uses TLS certificates. Configure in `authn` section.

## Key Reminders

1. **ALWAYS** use Context7 for library documentation and code generation
2. **ALWAYS** use TestSprite for comprehensive testing
3. **ALWAYS** read `.claude/deployment-context.md` before deployments
4. **ALWAYS** read `.claude/qa-test-resources.md` before creating test manifests
5. **NEVER** guess API signatures - consult documentation sources
6. **NEVER** reuse `:local-dev` tag for Kubernetes deployments
7. **ALWAYS** verify migrations, image hashes, and functionality after deployment
8. Russian language may be used throughout the project
