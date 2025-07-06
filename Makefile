BINARY_NAME=echospider
BUILD_DIR=build
MAIN_FILE=main.go

.PHONY: build run clean test deps

build:
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) .

run: build
	@echo "Usage: make run URL=https://example.com"
	@if [ "$(URL)" = "" ]; then \
		echo "Please provide a URL: make run URL=https://example.com"; \
		exit 1; \
	fi
	$(BUILD_DIR)/$(BINARY_NAME) $(URL)

clean:
	rm -rf $(BUILD_DIR)
	go clean

test:
	go test -v ./...

deps:
	go mod download
	go mod tidy

install: build
	go install .

.DEFAULT_GOAL := build
