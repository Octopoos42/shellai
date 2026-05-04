.PHONY: build run test test-integration lint swagger sqlc precommit-setup

SERVER_DIR := server

build:
	cd $(SERVER_DIR) && go build -o bin/shellai .

run:
	cd $(SERVER_DIR) && go run .

test:
	cd $(SERVER_DIR) && go test ./...

test-integration:
	cd $(SERVER_DIR) && go test -tags integration -timeout 300s ./tests/integration/...

# Non-LLM integration tests only — no DEEPSEEK_API_KEY required
test-integration-fast:
	cd $(SERVER_DIR) && go test -tags integration -timeout 120s ./tests/integration/... -run 'TestIntegration_Admin|TestIntegration_APIKey|TestIntegration_Shell|TestIntegration_Skills|TestIntegration_SessionRename'

test-all: test test-integration


lint:
	cd $(SERVER_DIR) && golangci-lint run ./...

format:
	cd $(SERVER_DIR) && go fmt ./...

sqlc:
	cd $(SERVER_DIR) && sqlc generate

swagger:
	cd $(SERVER_DIR) && swag init -g main.go -o docs

generate: sqlc swagger

precommit-setup:
	bash precommit-setup.sh
