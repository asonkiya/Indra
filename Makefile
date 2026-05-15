.PHONY: build test proto lint run clean docker-up docker-down setup mobile-android mobile-ios mobile-setup relay

BIN := bin/indra
GO  := $(shell mise which go 2>/dev/null || which go)

build:
	$(GO) build -o $(BIN) ./cmd/indra

test:
	$(GO) test -race -timeout 120s ./...

proto:
	protoc --go_out=. --go_opt=paths=source_relative \
		internal/protocol/pb/messages.proto

lint:
	golangci-lint run ./...

run: build
	./$(BIN) --debug

clean:
	rm -rf bin/

# Download dependencies without a network proxy.
deps:
	$(GO) mod tidy
	$(GO) mod download

# First-time setup: install mise toolchain + deps.
setup:
	mise install
	$(GO) mod tidy
	$(GO) mod download

# Derive a clean Go environment from the mise-resolved binary.
# This avoids inheriting stale GOROOT/GOBIN/PATH from the parent shell.
GO_DIR     := $(dir $(GO))
GO_ROOT    := $(shell GOROOT= $(GO) env GOROOT 2>/dev/null)
GOPATH_BIN := $(shell GOROOT= GOBIN= $(GO) env GOPATH 2>/dev/null)/bin
GOMOBILE   := $(GOPATH_BIN)/gomobile
MOBILE_ENV := PATH=$(GOPATH_BIN):$(GO_DIR):$(PATH) GOROOT=$(GO_ROOT) GOBIN=

mobile-android:
	mkdir -p mobile/build
	$(MOBILE_ENV) $(GOMOBILE) bind -target android -o mobile/build/indra.aar ./mobile/

mobile-ios:
	mkdir -p mobile/build
	$(MOBILE_ENV) $(GOMOBILE) bind -target ios -o mobile/build/Indra.xcframework ./mobile/

mobile-setup:
	$(MOBILE_ENV) $(GO) install golang.org/x/mobile/cmd/gomobile@latest
	$(MOBILE_ENV) $(GO) install golang.org/x/mobile/cmd/gobind@latest
	$(MOBILE_ENV) $(GOMOBILE) init

relay:
	cd relay && $(GO) build -o ../bin/relay .

docker-up:
	docker compose up --build -d

docker-down:
	docker compose down
