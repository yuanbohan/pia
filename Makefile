.PHONY: check fmt lint race test vet

check: fmt vet test lint

fmt:
	go fmt ./...

vet:
	go vet ./...

test:
	go test ./...

lint:
	golangci-lint run ./...

race:
	go test -race ./...
