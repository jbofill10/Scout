# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Install build dependencies required for cgo and sqlite
RUN apk add --no-cache build-base sqlite-dev pkgconfig

# Copy go mod files first
COPY torrenter/go.mod torrenter/go.sum ./

# Copy shared modules for local replace
COPY shared/media /shared/media
COPY shared/telemetry /shared/telemetry

# Download dependencies first (before copying source)
RUN go mod download

# Copy all source code at once
COPY torrenter/ .

# Build the binary with cgo enabled (required by mattn/go-sqlite3)
# Keep it dynamically linked to the sqlite library provided in the runtime image
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags "-s -w" -o torrenter ./cmd/torrenter

# Runtime stage

FROM alpine:3.18

# Runtime deps: ca-certificates and sqlite library
RUN apk --no-cache add ca-certificates sqlite-libs

WORKDIR /root/

# Copy the binary from builder stage
COPY --from=builder /app/torrenter .

# Expose port
EXPOSE 22001

# Run the binary
CMD ["./torrenter"]