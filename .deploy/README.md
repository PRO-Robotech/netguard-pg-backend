# NetGuard Debug Infrastructure

This directory contains all debugging tools and configurations for NetGuard development.

## Quick Start

### 🚀 Fastest Way (3 Commands)

```bash
# 1. Deploy debug pods (in skaffold directory)
cd skaffold && skaffold dev -p debug-all

# 2. In another terminal, start debug session
cd .deploy && ./start-debug.sh

# 3. Connect your IDE to localhost:2345 (Backend), 2346 (APIServer), 2347 (Webhook)
```

That's it! Set breakpoints and debug.

### Option 1: Skaffold (Recommended)

Best for debugging in real cluster environment with automated workflows.

```bash
# Deploy debug pods
cd skaffold/
skaffold dev -p debug-all  # All services
# OR
skaffold dev -p debug-backend  # Backend only

# In another terminal, start port-forwards and switch services
cd ..
./start-debug.sh

# Verify everything is working
./verify-debug-setup.sh
```

**What happens:**
- Debug pods are deployed with Delve debugger
- Production services are switched to debug pods
- Port-forwards are established (2345, 2346, 2347)
- Frontend requests now hit debug pods with breakpoints

Connect your IDE to:
- Backend: `localhost:2345`
- APIServer: `localhost:2346`
- Webhook: `localhost:2347`

When done: `./stop-debug.sh`

### Option 2: Telepresence (Fast Iteration)

Best for rapid development on a single service.

```bash
# Prerequisites check
./telepresence/setup-local-env.sh

# Start intercepting backend
./telepresence/intercept-backend.sh

# In another terminal, run locally with debugger
export $(cat /tmp/backend-env.txt | xargs)
export DATABASE_CONNECTION_URL="postgres://netguard:netguard@localhost:5432/netguard?sslmode=disable"
dlv debug ../../cmd/server/main.go --headless --listen=:2345 --api-version=2
```

Connect your IDE to `localhost:2345` and start debugging!

## Directory Structure

```
.deploy/
├── README.md                      # This file
├── QUICKSTART.md                  # One-page quick start guide
├── start-debug.sh                 # 🚀 Start debug session (all-in-one)
├── stop-debug.sh                  # Stop debug session and restore production
├── restart-port-forwards.sh       # Restart port-forwards if they crash
├── switch-to-debug.sh             # Redirect production to debug pods
├── restore-production.sh          # Restore production routing
├── verify-debug-setup.sh          # Verify debug infrastructure
├── dockerfiles/                   # Debug Dockerfiles with Delve
│   ├── Dockerfile.backend.debug
│   ├── Dockerfile.apiserver.debug
│   ├── Dockerfile.webhook.debug
│   └── Dockerfile.goose.debug
├── k8s/                           # Debug Kubernetes manifests
│   ├── backend-debug.yaml
│   ├── apiserver-debug.yaml
│   ├── apiserver-environment-debug.yaml  # APIServer env vars (BACKEND_ENDPOINT)
│   └── webhook-debug.yaml
├── skaffold/                      # Skaffold configuration
│   └── skaffold.yaml
└── telepresence/                  # Telepresence scripts
    ├── setup-local-env.sh
    ├── intercept-backend.sh
    ├── intercept-apiserver.sh
    └── intercept-webhook.sh
```

## Documentation

Comprehensive documentation is available in `.claude/knowledge/debug/`:

1. **[01-overview.md](../.claude/knowledge/debug/01-overview.md)** - Architecture and quick start
2. **[02-telepresence-guide.md](../.claude/knowledge/debug/02-telepresence-guide.md)** - Telepresence detailed guide
3. **[03-skaffold-guide.md](../.claude/knowledge/debug/03-skaffold-guide.md)** - Skaffold detailed guide
4. **[04-migrations-debug.md](../.claude/knowledge/debug/04-migrations-debug.md)** - Database migrations
5. **[05-external-services.md](../.claude/knowledge/debug/05-external-services.md)** - PostgreSQL and SGROUPS
6. **[06-ide-setup.md](../.claude/knowledge/debug/06-ide-setup.md)** - GoLand and VSCode setup
7. **[07-troubleshooting.md](../.claude/knowledge/debug/07-troubleshooting.md)** - Common issues
8. **[08-debug-chain.md](../.claude/knowledge/debug/08-debug-chain.md)** - Full stack debugging (Frontend → APIServer → Backend)

## Common Tasks

### Debug Backend

```bash
# Telepresence
./telepresence/intercept-backend.sh

# Skaffold
cd skaffold && skaffold dev -p debug-backend
```

### Debug APIServer

```bash
# Telepresence
./telepresence/intercept-apiserver.sh

# Skaffold
cd skaffold && skaffold dev -p debug-apiserver
```

### Debug Webhook

```bash
# Telepresence
./telepresence/intercept-webhook.sh

# Skaffold
cd skaffold && skaffold dev -p debug-webhook
```

### Apply New Migration

```bash
# 1. Create migration
cd ../  # Go to project root
goose -dir migrations create my_migration sql

# 2. Edit migrations/NNN_my_migration.sql

# 3a. For Telepresence: Apply locally
export DATABASE_CONNECTION_URL="postgres://netguard:netguard@localhost:5432/netguard?sslmode=disable"
goose -dir migrations postgres "$DATABASE_CONNECTION_URL" up

# 3b. For Skaffold: Rebuild goose image
docker build -f .deploy/dockerfiles/Dockerfile.goose.debug -t netguard/goose:debug-$(date +%s) .
minikube -p incloud image load netguard/goose:debug-$(date +%s)
kubectl delete pod -l app.kubernetes.io/name=backend -n incloud-sgroups
```

### Connect to PostgreSQL

```bash
# Via Telepresence (auto port-forward)
./telepresence/intercept-backend.sh
# PostgreSQL available at localhost:5432

# Manual port-forward
kubectl port-forward svc/postgresql 5432:5432 -n incloud-sgroups

# Connect
psql postgres://netguard:netguard@localhost:5432/netguard
```

### Clean Up

```bash
# Stop Telepresence
# Ctrl+C in intercept script terminal
telepresence quit

# Stop Skaffold
# Ctrl+C in skaffold terminal
# (automatically runs skaffold delete)

# Delete all debug resources
kubectl delete deployment,service -l app.kubernetes.io/component=debug -n incloud-sgroups
```

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│              Minikube (incloud-sgroups)                  │
│  ┌────────────────────────────────────────────────────┐ │
│  │                                                    │ │
│  │  Backend ──▶ PostgreSQL                           │ │
│  │  :9090       :5432                                 │ │
│  │  :2345                                             │ │
│  │    │                                               │ │
│  │    └───▶ SGROUPS                                   │ │
│  │          :9006                                     │ │
│  │                                                    │ │
│  │  APIServer ──▶ Backend                             │ │
│  │  :8443                                             │ │
│  │  :2346                                             │ │
│  │                                                    │ │
│  │  Webhook ──▶ Backend                               │ │
│  │  :9443                                             │ │
│  │  :2347                                             │ │
│  └────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────┘
          ▲                          ▲
          │ Telepresence             │ Port Forward
          │ (Intercept)              │ (Debug Ports)
          │                          │
    ┌─────┴──────────────────────────┴───────┐
    │      Local Development Machine          │
    │  ┌────────────┐      ┌────────────┐    │
    │  │    IDE     │◀────▶│    Delve   │    │
    │  │ (Debugger) │      │   :2345    │    │
    │  └────────────┘      └────────────┘    │
    └─────────────────────────────────────────┘
```

## Debug Ports

| Service    | Application Port | Debug Port |
|------------|-----------------|------------|
| Backend    | 9090 (gRPC)     | 2345       |
| APIServer  | 8443 (HTTPS)    | 2346       |
| Webhook    | 9443 (HTTPS)    | 2347       |
| PostgreSQL | 5432            | -          |

## Tips

- **Use Telepresence** for rapid iteration and live debugging
- **Use Skaffold** when you need to test in real cluster environment
- **Check logs** if something doesn't work: `kubectl logs -f <pod> -n incloud-sgroups`
- **Read docs** in `.claude/knowledge/debug/` for detailed information

## Support

For issues:
1. Check [07-troubleshooting.md](../.claude/knowledge/debug/07-troubleshooting.md)
2. Review pod logs: `kubectl logs <pod> -n incloud-sgroups`
3. Check events: `kubectl get events -n incloud-sgroups --sort-by='.lastTimestamp'`
