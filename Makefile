.PHONY: build migrate-up migrate-down test test-integration generate clean

build:
	go build -o dist/taskmgr ./cmd/server/
	go build -o dist/taskmgr-migrate ./cmd/migrate/

migrate-up:
	./dist/taskmgr-migrate up

migrate-down:
	./dist/taskmgr-migrate down

test:
	go test ./... -timeout 60s

test-integration:
	go test -tags integration ./... -timeout 300s -v

generate:
	cd api && oapi-codegen --generate types,gin --package api -o gen/server.gen.go openapi.yaml

publish:
	mkdir -p contracts
	cp api/openapi.yaml contracts/openapi.yaml

clean:
	rm -rf dist/
