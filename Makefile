.PHONY: proto build test

proto:
	protoc --go_out=gen/dns --go-grpc_out=gen/dns \
		--go_opt=paths=source_relative \
		--go-grpc_opt=paths=source_relative \
		proto/dns/dns.proto

build:
	go build ./...

test:
	go test ./...

cover:
	go test -cover ./...