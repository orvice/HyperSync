all: buf build
ent:
	ent generate ./pkg/ent/schema
buf:
	rm -rf pkg/proto
	cd proto/ && buf generate
	cd proto/ && buf format -w
	cd proto/ && buf lint
wire:
	wire ./internal/wire
lint:
	golangci-lint run

build:
	go build -o bin/hyper-sync ./cmd/main.go


upgrade-dep:
	go get butterfly.orx.me/core@main