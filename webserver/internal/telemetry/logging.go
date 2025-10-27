package telemetry

import (
	"context"
	"log/slog"
	"os"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// InitLogger initializes the OpenTelemetry logger provider and returns a slog.Logger
func InitLogger(serviceName, serviceVersion, otlpEndpoint string) (*slog.Logger, func(), error) {
	ctx := context.Background()

	// Create OTLP log exporter
	conn, err := grpc.Dial(
		otlpEndpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, nil, err
	}

	exporter, err := otlploggrpc.New(ctx, otlploggrpc.WithGRPCConn(conn))
	if err != nil {
		return nil, nil, err
	}

	// Create resource with service information
	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(serviceName),
		semconv.ServiceVersion(serviceVersion),
	)

	// Create logger provider
	lp := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter)),
		sdklog.WithResource(res),
	)

	// Create slog handler that bridges to OTel logs
	// This handler automatically correlates logs with traces/spans via context
	handler := otelslog.NewHandler(serviceName, otelslog.WithLoggerProvider(lp))

	// Create logger with the OTel handler
	logger := slog.New(handler)

	// Set as default logger so all slog calls use this handler
	slog.SetDefault(logger)

	// Also set the standard log package to use slog for legacy compatibility
	slog.SetLogLoggerLevel(slog.LevelDebug)

	// Return cleanup function
	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := lp.Shutdown(ctx); err != nil {
			// Use stderr to avoid circular logging
			os.Stderr.WriteString("Error shutting down logger provider: " + err.Error() + "\n")
		}
	}

	return logger, cleanup, nil
}

// WithTraceContext extracts trace_id and span_id from context and adds them to log attributes
// This ensures trace correlation works even when logs are batched and exported after span closes
func WithTraceContext(ctx context.Context, args ...any) []any {
	span := trace.SpanFromContext(ctx)
	spanCtx := span.SpanContext()

	if spanCtx.IsValid() {
		args = append(args,
			"trace_id", spanCtx.TraceID().String(),
			"span_id", spanCtx.SpanID().String(),
		)
	}

	return args
}
