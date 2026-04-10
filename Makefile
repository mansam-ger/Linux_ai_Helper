.PHONY: all build clean test

# Output binary name
APP_NAME=eugen
# Output directory
BIN_DIR=bin

all: clean build

build:
	@echo "Building $(APP_NAME)..."
	@mkdir -p $(BIN_DIR)
	@CGO_ENABLED=0 go build -a -installsuffix cgo -ldflags="-w -s" -o $(BIN_DIR)/$(APP_NAME) ./cmd/$(APP_NAME)
	@echo "Build complete. Binary is at $(BIN_DIR)/$(APP_NAME)"

clean:
	@echo "Cleaning up..."
	@rm -rf $(BIN_DIR)
	@echo "Clean complete."

test:
	@echo "Running tests..."
	@go test ./...
