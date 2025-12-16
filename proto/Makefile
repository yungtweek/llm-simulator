

# Proto / gRPC code generation for llm-simulator
#
# Requirements:
#   - protoc
#   - protoc-gen-go
#   - protoc-gen-go-grpc
#
# Install plugins (example):
#   go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
#   go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

PROTO_DIR := .
OUT_DIR   := ../gen

PROTO_FILES := $(wildcard $(PROTO_DIR)/*.proto)

.PHONY: all gen clean check

all: gen

check:
	@command -v protoc >/dev/null 2>&1 || (echo "protoc not found" && exit 1)
	@command -v protoc-gen-go >/dev/null 2>&1 || (echo "protoc-gen-go not found (go install google.golang.org/protobuf/cmd/protoc-gen-go@latest)" && exit 1)
	@command -v protoc-gen-go-grpc >/dev/null 2>&1 || (echo "protoc-gen-go-grpc not found (go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest)" && exit 1)

# Generate Go types + gRPC stubs into ../gen
# Note: go_package inside .proto controls the import path/package name.
gen: check
	@mkdir -p $(OUT_DIR)
	protoc -I $(PROTO_DIR) \
		--go_out=$(OUT_DIR) --go_opt=paths=source_relative \
		--go-grpc_out=$(OUT_DIR) --go-grpc_opt=paths=source_relative \
		$(PROTO_FILES)
	@echo "Generated Go code into $(OUT_DIR)"

clean:
	@rm -rf $(OUT_DIR)
	@echo "Removed $(OUT_DIR)"