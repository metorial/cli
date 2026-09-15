CLI_NAME := metorial
ADMIN_NAME := metorial-admin
BIN_DIR := bin
BIN := $(BIN_DIR)/$(CLI_NAME)
ADMIN_BIN := $(BIN_DIR)/$(ADMIN_NAME)
PKG := ./cmd/metorial
ADMIN_PKG := ./cmd/metorial-admin

.PHONY: help build run fmt test tidy check install clean completion-bash completion-zsh completion-fish completion-powershell generate release snapshot public

help:
	@printf "%s\n" \
		"Available targets:" \
		"  make build                 Build both CLIs into ./bin" \
		"  make run                   Run the consumer CLI locally" \
		"  make generate              Generate admin commands from Magnetar introspect" \
		"  make fmt                   Format all Go code" \
		"  make test                  Run CLI tests" \
		"  make tidy                  Sync go.mod and go.sum" \
		"  make check                 Run fmt, tidy, and tests" \
		"  make install               Install both CLIs into GOPATH/bin" \
		"  make clean                 Remove local build output" \
		"  make completion-bash       Print bash completions" \
		"  make completion-zsh        Print zsh completions" \
		"  make completion-fish       Print fish completions" \
		"  make completion-powershell Print PowerShell completions" \
		"  make public                Build the static installer site into ./public" \
		"  make snapshot              Build a Goreleaser snapshot" \
		"  make release               Run Goreleaser release --clean"

build:
	mkdir -p $(BIN_DIR)
	go build -o $(BIN) $(PKG)
	go build -o $(ADMIN_BIN) $(ADMIN_PKG)

run:
	go run $(PKG)

generate:
	cd generator && bun run generate $${API_URL:-http://metorial-root.localhost:4310}

fmt:
	gofmt -w $$(find . -name '*.go' -print)

test:
	go test ./...

tidy:
	go mod tidy

check: fmt tidy test

install:
	go install $(PKG)
	go install $(ADMIN_PKG)

clean:
	rm -rf $(BIN_DIR)

completion-bash:
	go run $(PKG) completion bash

completion-zsh:
	go run $(PKG) completion zsh

completion-fish:
	go run $(PKG) completion fish

completion-powershell:
	go run $(PKG) completion powershell

public:
	bun ./scripts/build-public.ts

snapshot:
	goreleaser release --clean --snapshot

release:
	goreleaser release --clean
