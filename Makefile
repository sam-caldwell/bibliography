SHELL := /bin/bash

.PHONY: build fmt test cover clean

build: clean
	go fmt ./src/...
	go build -o bin/bib ./src/cmd/bib

fmt:
	go fmt ./src/...

test:
	go test ./src/... -coverprofile=coverage.out -covermode=atomic

cover: test
	go tool cover -func=coverage.out | tail -n 1

clean:
	rm -rf bin
	rm -f coverage.out
	rm -f data/metadata/*.json
