// Package telemetry configures the OpenTelemetry meter provider.
package telemetry

import (
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.uber.org/zap"
)

// Setup builds a meter provider that exports metrics over OTLP/HTTP. All
// exporter settings (endpoint, headers, temporality, export interval) come
// from the standard OTEL_* environment variables, so none are set in code:
// code options would override the environment. When no OTLP endpoint is
// configured, metrics are disabled and a no-op provider is returned.
//
// The returned shutdown function flushes pending metrics and must be called
// before the Redis cache is closed, because the final collection runs the
// unique-count gauge callbacks.
func Setup(ctx context.Context, logger *zap.Logger) (metric.MeterProvider, func(context.Context) error, error) {
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" && os.Getenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT") == "" {
		logger.Info("OTEL_EXPORTER_OTLP_ENDPOINT not set, metrics export disabled")
		return noop.NewMeterProvider(), func(context.Context) error { return nil }, nil
	}

	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		logger.Warn("OpenTelemetry error", zap.Error(err))
	}))

	exporter, err := otlpmetrichttp.New(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create OTLP metric exporter: %w", err)
	}

	// WithService first so OTEL_SERVICE_NAME / OTEL_RESOURCE_ATTRIBUTES from
	// WithFromEnv override its defaults. WithService also sets a random
	// service.instance.id, keeping each replica's counters separate.
	res, err := resource.New(ctx,
		resource.WithService(),
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
	)
	if err != nil {
		// resource.New can return a usable partial resource alongside an error.
		logger.Warn("Failed to fully detect OpenTelemetry resource", zap.Error(err))
		if res == nil {
			res = resource.Default()
		}
	}

	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)),
	)
	logger.Info("OpenTelemetry metrics export enabled")
	return provider, provider.Shutdown, nil
}
