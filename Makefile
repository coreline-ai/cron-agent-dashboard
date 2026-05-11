.PHONY: test build web-build check run tidy

test:
	go test ./...

build:
	go build ./...

web-build:
	pnpm --filter web build

check: test build web-build

run:
	go run ./cmd/corn-agent-dashboard serve

tidy:
	go mod tidy
