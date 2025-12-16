.PHONY: build run dev

# Build llm-simulator binary
build:
	@echo "🔨 Building llm-simulator..."
	@go build -o bin/llm-simulator ./cmd/llm-simulator

# Run llm-simulator binary
run:
	@echo "🧪 Running llm-simulator..."
	@./bin/llm-simulator

# Build and run (dev)
dev: build run