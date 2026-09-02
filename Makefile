.PHONY: build test test-go test-python lint lint-go lint-python fmt proto venv clean

GO_BIN     := gateway
PYTHON     ?= python3
VENV       := .venv
VENV_PYTHON := $(VENV)/bin/python3

## Build the gateway binary into ./gateway
build:
	go build -o $(GO_BIN) ./cmd/gateway

## Run the full test suite (Go + Python). Spawns real Python worker processes, so it needs
## handlers-sdk installed into $(VENV) — run `make venv` once first.
test: test-go test-python

## Go unit/integration/contract tests. GATEWAY_TEST_PYTHON points at the venv so tests can
## launch real handler workers; override it to test against a different interpreter.
test-go:
	GATEWAY_TEST_PYTHON=$(abspath $(VENV_PYTHON)) go test ./... -timeout 60s

## Python SDK's own pytest suite
test-python: venv
	$(VENV_PYTHON) -m pytest python/tests

## Lint everything
lint: lint-go lint-python

lint-go:
	go vet ./...
	golangci-lint run

lint-python: venv
	$(VENV_PYTHON) -m ruff check handlers-sdk python

## Format Go and Python sources (excludes generated protobuf/grpc code)
fmt: venv
	gofmt -w $$(find . -name '*.go' -not -path './api/handler/v1/*')
	$(VENV_PYTHON) -m ruff format handlers-sdk python

## Regenerate Go + Python gRPC stubs from api/handler/v1/handler.proto. Requires protoc plus
## the protoc-gen-go/protoc-gen-go-grpc plugins on $PATH (`go install
## google.golang.org/protobuf/cmd/protoc-gen-go@latest` and
## `go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest`).
proto: venv
	protoc --go_out=. --go_opt=paths=source_relative \
	       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
	       api/handler/v1/handler.proto
	$(VENV_PYTHON) -m grpc_tools.protoc -I api/handler/v1 \
	       --python_out=handlers-sdk/gateway_handler_sdk/v1 \
	       --grpc_python_out=handlers-sdk/gateway_handler_sdk/v1 \
	       --pyi_out=handlers-sdk/gateway_handler_sdk/v1 \
	       api/handler/v1/handler.proto
	sed -i.bak 's/^import handler_pb2 as handler__pb2$$/from . import handler_pb2 as handler__pb2/' \
	       handlers-sdk/gateway_handler_sdk/v1/handler_pb2_grpc.py
	rm -f handlers-sdk/gateway_handler_sdk/v1/handler_pb2_grpc.py.bak
	@echo "Regenerated stubs — review the diff before committing."

## Create/refresh the local dev virtualenv with handlers-sdk installed in editable mode
venv:
	@test -d $(VENV) || $(PYTHON) -m venv $(VENV)
	$(VENV_PYTHON) -m pip install --quiet --upgrade pip
	$(VENV_PYTHON) -m pip install --quiet -e './handlers-sdk[dev]'

clean:
	rm -f $(GO_BIN)
	go clean -cache
	rm -rf $(VENV)
