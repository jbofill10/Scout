import { logs, SeverityNumber } from '@opentelemetry/api-logs';
import { LoggerProvider, BatchLogRecordProcessor } from '@opentelemetry/sdk-logs';
import { OTLPLogExporter } from '@opentelemetry/exporter-logs-otlp-http';
import { trace, context } from '@opentelemetry/api';

// Initialize the logger provider
let loggerProvider: LoggerProvider | null = null;
let logger: ReturnType<LoggerProvider['getLogger']> | null = null;

export function initLogger() {
  // Configure OTLP exporter to send logs to OTel Collector via nginx proxy
  const exporter = new OTLPLogExporter({
    url: '/v1/logs',
    headers: {},
  });

  // Create batch processor
  const processor = new BatchLogRecordProcessor(exporter);

  // Create a logger provider with batch processor in processors array
  loggerProvider = new LoggerProvider({
    processors: [processor],
  });

  // Register the global logger provider
  logs.setGlobalLoggerProvider(loggerProvider);

  // Get a logger instance with service name
  logger = loggerProvider.getLogger('scout-ui', '1.0.0');

  console.log('OpenTelemetry Logger initialized for browser');
}

/**
 * Extract trace context from the current active span
 */
function getTraceContext(): { traceId?: string; spanId?: string } {
  const span = trace.getActiveSpan();
  if (span) {
    const spanContext = span.spanContext();
    return {
      traceId: spanContext.traceId,
      spanId: spanContext.spanId,
    };
  }
  return {};
}

/**
 * Get current page context (URL, user agent, etc.)
 */
function getPageContext() {
  return {
    url: window.location.href,
    userAgent: navigator.userAgent,
    viewport: `${window.innerWidth}x${window.innerHeight}`,
  };
}

/**
 * Log an error with full context
 */
export function logError(
  message: string,
  error?: Error | unknown,
  additionalContext?: Record<string, unknown>
) {
  // Always log to console for development visibility
  if (error instanceof Error) {
    console.error(message, error, additionalContext);
  } else {
    console.error(message, error, additionalContext);
  }

  // Send to OTLP if logger is initialized
  if (!logger) {
    console.warn('Logger not initialized, skipping OTLP log');
    return;
  }

  const traceContext = getTraceContext();
  const pageContext = getPageContext();

  const logBody: Record<string, unknown> = {
    message,
    ...pageContext,
    ...traceContext,
    ...additionalContext,
  };

  // Add error details if provided
  if (error instanceof Error) {
    logBody.error = {
      name: error.name,
      message: error.message,
      stack: error.stack,
    };
  } else if (error) {
    logBody.error = error;
  }

  logger.emit({
    severityNumber: SeverityNumber.ERROR,
    severityText: 'ERROR',
    body: JSON.stringify(logBody),
    attributes: {
      'log.level': 'error',
      'log.message': message,
      ...(traceContext.traceId && { 'trace_id': traceContext.traceId }),
      ...(traceContext.spanId && { 'span_id': traceContext.spanId }),
    },
    context: context.active(),
  });
}

/**
 * Log a warning with context
 */
export function logWarning(
  message: string,
  additionalContext?: Record<string, unknown>
) {
  console.warn(message, additionalContext);

  if (!logger) {
    return;
  }

  const traceContext = getTraceContext();
  const pageContext = getPageContext();

  const logBody = {
    message,
    ...pageContext,
    ...traceContext,
    ...additionalContext,
  };

  logger.emit({
    severityNumber: SeverityNumber.WARN,
    severityText: 'WARN',
    body: JSON.stringify(logBody),
    attributes: {
      'log.level': 'warn',
      'log.message': message,
      ...(traceContext.traceId && { 'trace_id': traceContext.traceId }),
      ...(traceContext.spanId && { 'span_id': traceContext.spanId }),
    },
    context: context.active(),
  });
}

/**
 * Log info with context
 */
export function logInfo(
  message: string,
  additionalContext?: Record<string, unknown>
) {
  console.log(message, additionalContext);

  if (!logger) {
    return;
  }

  const traceContext = getTraceContext();
  const pageContext = getPageContext();

  const logBody = {
    message,
    ...pageContext,
    ...traceContext,
    ...additionalContext,
  };

  logger.emit({
    severityNumber: SeverityNumber.INFO,
    severityText: 'INFO',
    body: JSON.stringify(logBody),
    attributes: {
      'log.level': 'info',
      'log.message': message,
      ...(traceContext.traceId && { 'trace_id': traceContext.traceId }),
      ...(traceContext.spanId && { 'span_id': traceContext.spanId }),
    },
    context: context.active(),
  });
}

/**
 * Shutdown the logger provider gracefully
 */
export async function shutdownLogger() {
  if (loggerProvider) {
    await loggerProvider.shutdown();
    console.log('Logger provider shut down');
  }
}
