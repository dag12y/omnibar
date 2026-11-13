# Makefile for Omnibar backend and UI tests.

GO ?= go
QMLTESTRUNNER ?= /usr/lib/qt6/bin/qmltestrunner
BIN_DIR := build
BINARY := $(BIN_DIR)/omnibar

.PHONY: all build test ui-test e2e-ui e2e-awake e2e-audio e2e-clipboard e2e-shelf e2e-windows e2e-apps package run clean help

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

e2e-clipboard: build
	python3 scripts/e2e-clipboard.py

e2e-shelf: build
	python3 scripts/e2e-clipboard.py --shelf

e2e-apps: build
	python3 scripts/e2e-clipboard.py --apps

e2e-windows: build
	python3 scripts/e2e-clipboard.py --windows

e2e-audio: build
	python3 scripts/e2e-audio.py

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

e2e-audio: ## Verify mixer with isolated PipeWire services (needs Hyprland and WirePlumber)

e2e-clipboard: ## Verify clipboard in a private nested Wayland compositor

e2e-shelf: ## Verify File shelf and clipboard in a private nested compositor

e2e-windows: ## Verify window switching in a private nested Hyprland session

e2e-apps: ## Verify app launching with a private desktop entry and nested Hyprland
