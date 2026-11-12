# Makefile for Omnibar backend and UI tests.

GO ?= go
QMLTESTRUNNER ?= /usr/lib/qt6/bin/qmltestrunner
BIN_DIR := build
BINARY := $(BIN_DIR)/omnibar

.PHONY: all build test ui-test e2e-ui e2e-awake package run clean help

all: build

build:
	@mkdir -p $(BIN_DIR)
	$(GO) build -o $(BINARY) ./cmd/omnibar

test:
	$(GO) test ./...

ui-test:
	QT_QPA_PLATFORM=offscreen QT_QPA_PLATFORMTHEME= QT_QUICK_CONTROLS_STYLE=Basic $(QMLTESTRUNNER) -input ui/qml/tests -o -,txt

e2e-ui: build
	OMNIBAR_BIN=$(abspath $(BINARY)) ./scripts/e2e-ui.sh

e2e-awake: build
	python3 scripts/e2e-keep-awake.py

package:
	./scripts/package-local.sh

run: build
	$(BINARY)

clean:
	@rm -rf $(BIN_DIR) dist

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

# Target-specific documentation.
build: ## Build the omnibar binary into build/omnibar
test: ## Run all Go unit and integration tests
ui-test: ## Run the QML logic tests with Qt Test
e2e-ui: ## Run the real Quickshell UI against the real backend (needs Wayland)
package: ## Build the Arch package into dist/ from the working tree (does not install)
run: ## Build and run the omnibar backend
clean: ## Remove generated build artifacts

e2e-awake: ## Verify real logind and Wayland Keep Awake (needs Hyprland; briefly inhibits idle/sleep)
