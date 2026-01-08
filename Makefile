.PHONY: all build build-server proto clean install test help

# Binary names
BINARY=burnafter
BINARY_SERVER=burnafter-server

# Build directory
BUILD_DIR=.

# Embedded server binary directory
EMBED_DIR=internal/client/embedded

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

all: proto build-server build

build: ## Build the client binary
	$(GOBUILD) -o $(BUILD_DIR)/$(BINARY) ./cmd/burnafter

build-servers: ## Build server binaries for all platforms (compressed)
	./hack/build-server-all-platforms.sh

proto: ## Generate protobuf code
	./hack/genproto.sh

clean: ## Remove build artifacts
	$(GOCLEAN)
	rm -f $(BUILD_DIR)/$(BINARY)

install: build ## Install the binary to $GOPATH/bin
	$(GOCMD) install ./cmd/burnafter

test: ## Run tests
	$(GOTEST) -v ./...

tidy: ## Tidy go modules
	$(GOMOD) tidy

help: ## Display this help message
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
