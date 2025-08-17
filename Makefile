.PHONY: build test clean run release help

# Variables
BINARY_NAME=webpimg
GO_FILES=$(shell find . -name '*.go' -not -path "./test_*.go")
TEST_FILES=$(shell find . -name 'test_*.go')
VERSION=$(shell git describe --tags --always --dirty)
LDFLAGS=-ldflags "-s -w -X main.Version=${VERSION}"

# Colors for output
RED=\033[0;31m
GREEN=\033[0;32m
YELLOW=\033[1;33m
NC=\033[0m # No Color

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  ${GREEN}%-15s${NC} %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the binary
	@echo "Building ${BINARY_NAME}..."
	@go build ${LDFLAGS} -o ${BINARY_NAME} main.go
	@echo "${GREEN}✓ Build complete${NC}"

run: build ## Build and run the service
	@echo "Starting ${BINARY_NAME}..."
	@./${BINARY_NAME}

test: ## Run all tests
	@echo "Running tests..."
	@./run_tests.sh

test-quick: ## Run quick tests without starting server
	@echo "Running quick tests..."
	@go vet ./...
	@go fmt ./...
	@echo "${GREEN}✓ Quick tests passed${NC}"

test-integration: build ## Run integration tests
	@echo "Starting integration tests..."
	@echo "test123" > .pass
	@./${BINARY_NAME} & SERVER_PID=$$!; \
	sleep 3; \
	./run_tests.sh; \
	TEST_RESULT=$$?; \
	kill $$SERVER_PID 2>/dev/null || true; \
	exit $$TEST_RESULT

test-coverage: ## Run tests with coverage
	@echo "Running tests with coverage..."
	@go test -v -cover -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "${GREEN}✓ Coverage report generated: coverage.html${NC}"

benchmark: build ## Run benchmarks
	@echo "Running benchmarks..."
	@./${BINARY_NAME} & SERVER_PID=$$!; \
	sleep 3; \
	ab -n 1000 -c 10 http://localhost:8080/https://via.placeholder.com/100; \
	kill $$SERVER_PID 2>/dev/null || true

clean: ## Clean build artifacts
	@echo "Cleaning..."
	@rm -f ${BINARY_NAME}
	@rm -f ${BINARY_NAME}-*
	@rm -f *.tar.gz *.zip
	@rm -f test-report.md test-output.log
	@rm -f coverage.out coverage.html
	@rm -rf cache/
	@rm -f imgproxy.db
	@echo "${GREEN}✓ Clean complete${NC}"

deps: ## Download dependencies
	@echo "Downloading dependencies..."
	@go mod download
	@go mod verify
	@echo "${GREEN}✓ Dependencies installed${NC}"

update-io: ## Update io dependency to latest version
	@echo "Updating io dependency..."
	@scripts/update-io.sh

deps-update: ## Update all dependencies
	@echo "Updating all dependencies..."
	@go get -u ./...
	@go mod tidy
	@echo "${GREEN}✓ Dependencies updated${NC}"

deps-check: ## Check for outdated dependencies
	@echo "Checking dependencies..."
	@go list -u -m all | grep -v "indirect$$" || echo "${GREEN}✓ All dependencies up to date${NC}"

fmt: ## Format code
	@echo "Formatting code..."
	@go fmt ./...
	@echo "${GREEN}✓ Code formatted${NC}"

lint: ## Run linters
	@echo "Running linters..."
	@go vet ./...
	@golangci-lint run || true
	@echo "${GREEN}✓ Linting complete${NC}"

docker-build: ## Build Docker image
	@echo "Building Docker image..."
	@docker build -t ${BINARY_NAME}:${VERSION} .
	@docker tag ${BINARY_NAME}:${VERSION} ${BINARY_NAME}:latest
	@echo "${GREEN}✓ Docker image built${NC}"

docker-run: docker-build ## Run in Docker
	@echo "Running in Docker..."
	@docker run -p 8080:8080 ${BINARY_NAME}:latest

release-dry: ## Dry run of release process
	@echo "Release dry run for version ${VERSION}..."
	@goreleaser release --snapshot --skip=publish --clean

release: ## Create a new release
	@echo "Creating release ${VERSION}..."
	@if [ -z "$$(git status --porcelain)" ]; then \
		goreleaser release --clean; \
	else \
		echo "${RED}✗ Working directory is not clean${NC}"; \
		exit 1; \
	fi

# Platform-specific builds
build-all: ## Build for all platforms
	@echo "Building for all platforms..."
	@mkdir -p dist
	# Windows
	@GOOS=windows GOARCH=amd64 go build ${LDFLAGS} -o dist/${BINARY_NAME}-windows-amd64.exe main.go
	@GOOS=windows GOARCH=arm64 go build ${LDFLAGS} -o dist/${BINARY_NAME}-windows-arm64.exe main.go
	# Linux
	@GOOS=linux GOARCH=amd64 go build ${LDFLAGS} -o dist/${BINARY_NAME}-linux-amd64 main.go
	@GOOS=linux GOARCH=arm64 go build ${LDFLAGS} -o dist/${BINARY_NAME}-linux-arm64 main.go
	@GOOS=linux GOARCH=arm GOARM=7 go build ${LDFLAGS} -o dist/${BINARY_NAME}-linux-armv7 main.go
	# macOS
	@GOOS=darwin GOARCH=amd64 go build ${LDFLAGS} -o dist/${BINARY_NAME}-darwin-amd64 main.go
	@GOOS=darwin GOARCH=arm64 go build ${LDFLAGS} -o dist/${BINARY_NAME}-darwin-arm64 main.go
	@echo "${GREEN}✓ All platforms built${NC}"

install: build ## Install binary to GOPATH/bin
	@echo "Installing ${BINARY_NAME}..."
	@go install ${LDFLAGS}
	@echo "${GREEN}✓ Installed to $(GOPATH)/bin/${BINARY_NAME}${NC}"

uninstall: ## Uninstall binary from GOPATH/bin
	@echo "Uninstalling ${BINARY_NAME}..."
	@rm -f $(GOPATH)/bin/${BINARY_NAME}
	@echo "${GREEN}✓ Uninstalled${NC}"

.DEFAULT_GOAL := help