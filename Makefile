# Makefile orchestrates codegen, build/test, and the T0..T3 demo.
#
# The demo starts the modern server and runs both example clients:
#   - client/legacy: compiled from the frozen 1.0 proto snapshot (auto-pins 1.0)
#   - client/modern: compiled from the full 2.1 proto snapshot (auto-pins 2.1)
# It then fetches /metrics to showcase the interceptor's per-call counters.
# The example lives in its own nested Go module (example/go.mod) so the library
# module does not carry the example's dependencies (e.g. golang.org/x/net).

SHELL := /bin/bash
ADDR := :8080

.PHONY: all generate build vet test cover fmt run-server run-grpc-server legacy-example modern-example grpc-example metrics demo grpc-demo clean



all: generate build vet test

## generate: run buf codegen. The library generates only the reusable
## annotations; the example generates two greeter snapshots into the example
## module: legacy (frozen 1.0) and modern (2.1). The modern snapshot is both the
## up-to-date client contract AND what the example servers implement.
generate:
	buf generate
	cd example/proto/legacy && buf generate
	cd example/proto/modern && buf generate

## build: compile the library and the example module.
build:
	go build ./...
	cd example && go build ./...

## vet: run go vet on both modules.
vet:
	go vet ./...
	cd example && go vet ./...

## test: run the unit tests for both modules.
test:
	go test ./...
	cd example && go test ./...

## cover: run tests with coverage for the core module and each nested module.
## Generated protobuf code is excluded from the profile so the numbers reflect
## hand-written code. The core library is gated at a minimum coverage threshold
## (MIN_COVER).
MIN_COVER ?= 70.0
cover:
	@set -euo pipefail; \
	fail=0; \
	for m in . router/connect router/grpc autopin/connect autopin/grpc; do \
	  echo ">> coverage: $$m"; \
	  ( cd "$$m" && go test -covermode=atomic -coverprofile=/tmp/vi-cover.out ./... ); \
	  head -n1 /tmp/vi-cover.out > /tmp/vi-cover.final; \
	  grep -vE '^mode:|\.pb\.go:|/gen/|/testfixture/' /tmp/vi-cover.out >> /tmp/vi-cover.final || true; \
	  pct="$$(cd "$$m" && go tool cover -func=/tmp/vi-cover.final | awk '/^total:/ {gsub(/%/,"",$$3); print $$3}')"; \
	  echo "   total (excl. generated): $$pct%"; \
	  if [ "$$m" = "." ]; then \
	    below="$$(awk -v p="$$pct" -v min="$(MIN_COVER)" 'BEGIN { print (p < min) ? 1 : 0 }')"; \
	    if [ "$$below" = "1" ]; then \
	      echo "   FAIL: core coverage $$pct% < $(MIN_COVER)% threshold"; fail=1; \
	    fi; \
	  fi; \
	done; \
	exit $$fail

## fmt: format all Go sources in both modules.
fmt:
	gofmt -w .

## run-server: run the modern Connect server in the foreground.
run-server:
	cd example && go run ./server

## run-grpc-server: run the raw-gRPC server in the foreground (:8081).
run-grpc-server:
	cd example && go run ./grpcserver

## legacy-example: run the T1 (frozen 1.0) client. Requires a running server.
legacy-example:
	cd example && go run ./client/legacy

## modern-example: run the T3 (2.1) client. Requires a running server.
modern-example:
	cd example && go run ./client/modern

## grpc-example: run the raw-gRPC client. Requires a running gRPC server.
grpc-example:
	cd example && go run ./client/grpc

## metrics: fetch the interceptor's Prometheus metrics. Requires a running server.
metrics:
	@curl -fsS "http://localhost$(ADDR)/metrics" | grep version_intercept_requests_total

## demo: start the server, run both clients, showcase /metrics, then tear down.
demo:
	@echo ">> building..."
	@cd example && go build -o /tmp/vi-server ./server
	@echo ">> starting server on $(ADDR)..."
	@/tmp/vi-server & echo $$! > /tmp/vi-server.pid; \
	trap 'kill $$(cat /tmp/vi-server.pid) 2>/dev/null; rm -f /tmp/vi-server.pid /tmp/vi-server' EXIT; \
	for i in $$(seq 1 50); do \
	  if curl -fsS -o /dev/null "http://localhost$(ADDR)/" 2>/dev/null; then break; fi; \
	  sleep 0.1; \
	done; \
	echo; echo "==================== client/legacy (T1) ===================="; \
	(cd example && go run ./client/legacy); \
	echo; echo "==================== client/modern (T3) ===================="; \
	(cd example && go run ./client/modern); \
	echo; echo "==================== interceptor metrics (/metrics) ===================="; \
	echo "recorded per-call counters (service / method / api_version / code):"; \
	curl -fsS "http://localhost$(ADDR)/metrics" | grep version_intercept_requests_total; \
	echo; echo ">> raw-gRPC adapter demo (routergrpc + autopingrpc)..."; \
	$(MAKE) --no-print-directory grpc-demo

## grpc-demo: start the raw-gRPC server, run the raw-gRPC client, then tear down.
## Demonstrates the routergrpc server interceptors + autopingrpc client
## interceptors (auto-pin + explicit override) over google.golang.org/grpc.
GRPC_ADDR := :8081
GRPC_METRICS_ADDR := :8082
grpc-demo:
	@echo ">> building gRPC server..."
	@cd example && go build -o /tmp/vi-grpcserver ./grpcserver
	@echo ">> starting gRPC server on $(GRPC_ADDR) (metrics on $(GRPC_METRICS_ADDR))..."
	@GRPC_ADDR=$(GRPC_ADDR) METRICS_ADDR=$(GRPC_METRICS_ADDR) /tmp/vi-grpcserver & echo $$! > /tmp/vi-grpcserver.pid; \
	trap 'kill $$(cat /tmp/vi-grpcserver.pid) 2>/dev/null; rm -f /tmp/vi-grpcserver.pid /tmp/vi-grpcserver' EXIT; \
	for i in $$(seq 1 50); do \
	  if curl -fsS -o /dev/null "http://localhost$(GRPC_METRICS_ADDR)/metrics" 2>/dev/null; then break; fi; \
	  sleep 0.1; \
	done; \
	echo; echo "==================== client/grpc (raw gRPC) ===================="; \
	(cd example && go run ./client/grpc); \
	echo; echo "==================== interceptor metrics (gRPC, /metrics) ===================="; \
	echo "recorded per-call counters (service / method / api_version / code):"; \
	curl -fsS "http://localhost$(GRPC_METRICS_ADDR)/metrics" | grep version_intercept_requests_total

## clean: remove build artifacts.
clean:
	rm -f /tmp/vi-server /tmp/vi-server.pid /tmp/vi-grpcserver /tmp/vi-grpcserver.pid
