# WhatsApp MCP Tool - Makefile
# Simple build system for the WhatsApp MCP tool

# Variables
BINARY_NAME=whatsapp_mcp
BINARY_WINDOWS=$(BINARY_NAME).exe
BINARY_LINUX=$(BINARY_NAME)
BINARY_MAC=$(BINARY_NAME)

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Build flags for smaller binaries
LDFLAGS=-ldflags="-s -w"

# Default target - build for current platform
.PHONY: all
all: build

# Build for current platform
.PHONY: build
build:
	@echo "Building $(BINARY_NAME) for current platform..."
ifeq ($(OS),Windows_NT)
	cd cmd/whatsapp_mcp && $(GOBUILD) -a $(LDFLAGS) -o ../../$(BINARY_WINDOWS)
else
	cd cmd/whatsapp_mcp && $(GOBUILD) -a $(LDFLAGS) -o ../../$(BINARY_NAME)
endif

# Build for Windows
.PHONY: windows
windows:
	@echo "Building for Windows..."
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_WINDOWS) ./cmd/whatsapp_mcp

# Build for Linux
.PHONY: linux
linux:
	@echo "Building for Linux..."
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_LINUX) ./cmd/whatsapp_mcp

# Build for macOS
.PHONY: mac
mac:
	@echo "Building for macOS..."
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_MAC) ./cmd/whatsapp_mcp

# Build for all platforms
.PHONY: all-platforms
all-platforms: windows linux mac
	@echo "Built for all platforms"

# Clean build artifacts
.PHONY: clean
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_WINDOWS)
	rm -f $(BINARY_LINUX)
	rm -f $(BINARY_MAC)

# Run tests
.PHONY: test
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

# Download dependencies
.PHONY: deps
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

# Initialize Go module (first time setup)
.PHONY: init
init:
	@echo "Initializing Go module..."
	$(GOMOD) init github.com/yourusername/whatsapp-mcp
	@echo "Adding dependencies..."
	$(GOGET) go.mau.fi/whatsmeow@latest
	$(GOMOD) tidy

# Run the tool (for development)
.PHONY: run
run:
	@echo "Running $(BINARY_NAME)..."
	$(GOCMD) run ./cmd/whatsapp_mcp

# Install to GOPATH/bin
.PHONY: install
install:
	@echo "Installing $(BINARY_NAME)..."
	$(GOCMD) install $(LDFLAGS) ./cmd/whatsapp_mcp

# Show help
.PHONY: help
help:
	@echo "WhatsApp MCP Tool - Makefile targets:"
	@echo ""
	@echo "  make              - Build for current platform (default)"
	@echo "  make build        - Build for current platform"
	@echo "  make windows      - Build for Windows"
	@echo "  make linux        - Build for Linux"
	@echo "  make mac          - Build for macOS"
	@echo "  make all-platforms- Build for all platforms"
	@echo "  make clean        - Remove build artifacts"
	@echo "  make test         - Run tests"
	@echo "  make deps         - Download and tidy dependencies"
	@echo "  make init         - Initialize new Go module (first time)"
	@echo "  make run          - Run without building"
	@echo "  make install      - Install to GOPATH/bin"
	@echo "  make help         - Show this help"
	@echo ""
	@echo "Quick start:"
	@echo "  1. make init      - First time setup"
	@echo "  2. make           - Build the tool"
	@echo "  3. ./$(BINARY_NAME) - Run the tool"


