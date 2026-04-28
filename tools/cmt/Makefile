.PHONY: help
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Available targets:"
	@echo "  test              Run all tests"
	@echo "  test-parallel     Run all tests with explicit high parallelism"
	@echo "  test-verbose      Run all tests with verbose output"
	@echo "  test-race         Run all tests with race detector"
	@echo "  generate          Run go generate for codegen"
	@echo "  mocks             Generate all gomock mocks"
	@echo "  lint              Run golangci-lint"
	@echo "  build             Build the cmt binary"

# Run all tests (parallel by default via t.Parallel())
.PHONY: test
test:
	go test ./... -count=1

.PHONY: generate
generate:
	go generate ./...

.PHONY: mocks
mocks: generate

# Run golangci-lint
.PHONY: lint
lint:
	go fmt ./...
	golangci-lint run --config .golangci.yml --fix ./...

# Build the cmt binary
.PHONY: build
build:
	go build -o cmt .

.DEFAULT_GOAL := help
