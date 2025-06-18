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