package telemetry

import "strings"

// normalizeOTLPEndpoint converts an OTLP endpoint into the bare "host:port" form the
// gRPC exporters require, and reports whether the connection should be insecure.
//
// OTEL_EXPORTER_OTLP_ENDPOINT is conventionally written as a URL ("http://host:4317"),
// but otlptracegrpc/otlploggrpc WithEndpoint expects "host:port" with no scheme. Passing
// the raw URL produces an invalid gRPC target and every span and log is dropped silently
// by the batch processor, so tolerate both spellings here.
func normalizeOTLPEndpoint(endpoint string) (host string, insecure bool) {
	insecure = true

	switch {
	case strings.HasPrefix(endpoint, "https://"):
		endpoint = strings.TrimPrefix(endpoint, "https://")
		insecure = false
	case strings.HasPrefix(endpoint, "http://"):
		endpoint = strings.TrimPrefix(endpoint, "http://")
	}

	// Drop any path component ("host:4317/v1/traces"); the gRPC target is host:port only.
	if idx := strings.IndexByte(endpoint, '/'); idx >= 0 {
		endpoint = endpoint[:idx]
	}

	return endpoint, insecure
}
