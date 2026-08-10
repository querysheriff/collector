GOLANGCI := go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2
NFPM := go run github.com/goreleaser/nfpm/v2/cmd/nfpm@v2.44.0

COMPOSE := docker compose -f dev/docker-compose.yml

GIT_VERSION := $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//')
VERSION ?= $(or $(GIT_VERSION),0.0.0-dev)
DIST := dist
LDFLAGS := -s -w -X main.version=$(VERSION)
IMAGE ?= ghcr.io/querysheriff/collector

.PHONY: check
check:
	$(MAKE) fmt
	$(MAKE) tidy
	$(MAKE) lint
	$(MAKE) test

.PHONY: lint
lint:
	$(GOLANGCI) run -c .golangci.yml

.PHONY: fmt
fmt:
	$(GOLANGCI) fmt -c .golangci.yml

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: proto-update
proto-update:
	GOPRIVATE=buf.build go get buf.build/gen/go/querysheriff/backend/connectrpc/go@latest
	$(MAKE) tidy

.PHONY: test
test:
	go test ./...

.PHONY: run
run:
	go run ./cmd/collector

.PHONY: dev-postgres-up
dev-postgres-up:
	$(COMPOSE) up -d --wait

.PHONY: dev-postgres-down
dev-postgres-down:
	$(COMPOSE) down -v

.PHONY: dev
dev:
	go run ./cmd/collector -config dev/querysheriff-collector.yml

.PHONY: build-linux
build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/querysheriff-collector-linux-amd64 ./cmd/collector

.PHONY: deb
deb: build-linux
	ARCH=amd64 VERSION=$(VERSION) $(NFPM) pkg -f packaging/nfpm.yml -p deb -t $(DIST)

.PHONY: docker
docker:
	docker build --build-arg VERSION=$(VERSION) -t $(IMAGE):$(VERSION) .

# Usage: `make release VERSION=0.1.0`.
# Validates -> pushes -> fires .github/workflows/release.yml -> builds the .deb -> publishes to GitHub Release.
.PHONY: release
release:
	@echo "$(VERSION)" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$$' \
		|| { echo "error: pass a semantic version, e.g. make release VERSION=0.1.0"; exit 1; }
	@git diff --quiet && git diff --cached --quiet \
		|| { echo "error: uncommitted changes — commit them before releasing"; exit 1; }
	@if git rev-parse -q --verify "refs/tags/v$(VERSION)" >/dev/null; then \
		echo "error: tag v$(VERSION) already exists"; exit 1; fi
	$(MAKE) check
	@git diff --quiet && git diff --cached --quiet \
		|| { echo "error: 'make check' reformatted files — commit them, then re-run"; exit 1; }
	git tag -a "v$(VERSION)" -m "v$(VERSION)"
	git push origin "v$(VERSION)"
	@echo "Tagged and pushed v$(VERSION). GitHub Actions is building and publishing the release."

