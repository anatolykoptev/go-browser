.PHONY: lint test test-integration server run

lint:
	golangci-lint run ./...

test:
	go test ./... -short -v -count=1

test-integration:
	go test ./... -v -count=1 -timeout 60s

server:
	go build -o bin/go-browser-server ./cmd/server

run: server
	./bin/go-browser-server
