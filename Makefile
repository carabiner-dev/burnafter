.PHONY: all build proto clean install test help

# Binary name
BINARY=burnafter

# Build directory
BUILD_DIR=.

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

all: proto build

build: ## Build the binary
	$(GOBUILD) -o $(BUILD_DIR)/$(BINARY) ./cmd/burnafter

proto: ## Generate protobuf code
	./generate.sh

clean: ## Remove build artifacts
	$(GOCLEAN)
	rm -f $(BUILD_DIR)/$(BINARY)
	rm -f /tmp/burnafter.sock

install: build ## Install the binary to $GOPATH/bin
	$(GOCMD) install ./cmd/burnafter

test: ## Run tests
	$(GOTEST) -v ./...

tidy: ## Tidy go modules
	$(GOMOD) tidy

run-server: build ## Run the server in debug mode
	./$(BINARY) -debug server

help: ## Display this help message
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
