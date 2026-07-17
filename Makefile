.PHONY: lint test test-integration preflight server run gostall

GOSTALL_VERSION := v1.0.0
GOSTALL := $(shell command -v gostall 2>/dev/null || echo $$(go env GOPATH)/bin/gostall)

lint:
	golangci-lint run ./...

test:
	go test ./... -short -v -count=1

test-integration:
	go test ./... -v -count=1 -timeout 60s

# gostall: static analysis for lock-order inversions, missing unlocks,
# and lock-held-while-blocking starvation.
# #53: Fleet-wide prevention tool — catches concurrency bugs at build time.
# Uses -lockorder -missingunlock -starvation only; -waitgroup -channel -livelock
# excluded (intra-procedural false positives on defer wg.Done() in goroutines,
# signal.Notify channels, and test spin loops).
gostall:
	@[ -x "$(GOSTALL)" ] || { echo "gostall not installed: go install github.com/erfanmomeniii/gostall/cmd/gostall@$(GOSTALL_VERSION)"; exit 1; }
	@echo "==> gostall"
	GOWORK=off "$(GOSTALL)" -lockorder -missingunlock -starvation ./...

# preflight = the CI gate: gofmt + vet + build + short tests with race detector.
# Integration tests (requiring a live Chrome) are skipped under -short.
# #53: Race detector + gostall enabled in CI — catches concurrency bugs that vet misses.
preflight: lint gostall
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
