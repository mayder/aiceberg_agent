BINARY=aiceberg_agent
all: build
tidy: ; go mod tidy
build: ; go build -o ./bin/$(BINARY) ./cmd/agent
run: ; go run ./cmd/agent -config ./configs/config.example.yml
