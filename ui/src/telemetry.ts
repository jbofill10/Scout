import { WebTracerProvider } from '@opentelemetry/sdk-trace-web';
import { Resource } from '@opentelemetry/resources';
import { SemanticResourceAttributes } from '@opentelemetry/semantic-conventions';
import { registerInstrumentations } from '@opentelemetry/instrumentation';
import { getWebAutoInstrumentations } from '@opentelemetry/auto-instrumentations-web';
import { OTLPTraceExporter } from '@opentelemetry/exporter-trace-otlp-http';
import { BatchSpanProcessor } from '@opentelemetry/sdk-trace-base';

export function initTelemetry() {
  // Create a tracer provider
  const provider = new WebTracerProvider({
    resource: new Resource({
      [SemanticResourceAttributes.SERVICE_NAME]: 'scout-ui',
      [SemanticResourceAttributes.SERVICE_VERSION]: '1.0.0',
    }),
  });

  // Configure OTLP exporter to send traces to OTel Collector via nginx proxy
  const exporter = new OTLPTraceExporter({
    url: '/v1/traces',
    headers: {},
  });

  // Add batch span processor
  provider.addSpanProcessor(new BatchSpanProcessor(exporter));

  // Register the provider
  provider.register();

  // Register auto-instrumentations for web
  registerInstrumentations({
    instrumentations: [
      getWebAutoInstrumentations({
        '@opentelemetry/instrumentation-document-load': {},
        '@opentelemetry/instrumentation-user-interaction': {},
        '@opentelemetry/instrumentation-fetch': {
          propagateTraceHeaderCorsUrls: [
            /\/api\/.*/,
            /http:\/\/localhost:22920\/.*/,
          ],
        },
        '@opentelemetry/instrumentation-xml-http-request': {
          propagateTraceHeaderCorsUrls: [
            /\/api\/.*/,
            /http:\/\/localhost:22920\/.*/,
          ],
        },
      }),
    ],
  });

  console.log('OpenTelemetry initialized for browser');
}
