# ===========================================
# Makefile for LoraFog IoT Project
# Author: Nguyễn Đức Nam
# Description: Build, run, test, and manage LoraFog components
# ===========================================

APP_NAME := lora
MAIN := cmd/lora/main.go
BUILD_DIR := build
BIN := $(BUILD_DIR)/$(APP_NAME)
CONFIG := configs/config.yml
TEST_CONFIG := configs/config_test.yml
DEMO_CONFIG := configs/config_demo.yml
VEHICLE_CONFIG := configs/config_vehicle.yml
GATEWAY_CONFIG := configs/config_gateway.yml

# Go parameters
GO := go
GOFLAGS :=
LDFLAGS := -s -w

# Default target
.PHONY: all
all: tidy build

# =====================
# 🚀 Build & Run
# =====================

# Build the main binary
.PHONY: build
build:
	@echo "🔧 Building $(APP_NAME)..."
	@mkdir -p $(BUILD_DIR)
	@$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BIN) $(MAIN)
	@echo "✅ Built $(BIN)"

# Run directly using Go (for development)
.PHONY: run
run:
	@echo "🚀 Running LoraFog with config: $(CONFIG)..."
	@$(GO) run $(MAIN) -c $(CONFIG)

.PHONY: test
test:
	@echo "🚀 Running LoraFog with config: $(TEST_CONFIG)..."
	@$(GO) run $(MAIN) -c $(TEST_CONFIG)

.PHONY: demo
demo:
	@echo "🚀 Running LoraFog with config: $(DEMO_CONFIG)..."
	@$(GO) run $(MAIN) -c $(DEMO_CONFIG)

.PHONY: vehicle
vehicle:
	@echo "🚀 Running LoraFog with config: $(VEHICLE_CONFIG)..."
	@$(GO) run $(MAIN) -c $(VEHICLE_CONFIG)

.PHONY: gateway
gateway:
	@echo "🚀 Running LoraFog with config: $(GATEWAY_CONFIG)..."
	@$(GO) run $(MAIN) -c $(GATEWAY_CONFIG)

# Run with verbose output
.PHONY: runv
runv:
	@echo "🚀 Running LoraFog (verbose) with config: $(CONFIG)..."
	@$(GO) run -v $(MAIN) -c $(CONFIG)

# Run using built binary
.PHONY: exec
exec: build
	@echo "🚀 Executing $(BIN) with config: $(CONFIG)..."
	@$(BIN) -c $(CONFIG)

# =====================
# 🧹 Maintenance
# =====================

# Format code and tidy dependencies
.PHONY: tidy
tidy:
	@echo "🧽 Cleaning and formatting..."
	@$(GO) fmt ./...
	@$(GO) mod tidy
	@echo "✅ Code formatted and dependencies tidied"

# Run Go vet + lint
.PHONY: lint
lint:
	@echo "🔍 Running Go vet..."
	@$(GO) vet ./...
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run || echo "⚠️ golangci-lint not installed"

# Clean build files
.PHONY: clean
clean:
	@echo "🧹 Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@find . -name "*.out" -delete
	@echo "✅ Clean done."

# =====================
# 🧪 Testing
# =====================

.PHONY: testing
testing:
	@echo "🧪 Running unit tests..."
	@$(GO) test -v ./...

.PHONY: cover
cover:
	@echo "📊 Generating test coverage..."
	@$(GO) test -coverprofile=coverage.out ./...
	@$(GO) tool cover -html=coverage.out

# =====================
# ⚙️ Virtual serial setup (optional)
# =====================

.PHONY: virt
virt:
	@echo "🔧 Creating virtual serial pairs with socat..."
	@socat -d -d \
		pty,raw,echo=0,link=/tmp/ttyGW1 \
		pty,raw,echo=0,link=/tmp/ttyLR1 & \
	socat -d -d \
		pty,raw,echo=0,link=/tmp/ttyGW2 \
		pty,raw,echo=0,link=/tmp/ttyLR2 & \
	socat -d -d \
		pty,raw,echo=0,link=/tmp/ttyGPSS1 \
		pty,raw,echo=0,link=/tmp/ttyGPSR1 & \
	socat -d -d \
		pty,raw,echo=0,link=/tmp/ttyGPSS2 \
		pty,raw,echo=0,link=/tmp/ttyGPSR2 & \
	wait
	@echo "✅ Virtual serial pairs created."

.PHONY: virt-clean
virt-clean:
	@echo "🧹 Removing virtual serial pairs..."
	@pkill -f socat || true
	@rm -f /tmp/ttyGW* /tmp/ttyLR* /tmp/ttyGPS*
	@echo "✅ Virtual serials cleaned."

# =====================
# 📦 Release build (optional)
# =====================

.PHONY: release
release:
	@echo "📦 Building release binary..."
	@GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME)_linux_amd64 $(MAIN)
	@GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME)_arm64 $(MAIN)
	@echo "✅ Release binaries ready in $(BUILD_DIR)"

