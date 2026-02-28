.PHONY: build test lint clean

build:
	go build -o bin/perry ./cmd/perry

test:
	go test ./... -v -race

lint:
	go vet ./...

clean:
	rm -rf bin/
