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
	go test -v ./internal/infrastructure/repositories/pg/...

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
	docker build -f config/docker/Dockerfile.k8s-apiserver -t netguard/k8s-apiserver:latest .

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