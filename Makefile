GO ?= go
GOBIN ?= $(shell $(GO) env GOBIN)
GOPATH ?= $(shell $(GO) env GOPATH)

ifeq ($(strip $(GOBIN)),)
BIN_DIR := $(GOPATH)/bin
else
BIN_DIR := $(GOBIN)
endif

PP_BIN ?= $(BIN_DIR)/pp

.PHONY: install test test-storage build clean

install:
	@mkdir -p "$(dir $(PP_BIN))"
	$(GO) build -o "$(PP_BIN)" ./cmd/deploy
	@echo "installed $(PP_BIN)"

build: install

test:
	$(GO) test ./...

test-storage:
	node --test infra/ansible/tests/containerd-storage.test.ts

clean:
	rm -f "$(PP_BIN)"
