.PHONY: build test proto

build:
	go build -o kd ./cmd/kd

test:
	go test ./...

proto:
	./scripts/gen-proto.sh
