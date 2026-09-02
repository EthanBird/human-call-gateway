# Multi-stage Dockerfile for Human Call Gateway
# Stage 1: Build the Go binary
FROM golang:1.22-alpine AS builder

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o gateway ./cmd/gateway

# Stage 2: Create minimal runtime image
FROM alpine:latest

# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy the binary from builder
COPY --from=builder /build/gateway .

# Copy test config as default
COPY deploy/config.test.yaml /app/config.test.yaml

# Set default environment variables
ENV PORT=8080
ENV CONFIG_PATH=/app/config.test.yaml

# Expose the port
EXPOSE 8080

# Run the gateway
CMD ["./gateway"]
