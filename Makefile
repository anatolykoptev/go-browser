.PHONY: lint test test-integration preflight server run

lint:
	golangci-lint run ./...

test:
	go test ./... -short -v -count=1

test-integration:
	go test ./... -v -count=1 -timeout 60s

# preflight = the CI gate: gofmt + vet + build + short tests with race detector.
# Integration tests (requiring a live Chrome) are skipped under -short.
# #53: Race detector enabled in CI — catches concurrency bugs that vet misses.
preflight: lint
	@echo "==> gofmt"
	@gofmt -l . | tee /dev/stderr | grep -q . && (echo "gofmt issues found" && exit 1) || true
	@echo "==> go vet"
	go vet ./...
	@echo "==> go build"
	go build ./...
	@echo "==> go test -short -race"
	go test ./... -short -race -count=1 -timeout 120s
	@echo "preflight OK"

server:
	go build -o bin/go-browser-server ./cmd/server

run: server
	./bin/go-browser-server
