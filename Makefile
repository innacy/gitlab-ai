APP_NAME := gitlab-ai
VERSION := 0.1.0
BUILD_DIR := bin
GO_FILES := $(shell find . -name '*.go' -type f)

LDFLAGS :=

.PHONY: all build clean test lint run install

all: build

build:
	@echo "Building $(APP_NAME)..."
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME) .

run: build
	@./$(BUILD_DIR)/$(APP_NAME)

install: build
	@echo "Installing $(APP_NAME)..."
	go install $(LDFLAGS) .

clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)

test:
	@echo "Running tests..."
	go test -v -race -coverprofile=coverage.out ./...

coverage: test
	go tool cover -html=coverage.out -o coverage.html

lint:
	@echo "Running linter..."
	golangci-lint run ./...

fmt:
	@echo "Formatting..."
	gofmt -s -w .

vet:
	@echo "Vetting..."
	go vet ./...

tidy:
	@echo "Tidying modules..."
	go mod tidy

# Cross-compilation targets
build-linux:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME)-linux-amd64 .

build-darwin:
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME)-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME)-darwin-arm64 .

build-windows:
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME)-windows-amd64.exe .

build-all: build-linux build-darwin build-windows
