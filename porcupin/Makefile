BINARY_NAME=porcupin
BuildDir=build/bin
ManDir=docs/man
INSTALL_BIN_DIR=/usr/local/bin
INSTALL_MAN_DIR=/usr/local/share/man/man1

.PHONY: all build build-all build-linux-amd64 build-linux-arm64 install uninstall clean help

all: build

# Detect OS to select correct mandb update method
UNAME_S := $(shell uname -s)

# Version injection
VERSION ?= $(shell git describe --tags --always --dirty || echo "dev")
LDFLAGS := -ldflags "-X porcupin/backend/version.Version=$(VERSION)"

build:
	@echo "Building $(BINARY_NAME) (current platform)..."
	@mkdir -p $(BuildDir)
	@go build $(LDFLAGS) -o $(BuildDir)/$(BINARY_NAME) ./cmd/headless

build-all: build build-linux-amd64 build-linux-arm64

build-linux-amd64:
	@echo "Building $(BINARY_NAME)-linux-amd64..."
	@mkdir -p $(BuildDir)
	@GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BuildDir)/$(BINARY_NAME)-linux-amd64 ./cmd/headless

build-linux-arm64:
	@echo "Building $(BINARY_NAME)-linux-arm64 (Raspberry Pi)..."
	@mkdir -p $(BuildDir)
	@GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BuildDir)/$(BINARY_NAME)-linux-arm64 ./cmd/headless

install: build
	@echo "Installing $(BINARY_NAME) to $(INSTALL_BIN_DIR)..."
	@install -m 755 $(BuildDir)/$(BINARY_NAME) $(INSTALL_BIN_DIR)/$(BINARY_NAME)
	@echo "Installing man page..."
	@mkdir -p $(INSTALL_MAN_DIR)
	@install -m 644 $(ManDir)/$(BINARY_NAME).1 $(INSTALL_MAN_DIR)/$(BINARY_NAME).1
	@echo "Updating man database..."
	@if [ "$(UNAME_S)" = "Darwin" ]; then \
		/usr/libexec/makewhatis /usr/local/share/man 2>/dev/null || true; \
	else \
		mandb >/dev/null 2>&1 || true; \
	fi
	@echo "Installation complete."
	@echo "Run '$(BINARY_NAME) --help' or 'man $(BINARY_NAME)'"

uninstall:
	@echo "Removing $(BINARY_NAME)..."
	@rm -f $(INSTALL_BIN_DIR)/$(BINARY_NAME)
	@rm -f $(INSTALL_MAN_DIR)/$(BINARY_NAME).1
	@echo "Uninstall complete."

clean:
	@echo "Cleaning build directory..."
	@rm -f $(BuildDir)/$(BINARY_NAME)
	@rm -f $(BuildDir)/$(BINARY_NAME)-linux-amd64
	@rm -f $(BuildDir)/$(BINARY_NAME)-linux-arm64

help:
	@echo "Available targets:"
	@echo "  build              - Build for current platform (default)"
	@echo "  build-all          - Build for current, Linux AMD64, and Linux ARM64"
	@echo "  build-linux-amd64  - Build for Linux AMD64"
	@echo "  build-linux-arm64  - Build for Raspberry Pi (ARM64)"
	@echo "  install            - Install binary and man page (requires sudo)"
	@echo "  uninstall          - Remove installed binary and man page"
	@echo "  clean              - Remove build artifacts"
