SHELL := /bin/bash

BIN_DIR  := bin
BIN_FILE := message-delivery
CLIENT_FILE := message-delivery-send
PKG      := github.com/darkrain/message-delivery

VERSION  := $(or $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//'),dev)
BUILD    := $(or $(shell git rev-parse --short HEAD 2>/dev/null),unknown)

LDFLAGS  := -ldflags "-X main.Version=$(VERSION) -X main.Build=$(BUILD) -X main.ProjectName=$(BIN_FILE)"

.PHONY: get vendor build test run install uninstall clean deb docker-build docker-test integration-test

get:
	go mod download

vendor:
	go mod vendor

build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BIN_DIR)/$(BIN_FILE) ./cmd/main.go
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BIN_DIR)/$(BIN_FILE)-arm64 ./cmd/main.go
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BIN_DIR)/$(CLIENT_FILE) ./cmd/send-test-message
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BIN_DIR)/$(CLIENT_FILE)-arm64 ./cmd/send-test-message

test:
	go test ./...

run:
	go run ./cmd/main.go --config message-delivery.example.json

install: build
	install -D -m 0755 $(BIN_DIR)/$(BIN_FILE) /usr/bin/$(BIN_FILE)
	install -D -m 0755 $(BIN_DIR)/$(CLIENT_FILE) /usr/bin/$(CLIENT_FILE)
	install -D -m 0644 message-delivery.service /etc/systemd/system/message-delivery.service
	install -d /etc/message-delivery
	@if [ ! -f /etc/message-delivery/config.json ]; then \
		install -D -m 0600 message-delivery.example.json /etc/message-delivery/config.json; \
		echo "Installed default config to /etc/message-delivery/config.json — please edit it!"; \
	fi
	systemctl daemon-reload

uninstall:
	systemctl stop message-delivery 2>/dev/null || true
	systemctl disable message-delivery 2>/dev/null || true
	rm -f /usr/bin/$(BIN_FILE)
	rm -f /usr/bin/$(CLIENT_FILE)
	rm -f /etc/systemd/system/message-delivery.service
	systemctl daemon-reload

clean:
	rm -rf $(BIN_DIR)

deb: build
	$(eval DEB_AMD64 := $(BIN_FILE)_$(VERSION)_amd64)
	mkdir -p /tmp/$(DEB_AMD64)/DEBIAN
	chmod 0755 /tmp/$(DEB_AMD64)/DEBIAN
	mkdir -p /tmp/$(DEB_AMD64)/usr/bin
	mkdir -p /tmp/$(DEB_AMD64)/etc/systemd/system
	mkdir -p /tmp/$(DEB_AMD64)/etc/message-delivery
	install -m 0755 $(BIN_DIR)/$(BIN_FILE) /tmp/$(DEB_AMD64)/usr/bin/$(BIN_FILE)
	install -m 0755 $(BIN_DIR)/$(CLIENT_FILE) /tmp/$(DEB_AMD64)/usr/bin/$(CLIENT_FILE)
	install -m 0644 message-delivery.service /tmp/$(DEB_AMD64)/etc/systemd/system/message-delivery.service
	install -m 0600 message-delivery.example.json /tmp/$(DEB_AMD64)/etc/message-delivery/config.json
	printf 'Package: $(BIN_FILE)\nVersion: $(VERSION)\nArchitecture: amd64\nMaintainer: darkrain\nDescription: message-delivery\n' \
		> /tmp/$(DEB_AMD64)/DEBIAN/control
	dpkg-deb --build /tmp/$(DEB_AMD64) $(BIN_DIR)/$(DEB_AMD64).deb
	rm -rf /tmp/$(DEB_AMD64)
	@echo "Built: $(BIN_DIR)/$(DEB_AMD64).deb"
	$(eval DEB_ARM64 := $(BIN_FILE)_$(VERSION)_arm64)
	mkdir -p /tmp/$(DEB_ARM64)/DEBIAN
	chmod 0755 /tmp/$(DEB_ARM64)/DEBIAN
	mkdir -p /tmp/$(DEB_ARM64)/usr/bin
	mkdir -p /tmp/$(DEB_ARM64)/etc/systemd/system
	mkdir -p /tmp/$(DEB_ARM64)/etc/message-delivery
	install -m 0755 $(BIN_DIR)/$(BIN_FILE)-arm64 /tmp/$(DEB_ARM64)/usr/bin/$(BIN_FILE)
	install -m 0755 $(BIN_DIR)/$(CLIENT_FILE)-arm64 /tmp/$(DEB_ARM64)/usr/bin/$(CLIENT_FILE)
	install -m 0644 message-delivery.service /tmp/$(DEB_ARM64)/etc/systemd/system/message-delivery.service
	install -m 0600 message-delivery.example.json /tmp/$(DEB_ARM64)/etc/message-delivery/config.json
	printf 'Package: $(BIN_FILE)\nVersion: $(VERSION)\nArchitecture: arm64\nMaintainer: darkrain\nDescription: message-delivery\n' \
		> /tmp/$(DEB_ARM64)/DEBIAN/control
	dpkg-deb --build /tmp/$(DEB_ARM64) $(BIN_DIR)/$(DEB_ARM64).deb
	rm -rf /tmp/$(DEB_ARM64)
	@echo "Built: $(BIN_DIR)/$(DEB_ARM64).deb"

docker-build:
	docker build -t message-delivery:local .

docker-test:
	docker compose -f docker-compose.integration.yml up --build --abort-on-container-exit --exit-code-from tests

integration-test: docker-test
