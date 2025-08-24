.PHONY: test
test: test-unit test-integration test-e2e

.PHONY: test-unit
test-unit:
	go test -v ./internal/domain/...

.PHONY: test-integration
test-integration: test-mem test-pg

.PHONY: test-mem
test-mem:
	go test -v ./internal/infrastructure/repositories/mem/...

.PHONY: test-pg
test-pg:
	@echo "🐘 Running PostgreSQL repository tests..."
	TEST_PG_URI="postgres://postgres:postgres@localhost:5432/netguard_test?sslmode=disable" go test -v ./internal/infrastructure/repositories/pg/...

.PHONY: test-e2e
test-e2e:
	go test -v ./internal/api/...
	go test -v ./internal/app/...
	go test -v ./internal/application/...

.PHONY: test-coverage
test-coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# K8s API Server targets
.PHONY: generate-k8s
generate-k8s:
	./hack/k8s/update-codegen.sh

.PHONY: build-k8s-apiserver
build-k8s-apiserver:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
		-a -installsuffix cgo \
		-ldflags='-w -s -extldflags "-static"' \
		-o bin/k8s-apiserver \
		./cmd/k8s-apiserver

.PHONY: docker-build-k8s-apiserver
docker-build-k8s-apiserver:
	docker build -f Dockerfile.apiserver -t netguard/k8s-apiserver:latest .

.PHONY: docker-push-k8s-apiserver
docker-push-k8s-apiserver:
	docker push netguard/k8s-apiserver:latest

.PHONY: test-k8s-unit
test-k8s-unit:
	go test -v ./internal/k8s/...

.PHONY: test-k8s-integration
test-k8s-integration:
	go test -v ./test/k8s/integration/...

.PHONY: test-k8s-e2e
test-k8s-e2e:
	go test -v ./test/k8s/e2e/...

.PHONY: deploy-k8s
deploy-k8s:
	kubectl apply -k config/k8s/

.PHONY: undeploy-k8s
undeploy-k8s:
	kubectl delete -k config/k8s/

.PHONY: logs-k8s
logs-k8s:
	kubectl logs -f deployment/netguard-apiserver -n netguard-system

# Separate Deployment Flows: Memory vs PostgreSQL
.PHONY: deploy-memory
deploy-memory: ## Deploy NetGuard with in-memory backend (fast development)
	@echo "🧠 Deploying NetGuard with in-memory backend..."
	kubectl apply -k config/k8s/overlays/memory

.PHONY: deploy-postgresql  
deploy-postgresql: ## Deploy NetGuard with PostgreSQL backend (production-ready)
	@echo "🐘 Deploying NetGuard with PostgreSQL backend..."
	kubectl apply -k config/k8s/overlays/postgresql

.PHONY: switch-to-memory
switch-to-memory: ## Switch from PostgreSQL to memory mode
	@echo "🔄 Switching to memory mode..."
	kubectl delete -k config/k8s/overlays/postgresql || true
	@echo "⏳ Waiting for cleanup..."
	@sleep 5
	kubectl apply -k config/k8s/overlays/memory
	@echo "✅ Switched to memory mode!"

.PHONY: switch-to-postgresql
switch-to-postgresql: ## Switch from memory to PostgreSQL mode
	@echo "🔄 Switching to PostgreSQL mode..."
	kubectl delete -k config/k8s/overlays/memory || true
	@echo "⏳ Waiting for cleanup..."
	@sleep 5
	kubectl apply -k config/k8s/overlays/postgresql
	@echo "✅ Switched to PostgreSQL mode!"

.PHONY: clean-memory
clean-memory: ## Remove memory deployment
	@echo "🗑️ Removing memory deployment..."
	kubectl delete -k config/k8s/overlays/memory

.PHONY: clean-postgresql
clean-postgresql: ## Remove PostgreSQL deployment
	@echo "🗑️ Removing PostgreSQL deployment..."
	kubectl delete -k config/k8s/overlays/postgresql

.PHONY: status-deployment
status-deployment: ## Check deployment status
	@echo "📊 Checking deployment status..."
	kubectl get pods,svc,deployment,statefulset -n netguard-system
	@echo "\n🏷️ Deployment labels:"
	kubectl get deployment -n netguard-system --show-labels

.PHONY: logs-backend
logs-backend: ## Show backend logs
	@echo "📜 Backend logs:"
	kubectl logs -f deployment/netguard-backend -n netguard-system

.PHONY: logs-postgresql
logs-postgresql: ## Show PostgreSQL logs (if deployed)
	@echo "🐘 PostgreSQL logs:"
	kubectl logs -f statefulset/postgresql -n netguard-system

.PHONY: migrate
migrate:
	go run cmd/migrate/main.go --pg-uri="postgres://postgres:postgres@localhost:5432/netguard?sslmode=disable"

.PHONY: run
run:
	go run cmd/server/main.go --memory

.PHONY: run-pg
run-pg:
	go run cmd/server/main.go --pg-uri="postgres://postgres:postgres@localhost:5432/netguard?sslmode=disable" --migrate

.PHONY: build
build:
	go build -o bin/netguard-server cmd/server/main.go

.PHONY: docker-build
docker-build:
	docker build -t netguard-pg-backend .

.PHONY: docker-run
docker-run:
	docker run -p 8080:8080 -p 9090:9090 netguard-pg-backend

.PHONY: docker-compose-up
docker-compose-up:
	docker-compose up

.PHONY: docker-compose-down
docker-compose-down:
	docker-compose down

.PHONY: clean
clean:
	rm -rf bin
	rm -f coverage.out coverage.html

# Build targets
.PHONY: build-apiserver
build-apiserver: ## Build API server binary
	@echo "🔨 Building API server..."
	go build -o bin/k8s-apiserver ./cmd/k8s-apiserver/

.PHONY: run-apiserver
run-apiserver: build-apiserver ## Build and run API server locally
	@echo "🚀 Starting API server..."
	./bin/k8s-apiserver --help

.PHONY: test-apiserver
test-apiserver: build-apiserver ## Build and test API server with basic flags
	@echo "🧪 Testing API server..."
	./bin/k8s-apiserver --logtostderr=true --v=2 --secure-port=8443 --cert-dir=/tmp

# Backend image
.PHONY: docker-build-pg-backend
docker-build-pg-backend:
	docker build -f Dockerfile.backend -t netguard/pg-backend:latest .

# PostgreSQL Development Commands

# Goose Migration System (sgroups pattern)
GOOSE_REPO:=https://github.com/pressly/goose
GOOSE_LATEST_VERSION:=v3.23.2
GOOSE:=./bin/goose
GOBIN:=./bin
GO:=go
ifneq ($(wildcard $(GOOSE)),)
	GOOSE_CUR_VERSION?=$(shell $(GOOSE) -version|egrep -o "v[0-9\.]+")	
else
	GOOSE_CUR_VERSION?=
endif

PG_MIGRATIONS?=./internal/infrastructure/repositories/pg/migrations
PG_URI?=

.PHONY: .install-goose
.install-goose: 
	@echo installing \'goose\' $(GOOSE_LATEST_VERSION) util... && \
	mkdir -p $(GOBIN) && \
	GOBIN=$(GOBIN) $(GO) install github.com/pressly/goose/v3/cmd/goose@$(GOOSE_LATEST_VERSION)

.PHONY: netguard-pg-migrations
netguard-pg-migrations: ## Run NetGuard PostgreSQL migrations
ifneq ($(PG_URI),)
	@$(MAKE) .install-goose && \
	cd $(PG_MIGRATIONS) && \
	$(GOOSE) -table=netguard_db_ver postgres $(PG_URI) up
else
	$(error need define PG_URI environment variable)
endif

.PHONY: docker-build-goose
docker-build-goose: ## Build Goose migration container
	@echo "🐘 Building Goose migration container..."
	docker build -f Dockerfile.goose -t netguard/goose:latest .

.PHONY: pg-setup
pg-setup: ## Setup PostgreSQL development environment
	@echo "🐘 Setting up PostgreSQL development environment..."
	docker run --name netguard-postgres -d -p 5432:5432 -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=netguard postgres:15
	@echo "⏳ Waiting for PostgreSQL to be ready..."
	@sleep 5
	@echo "🗄️ Creating test database..."
	docker exec netguard-postgres createdb -U postgres netguard_test || true

.PHONY: pg-stop
pg-stop: ## Stop PostgreSQL development environment
	@echo "🛑 Stopping PostgreSQL development environment..."
	docker stop netguard-postgres || true
	docker rm netguard-postgres || true

.PHONY: pg-migrate
pg-migrate: ## Run PostgreSQL migrations
	@echo "🗄️ Running PostgreSQL migrations..."
	go run cmd/server/main.go --pg-uri="postgres://postgres:postgres@localhost:5432/netguard?sslmode=disable" --migrate --memory=false

.PHONY: pg-migrate-test
pg-migrate-test: ## Run PostgreSQL test migrations
	@echo "🧪 Running PostgreSQL test migrations..."
	go run cmd/server/main.go --pg-uri="postgres://postgres:postgres@localhost:5432/netguard_test?sslmode=disable" --migrate --memory=false

.PHONY: pg-status
pg-status: ## Check PostgreSQL connection status
	@echo "🔍 Checking PostgreSQL status..."
	@docker exec netguard-postgres pg_isready -U postgres || echo "❌ PostgreSQL not ready"

.PHONY: pg-logs
pg-logs: ## Show PostgreSQL logs
	@echo "📜 PostgreSQL logs:"
	docker logs netguard-postgres --tail=50

.PHONY: pg-shell
pg-shell: ## Open PostgreSQL shell
	@echo "🐚 Opening PostgreSQL shell..."
	docker exec -it netguard-postgres psql -U postgres -d netguard

.PHONY: pg-reset
pg-reset: pg-stop pg-setup pg-migrate ## Reset PostgreSQL development environment
	@echo "♻️ PostgreSQL environment reset complete!"

.PHONY: dev-pg
dev-pg: pg-setup pg-migrate ## Start development with PostgreSQL backend
	@echo "🚀 Starting development server with PostgreSQL..."
	go run cmd/server/main.go --pg-uri="postgres://postgres:postgres@localhost:5432/netguard?sslmode=disable"

.PHONY: test-pg-integration
test-pg-integration: pg-setup pg-migrate-test ## Run full PostgreSQL integration tests
	@echo "🧪 Running PostgreSQL integration tests..."
	TEST_PG_URI="postgres://postgres:postgres@localhost:5432/netguard_test?sslmode=disable" go test -v -tags integration ./internal/infrastructure/repositories/pg/...

.PHONY: benchmark-pg
benchmark-pg: pg-setup pg-migrate-test ## Run PostgreSQL performance benchmarks
	@echo "⚡ Running PostgreSQL benchmarks..."
	TEST_PG_URI="postgres://postgres:postgres@localhost:5432/netguard_test?sslmode=disable" go test -v -bench=. -benchtime=10s ./internal/infrastructure/repositories/pg/...