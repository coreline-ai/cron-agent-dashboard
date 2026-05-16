BINARY ?= corn-agent-dashboard
PLATFORMS ?= darwin/arm64 darwin/amd64 linux/amd64 linux/arm64
VERSION ?= 0.1.0
LDFLAGS ?= -s -w -X github.com/coreline-ai/corn-agent-dashboard/internal/httpapi.Version=$(VERSION)

.PHONY: install test build web-build prepare-static release-build e2e-smoke e2e-full screenshots verify-clean-clone check run tidy clean

install:
	pnpm install --frozen-lockfile --ignore-scripts

test:
	go test ./...

build: web-build prepare-static
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/corn-agent-dashboard

web-build: install
	pnpm --filter web build

prepare-static: web-build
	rm -rf internal/httpapi/web_dist
	mkdir -p internal/httpapi/web_dist
	cp -R web/dist/. internal/httpapi/web_dist/

release-build: web-build prepare-static
	mkdir -p dist
	for target in $(PLATFORMS); do \
		GOOS=$${target%/*} GOARCH=$${target#*/} go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-$${target%/*}-$${target#*/} ./cmd/corn-agent-dashboard; \
	done

e2e-smoke: build
	pnpm exec playwright test --config playwright.config.ts tests/e2e/smoke.spec.ts

e2e-full: build
	pnpm exec playwright test --config playwright.config.ts

screenshots: build
	GENERATE_SCREENSHOTS=1 pnpm exec playwright test --config playwright.config.ts tests/e2e/screenshots.spec.ts

verify-clean-clone:
	./scripts/verify-clean-clone.sh

check: test web-build prepare-static
	go build ./...

run:
	go run ./cmd/corn-agent-dashboard serve

tidy:
	go mod tidy

clean:
	rm -rf dist web/dist web/tsconfig.tsbuildinfo .tmp test-results playwright-report $(BINARY)
