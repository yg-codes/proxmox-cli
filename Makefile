# Proxmox Admin CLI - Makefile

# Build variables
BINARY_NAME=pve
# Deliberately NOT --abbrev=0: that returns the nearest tag only, so a build
# from a commit AFTER v1.10.0 still claims to be "v1.10.0" — indistinguishable
# from the release artifact. Plain `git describe` appends -<n>-g<sha> when HEAD
# is not exactly on a tag, and --dirty flags uncommitted changes, so a local
# build can never be mistaken for a released one. Goreleaser sets its own
# version, so the published release binaries are unaffected by this.
#
# Side effect: `make release` archive names and the docker tag also carry the
# describe string, so a manual build off-tag produces e.g.
# pve-v1.10.0-1-gabc1234-linux-amd64.tar.gz. That is intended — an off-tag
# archive should not be named as though it were the tagged release. Build from
# a clean checkout of the tag when the plain name matters.
VERSION=$(shell git describe --tags --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS=-ldflags "-X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${BUILD_TIME}"

# Go variables
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Directories
BUILD_DIR=build
CMD_DIR=pve

.PHONY: all build clean test deps fmt vet lint install uninstall run help

# Default target
all: clean deps fmt vet build

# Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./$(CMD_DIR)

# Build for multiple platforms
build-all: clean deps
	@echo "Building for multiple platforms..."
	@mkdir -p $(BUILD_DIR)
	
	# Linux AMD64
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./$(CMD_DIR)
	
	# Linux ARM64
	GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 ./$(CMD_DIR)
	
	# macOS AMD64
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 ./$(CMD_DIR)
	
	# macOS ARM64 (Apple Silicon)
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 ./$(CMD_DIR)
	
	# Windows AMD64
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe ./$(CMD_DIR)
	
	@echo "Cross-compilation complete. Binaries in $(BUILD_DIR)/"

# Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html
	rm -f *.test
	@echo "✓ All build artifacts cleaned"

# Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

# Format code
fmt:
	@echo "Formatting code..."
	$(GOCMD) fmt ./...

# Run go vet
vet:
	@echo "Running go vet..."
	$(GOCMD) vet ./...

# Run golangci-lint (requires golangci-lint to be installed)
lint:
	@echo "Running golangci-lint..."
	@which golangci-lint > /dev/null || (echo "golangci-lint not found. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest" && exit 1)
	golangci-lint run

# Install binary to GOPATH/bin
install: build
	@echo "Installing $(BINARY_NAME) to $(shell go env GOPATH)/bin..."
	cp $(BUILD_DIR)/$(BINARY_NAME) $(shell go env GOPATH)/bin/

# Uninstall binary from GOPATH/bin
uninstall:
	@echo "Uninstalling $(BINARY_NAME) from $(shell go env GOPATH)/bin..."
	rm -f $(shell go env GOPATH)/bin/$(BINARY_NAME)

# Run the application
run:
	@echo "Running $(BINARY_NAME)..."
	$(GOCMD) run ./$(CMD_DIR) $(ARGS)

# Run in development mode with verbose logging
dev:
	@echo "Running $(BINARY_NAME) in development mode..."
	$(GOCMD) run ./$(CMD_DIR) --verbose $(ARGS)

# Create release archives
release: build-all
	@echo "Creating release archives..."
	@mkdir -p $(BUILD_DIR)/release
	
	# Linux AMD64
	tar -czf $(BUILD_DIR)/release/$(BINARY_NAME)-$(VERSION)-linux-amd64.tar.gz -C $(BUILD_DIR) $(BINARY_NAME)-linux-amd64
	
	# Linux ARM64
	tar -czf $(BUILD_DIR)/release/$(BINARY_NAME)-$(VERSION)-linux-arm64.tar.gz -C $(BUILD_DIR) $(BINARY_NAME)-linux-arm64
	
	# macOS AMD64
	tar -czf $(BUILD_DIR)/release/$(BINARY_NAME)-$(VERSION)-darwin-amd64.tar.gz -C $(BUILD_DIR) $(BINARY_NAME)-darwin-amd64
	
	# macOS ARM64
	tar -czf $(BUILD_DIR)/release/$(BINARY_NAME)-$(VERSION)-darwin-arm64.tar.gz -C $(BUILD_DIR) $(BINARY_NAME)-darwin-arm64
	
	# Windows AMD64
	zip -j $(BUILD_DIR)/release/$(BINARY_NAME)-$(VERSION)-windows-amd64.zip $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe
	
	@echo "Release archives created in $(BUILD_DIR)/release/"

# Generate sample configuration
config:
	@echo "Generating sample configuration..."
	@mkdir -p ~/.config/pve
	$(GOCMD) run ./$(CMD_DIR) config > ~/.config/pve/pve.yaml || true
	@echo "Sample configuration written to ~/.config/pve/pve.yaml"

# Docker build
docker-build:
	@echo "Building Docker image..."
	docker build -t $(BINARY_NAME):$(VERSION) .
	docker tag $(BINARY_NAME):$(VERSION) $(BINARY_NAME):latest

# Docker run
docker-run:
	@echo "Running Docker container..."
	docker run --rm -it \
		-e PVE_HOST \
		-e PVE_USER \
		-e PVE_TOKEN_NAME \
		-e PVE_TOKEN_VALUE \
		$(BINARY_NAME):latest $(ARGS)

# Show help
help:
	@echo "Available targets:"
	@echo "  all          - Clean, download deps, format, vet, and build"
	@echo "  build        - Build the binary for current platform"
	@echo "  build-all    - Build binaries for all supported platforms"
	@echo "  clean        - Remove build artifacts"
	@echo "  test         - Run tests"
	@echo "  test-coverage - Run tests with coverage report"
	@echo "  deps         - Download and tidy dependencies"
	@echo "  fmt          - Format Go code"
	@echo "  vet          - Run go vet"
	@echo "  lint         - Run golangci-lint"
	@echo "  install      - Install binary to GOPATH/bin"
	@echo "  uninstall    - Remove binary from GOPATH/bin"
	@echo "  run          - Run the application (use ARGS=... for arguments)"
	@echo "  dev          - Run in development mode with verbose logging"
	@echo "  release      - Create release archives for all platforms"
	@echo "  config       - Generate sample configuration file"
	@echo "  docker-build - Build Docker image"
	@echo "  docker-run   - Run Docker container"
	@echo "  help         - Show this help message"
	@echo ""
	@echo "Examples:"
	@echo "  make build"
	@echo "  make run ARGS='--help'"
	@echo "  make dev ARGS='create --vmid 100 --prefix backup'"
	@echo "  make install"