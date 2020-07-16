SHELL := /bin/bash

commit ?= $(shell git rev-parse --short HEAD)
image := nanzhong/tester

include ./dev/dev.mk

.PHONY: clean
clean:
	rm -rf dist
	rm -rf ./cmd/tester/pkged.go

.PHONY: deps
deps:
	go get github.com/markbates/pkger/cmd/pkger

.PHONY: build
build: deps
	pkger -o ./cmd/tester
	GOOS=linux GOARCH=amd64 go build -o ./dist/tester-linux-amd64 ./cmd/tester/...

.PHONY: build-image
build-image:
	docker build -t $(image):$(commit) .
ifdef LATEST
	docker tag $(image):$(commit) $(image):latest
endif
ifdef PUSH
	docker push $(image):$(commit)
	docker push $(image):latest
endif

.PHONY: install
install: deps
	pkger -o ./cmd/tester
	go install ./cmd/tester/...
