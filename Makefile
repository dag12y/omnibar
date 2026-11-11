# Makefile for Omnibar backend and UI tests.

GO ?= go
QMLTESTRUNNER ?= /usr/lib/qt6/bin/qmltestrunner
BIN_DIR := build
BINARY := $(BIN_DIR)/omnibar

.PHONY: all build test ui-test run clean help

all: build

build:
	@mkdir -p $(BIN_DIR)
	$(GO) build -o $(BINARY) ./cmd/omnibar

test:
	$(GO) test ./...

ui-test:
	QT_QPA_PLATFORM=offscreen $(QMLTESTRUNNER) -input ui/qml/tests -o -,txt

run: build
	$(BINARY)

clean:
	@rm -rf $(BIN_DIR)

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

# Target-specific documentation.
build: ## Build the omnibar binary into build/omnibar
test: ## Run all Go unit and integration tests
ui-test: ## Run the QML logic tests with Qt Test
run: ## Build and run the omnibar backend
clean: ## Remove generated build artifacts
