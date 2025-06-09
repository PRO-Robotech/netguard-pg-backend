# Use official Golang image as builder
FROM golang:1.23-alpine AS builder

# Install necessary dependencies
RUN apk add --no-cache git make protoc curl unzip bash

# Set working directory
WORKDIR /app

# Copy go.mod and go.sum
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Download missing proto files into /usr/local/include directory
RUN mkdir -p /usr/local/include/google/protobuf && \
    curl -L https://raw.githubusercontent.com/protocolbuffers/protobuf/master/src/google/protobuf/empty.proto -o /usr/local/include/google/protobuf/empty.proto && \
    curl -L https://raw.githubusercontent.com/protocolbuffers/protobuf/master/src/google/protobuf/timestamp.proto -o /usr/local/include/google/protobuf/timestamp.proto && \
    curl -L https://raw.githubusercontent.com/protocolbuffers/protobuf/master/src/google/protobuf/descriptor.proto -o /usr/local/include/google/protobuf/descriptor.proto && \
    curl -L https://raw.githubusercontent.com/protocolbuffers/protobuf/master/src/google/protobuf/struct.proto -o /usr/local/include/google/protobuf/struct.proto && \
    mkdir -p /usr/local/include/protoc-gen-openapiv2/options && \
    curl -L https://raw.githubusercontent.com/grpc-ecosystem/grpc-gateway/master/protoc-gen-openapiv2/options/openapiv2.proto -o /usr/local/include/protoc-gen-openapiv2/options/openapiv2.proto && \
    curl -L https://raw.githubusercontent.com/grpc-ecosystem/grpc-gateway/master/protoc-gen-openapiv2/options/annotations.proto -o /usr/local/include/protoc-gen-openapiv2/options/annotations.proto

# Install protoc-gen-go and other required plugins
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest && \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest && \
    go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@latest && \
    go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2@latest && \
    go install connectrpc.com/connect/cmd/protoc-gen-connect-go@latest

# Generate API from proto files
RUN cd protos && make generate-api

# Build the application
RUN go build -o netguard-server ./cmd/server

# Use minimal alpine image for runtime
FROM alpine:latest

# Install necessary dependencies
RUN apk --no-cache add ca-certificates

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/netguard-server .

# Copy Swagger UI files
COPY --from=builder /app/swagger-ui ./swagger-ui

# Expose ports for gRPC and HTTP servers
EXPOSE 9090 8080

# Run the server with in-memory database by default
ENTRYPOINT ["./netguard-server", "--memory", "--grpc-addr=:9090", "--http-addr=:8080"]
