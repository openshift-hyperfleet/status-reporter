# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /build

# Copy go mod files for dependency caching
COPY go.mod go.sum ./

# Download and verify dependencies
RUN go mod download && go mod verify

# Copy source code
COPY . .

# Build binary for amd64 (most common k8s node architecture)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o status-reporter ./cmd/reporter

# Runtime stage
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/status-reporter /app/status-reporter

ENTRYPOINT ["/app/status-reporter"]

LABEL name="status-reporter" \
      vendor="Red Hat" \
      version="0.0.1" \
      summary="Status Reporter - Kubernetes Job status reporter for adapter" \
      description="Monitors adapter execution, parses results, and updates Kubernetes Job status conditions based on adapter outcomes"
