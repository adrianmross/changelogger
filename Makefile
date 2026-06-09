SHELL := /bin/bash
GOFILES := $(shell find . -name '*.go' -not -path './vendor/*')

.PHONY: fmt vet test build ci

fmt:
	gofmt -w $(GOFILES)

vet:
	go vet ./...

test:
	go test ./...

build:
	go build -o bin/changelogger ./cmd/changelogger

ci: fmt vet test build
