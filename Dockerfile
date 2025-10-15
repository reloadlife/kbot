# Build stage
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a -installsuffix cgo \
    -o kubectl-bot ./cmd/bot

# Final stage
FROM alpine:latest

# Install ca-certificates for HTTPS
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/kubectl-bot .

# Run as non-root user
RUN addgroup -g 1000 botuser && \
    adduser -D -u 1000 -G botuser botuser && \
    chown -R botuser:botuser /app

USER botuser

ENTRYPOINT ["/app/kubectl-bot"]
