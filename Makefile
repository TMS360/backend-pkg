.PHONY: gen-proto

## gen-proto: Regenerates Go code for all .proto files in the repository
gen-proto:
	protoc --proto_path=proto \
		--go_out=proto --go_opt=paths=source_relative \
		--go-grpc_out=proto --go-grpc_opt=paths=source_relative \
		proto/**/*.proto