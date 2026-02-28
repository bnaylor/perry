.PHONY: build test test-go test-python lint clean

build:
	go build -o bin/perry ./cmd/perry

test: test-go test-python

test-go:
	go test ./... -v -race

test-python:
	python3 -m pytest python/ -v

lint:
	go vet ./...

clean:
	rm -rf bin/
