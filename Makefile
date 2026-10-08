.PHONY: test lint fmt

test:
	go test ./...

fmt:
	gofmt -l -w .

lint:
	go vet ./...
