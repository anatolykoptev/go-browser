.PHONY: lint test test-integration

lint:
	golangci-lint run ./...

test:
	go test ./... -short -v -count=1

test-integration:
	go test ./... -v -count=1 -timeout 60s
