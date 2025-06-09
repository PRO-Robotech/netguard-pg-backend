SHELL:=/bin/sh
export GOSUMDB=off
export GO111MODULE=on
$(value $(shell [ ! -d "$(CURDIR)/bin" ] && mkdir -p "$(CURDIR)/bin"))
export GOBIN=$(CURDIR)/bin
GO?=$(shell which go)

.PHONY: .grpc-plugins
.grpc-plugins:
	@echo - ensure GRPC plugins are installed
ifeq ($(wildcard $(GOBIN)/protoc-gen-grpc-gateway),)
	@echo install \"protoc-gen-grpc-gateway\" && \
	$(GO) install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway
endif
ifeq ($(wildcard $(GOBIN)/protoc-gen-openapiv2),)
	@echo install \"protoc-gen-openapiv2\" && \
	$(GO) install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2
endif
ifeq ($(wildcard $(GOBIN)/protoc-gen-go),)
	@echo install \"protoc-gen-go\" && \
	$(GO) install google.golang.org/protobuf/cmd/protoc-gen-go
endif
ifeq ($(wildcard $(GOBIN)/protoc-gen-go-grpc),)
	@echo install \"protoc-gen-go-grpc\" && \
	$(GO) install google.golang.org/grpc/cmd/protoc-gen-go-grpc
endif
ifeq ($(wildcard $(GOBIN)/protoc-gen-connect-go),)
	@echo install \"protoc-gen-connect-go\" && \
	$(GO) install connectrpc.com/connect/cmd/protoc-gen-connect-go@latest
endif

proto_dirs := netguard common
.PHONY: generate-api
generate-api: | .grpc-plugins
	@(\
	apis=$(CURDIR)/api && \
	dest=$(CURDIR)/pkg/api && \
	PATH="$(PATH):$(GOBIN):/usr/include:/usr/local/include:$(go env GOPATH)/bin" && \
	rm -rf $$dest 2>/dev/null && \
	mkdir -p $$dest && \
	echo generating API in \"$$dest\" ... && \
	for p in $(proto_dirs); do \
		for v in $$apis/$$p/*.proto; do \
			echo  "  - " \"$$p/$$(basename $$v)\" ;\
   protoc \
                                -I $(CURDIR)/vendor/github.com/grpc-ecosystem/grpc-gateway/v2/ \
                                -I $(CURDIR)/3d-party \
                                -I /usr/local/include \
				--go_opt=paths=source_relative \
				--go-grpc_opt=paths=source_relative \
				--go_out $$dest \
				--go-grpc_out $$dest \
				--proto_path=$$apis \
				--grpc-gateway_out $$dest \
				--grpc-gateway_opt paths=source_relative \
				--grpc-gateway_opt logtostderr=true \
				--grpc-gateway_opt standalone=false \
				--openapiv2_out $$dest \
				--openapiv2_opt logtostderr=true \
				--connect-go_out=$$dest \
				--connect-go_opt=paths=source_relative \
				"$$v" ||\
			exit 1;\
		done; \
	done; \
	echo -=OK=- ;\
	)
	# Copy Swagger JSON file to swagger-ui directory
	cd .. && bash copy-swagger.sh
