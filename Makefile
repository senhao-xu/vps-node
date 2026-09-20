BINARY_PANEL ?= panel
BINARY_AGENT ?= panel-agent
GO ?= go
VERSION ?= $(shell date -u +%Y%m%d)
DOWNLOAD_BASE ?= https://example.com/downloads/vps-node

.PHONY: all build agent panel ui build-embed test test-race vet test-integration release-agent clean

all: build

build: vet test agent panel

agent:
	$(GO) build -trimpath -ldflags "-s -w" -o $(BINARY_AGENT) ./cmd/agent

panel:
	$(GO) build -trimpath -ldflags "-s -w" -o $(BINARY_PANEL) ./cmd/panel

ui:
	cd web && npm ci && npm run build

build-embed: ui
	rm -rf internal/webui/dist
	cp -r web/dist internal/webui/dist
	$(GO) build -trimpath -tags embed_ui -ldflags "-s -w" -o $(BINARY_PANEL) ./cmd/panel

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

test-integration:
	$(GO) test ./... -tags integration

release-agent:
	for arch in amd64 arm64 386; do \
		mkdir -p dist/panel-agent-linux-$$arch; \
		$(GO) build -trimpath -ldflags "-s -w" \
			-o dist/panel-agent-linux-$$arch/panel-agent ./cmd/agent; \
		cp deploy/panel-agent.service deploy/install-agent.sh dist/panel-agent-linux-$$arch/; \
		tar -czf dist/panel-agent-$(VERSION)-linux-$$arch.tar.gz -C dist/panel-agent-linux-$$arch \
			panel-agent panel-agent.service install-agent.sh; \
	done
	@echo "release tarballs in dist/ (set DOWNLOAD_BASE/VERSION when installing)"

clean:
	rm -rf dist $(BINARY_PANEL) $(BINARY_AGENT)
