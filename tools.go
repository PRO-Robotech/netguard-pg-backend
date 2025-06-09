//go:build tools
// +build tools

// This file is used to track tool dependencies.
// It is not included in the build.
//
// Tools required:
// - google.golang.org/grpc/cmd/protoc-gen-go-grpc
// - google.golang.org/protobuf/cmd/protoc-gen-go
// - connectrpc.com/connect/cmd/protoc-gen-connect-go
// - github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway
// - github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2
package tools

import (
	_ "connectrpc.com/connect"
	_ "github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	_ "github.com/grpc-ecosystem/grpc-gateway/v2/utilities"
)
