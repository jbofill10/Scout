# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY tvdb_proxy/go.mod tvdb_proxy/go.sum ./

# Copy shared modules for local replace
COPY shared/media /shared/media
COPY shared/telemetry /shared/telemetry

# Download dependencies
RUN go mod download

# Copy source code
COPY tvdb_proxy/ .

# Build the binary with static linking
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o tvdb_proxy .

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy the binary from builder stage
COPY --from=builder /app/tvdb_proxy .

# Expose port
EXPOSE 22000

# Run the binary
CMD ["./tvdb_proxy"]