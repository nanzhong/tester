IMAGE_NAME ?= nanzhong/tester
COMMIT ?= $(shell git rev-parse HEAD)

.PHONY: all
all: tester

.PHONY: clean
clean:
	rm -rf dist
	packr2 clean

.PHONY: install
install:
	packr2
	go install ./cmd/tester/...

.PHONY: tester
tester:
	packr2
	GOOS=linux GOARCH=amd64 go build -o ./dist/tester-linux-amd64 ./cmd/tester/...
	docker build -t $(IMAGE_NAME):$(COMMIT) .
ifdef LATEST
	docker tag $(IMAGE_NAME):$(COMMIT) $(IMAGE_NAME):latest
endif
ifdef PUSH
	docker push $(IMAGE_NAME):$(COMMIT)
ifdef LATEST
	docker push $(IMAGE_NAME):latest
endif
endif
