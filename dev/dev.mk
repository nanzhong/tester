PG_HOST ?= 127.0.0.1
PG_PORT ?= 5432
PG_ADDR ?= $(PG_HOST):$(PG_PORT)
PG_USER ?= tester
PG_PASS ?= password
PG_NAME ?= tester
PG_DSN ?= postgres://$(PG_USER):$(PG_PASS)@$(PG_ADDR)/$(PG_NAME)?sslmode=disable

.PHONY: dev/up
dev/up:
	docker run --rm -d \
		--name tester-pg \
		-e POSTGRES_USER=$(PG_USER) \
		-e POSTGRES_PASSWORD=$(PG_PASS) \
		-e POSTGERS_DB=$(PG_NAME) \
		-p $(PG_ADDR):5432 \
		postgres:12

.PHONY: dev/down
dev/down:
	docker rm -f tester-pg

.PHONY: dev/pg_dsn
dev/pg_dsn:
	@echo $(PG_DSN)

.PHONY: dev/psql
dev/psql:
	docker run -it --rm --network=host postgres:12 psql $(PG_DSN)

.PHONY: dev/test
dev/test:
	PG_DSN=$(PG_DSN) go test ./...
