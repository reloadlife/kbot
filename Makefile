.PHONY: help build test test-verbose test-coverage clean run docker-build docker-push deploy lint fmt vet

# Variables
BINARY_NAME=kubectl-bot
DOCKER_IMAGE=ghcr.io/reloadlife/kbot
VERSION?=latest
MAIN_PATH=./cmd/bot

# Default target
help:
	@echo "Available targets:"
	@echo "  build           - Build the binary"
	@echo "  test            - Run unit tests"
	@echo "  test-verbose    - Run tests with verbose output"
	@echo "  test-coverage   - Run tests with coverage report"
	@echo "  clean           - Remove build artifacts"
	@echo "  run             - Run the bot locally (requires env vars)"
	@echo "  docker-build    - Build Docker image"
	@echo "  docker-push     - Push Docker image to registry"
	@echo "  deploy          - Deploy to Kubernetes"
	@echo "  lint            - Run golangci-lint"
	@echo "  fmt             - Format code"
	@echo "  vet             - Run go vet"
	@echo "  deps            - Download dependencies"

# Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p bin
	@go build -o bin/$(BINARY_NAME) $(MAIN_PATH)
	@echo "Binary built: bin/$(BINARY_NAME)"

# Run all tests
test:
	@echo "Running tests..."
	@go test ./...

# Run tests with verbose output
test-verbose:
	@echo "Running tests (verbose)..."
	@go test -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	@go test -cover ./...
	@go test -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf bin/
	@rm -f coverage.out coverage.html
	@echo "Clean complete"

# Run the bot locally (requires TELEGRAM_BOT_TOKEN and ADMIN_TELEGRAM_IDS env vars)
run:
	@echo "Running $(BINARY_NAME)..."
	@go run $(MAIN_PATH)

# Build Docker image
docker-build:
	@echo "Building Docker image..."
	@docker build -t $(DOCKER_IMAGE):$(VERSION) .
	@echo "Docker image built: $(DOCKER_IMAGE):$(VERSION)"

# Push Docker image to registry
docker-push:
	@echo "Pushing Docker image..."
	@docker push $(DOCKER_IMAGE):$(VERSION)
	@echo "Docker image pushed: $(DOCKER_IMAGE):$(VERSION)"

# Deploy to Kubernetes
deploy:
	@echo "Deploying to Kubernetes..."
	@kubectl apply -f manifests/crd.yaml
	@kubectl apply -f manifests/rbac.yaml
	@kubectl apply -f manifests/deployment.yaml
	@echo "Deployment complete"

# Deploy only the CRD
deploy-crd:
	@echo "Deploying CRD..."
	@kubectl apply -f manifests/crd.yaml

# Deploy only the bot
deploy-bot:
	@echo "Deploying bot..."
	@kubectl apply -f manifests/rbac.yaml
	@kubectl apply -f manifests/deployment.yaml

# Delete deployment
delete:
	@echo "Deleting deployment..."
	@kubectl delete -f manifests/deployment.yaml --ignore-not-found
	@kubectl delete -f manifests/rbac.yaml --ignore-not-found
	@kubectl delete -f manifests/crd.yaml --ignore-not-found
	@echo "Deletion complete"

# Run linter (requires golangci-lint)
lint:
	@echo "Running golangci-lint..."
	@which golangci-lint > /dev/null || (echo "golangci-lint not found. Install: https://golangci-lint.run/usage/install/" && exit 1)
	@golangci-lint run ./...

# Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...

# Run go vet
vet:
	@echo "Running go vet..."
	@go vet ./...

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	@go mod download
	@go mod tidy

# Build for multiple platforms
build-all:
	@echo "Building for multiple platforms..."
	@mkdir -p bin
	@GOOS=linux GOARCH=amd64 go build -o bin/$(BINARY_NAME)-linux-amd64 $(MAIN_PATH)
	@GOOS=linux GOARCH=arm64 go build -o bin/$(BINARY_NAME)-linux-arm64 $(MAIN_PATH)
	@GOOS=darwin GOARCH=amd64 go build -o bin/$(BINARY_NAME)-darwin-amd64 $(MAIN_PATH)
	@GOOS=darwin GOARCH=arm64 go build -o bin/$(BINARY_NAME)-darwin-arm64 $(MAIN_PATH)
	@echo "Binaries built in bin/"

# Check Kubernetes deployment status
status:
	@echo "Checking deployment status..."
	@kubectl get deployment telegram-bot
	@kubectl get pods -l app=telegram-bot
	@echo ""
	@echo "Recent logs:"
	@kubectl logs -l app=telegram-bot --tail=20

# View logs
logs:
	@kubectl logs -l app=telegram-bot -f

# Create example permission
create-example-permission:
	@echo "Creating example permission..."
	@kubectl apply -f - <<EOF
	apiVersion: telegram.k8s.io/v1
	kind: TelegramBotPermission
	metadata:
	  name: example-user-123456789
	spec:
	  telegramUserId: 123456789
	  role: viewer
	  permissions:
	    - namespace: "default"
	      resources: ["pods"]
	      verbs: ["get", "list", "logs"]
	EOF

# List all permissions
list-permissions:
	@echo "Listing all TelegramBotPermissions..."
	@kubectl get telegrambotpermissions

# Development: run all checks before commit
pre-commit: fmt vet test
	@echo "Pre-commit checks passed!"

# CI: run all tests and checks
ci: deps vet test test-coverage
	@echo "CI checks passed!"
