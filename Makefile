IMAGE_NAME ?= nanzhong/tester
COMMIT ?= $(shell git rev-parse HEAD)

.PHONY: all
all: tester

.PHONY: clean
clean:
	rm -rf dist
	packr2 clean

.PHONY: tester
tester:
ifdef ASSETS
	packr2
endif
	mkdir -p dist
	go build  -o ./dist ./cmd/tester/...

.PHONY: install
install:
	go install ./cmd/tester/...

tester-container:
	packr2
	GOOS=linux GOARCH=amd64 go build -o ./dist/tester-linux-amd64 ./cmd/tester/...
	docker build -t $(IMAGE_NAME):$(COMMIT) .
ifdef PUSH
	docker push $(IMAGE_NAME):$(COMMIT)
endif
