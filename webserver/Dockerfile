# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Install build dependencies required for cgo and sqlite
RUN apk add --no-cache build-base sqlite-dev pkgconfig

# Copy go mod files
COPY webserver/go.mod webserver/go.sum ./

# Copy shared modules for local replace
COPY shared/media /shared/media
COPY shared/status /shared/status
COPY shared/telemetry /shared/telemetry

# Download dependencies
RUN go mod download

# Copy source code
COPY webserver/ .

# Build the binary with cgo enabled (required by mattn/go-sqlite3)
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags "-s -w" -o webserver ./cmd/webserver

# Runtime stage

FROM alpine:3.18

# Runtime deps: ca-certificates and sqlite library
RUN apk --no-cache add ca-certificates sqlite-libs

WORKDIR /root/

# Copy the binary from builder stage
COPY --from=builder /app/webserver .

# Expose port
EXPOSE 22920

# Run the binary
CMD ["./webserver"]