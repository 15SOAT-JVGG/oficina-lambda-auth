// Package telemetry configures optional OTLP tracing for the Lambda runtime.
package telemetry

import (
	"context"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const flushTimeout = 75 * time.Millisecond

var (
	providerMu sync.RWMutex
	provider   *sdktrace.TracerProvider
)

// Configure instala tracing quando há um endpoint privado configurado. Sem endpoint, mantém o
// processo funcional e sem criar IDs de trace que não possam ser exportados.
func Configure(ctx context.Context, endpoint, service, environment, version string) (func(), error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return func() {}, nil
	}

	// O receiver Alloy é plaintext na rede interna; o SG da Lambda limita esse tráfego ao
	// security group do collector configurado no Terraform.
	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}
	attributes := []attribute.KeyValue{attribute.String("service.name", service)}
	if environment != "" {
		attributes = append(attributes, attribute.String("deployment.environment.name", environment))
	}
	if version != "" {
		attributes = append(attributes, attribute.String("service.version", version))
	}
	res, err := resource.New(ctx, resource.WithAttributes(attributes...))
	if err != nil {
		_ = exporter.Shutdown(ctx)
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter), sdktrace.WithResource(res))
	providerMu.Lock()
	provider = tp
	providerMu.Unlock()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	return func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), flushTimeout)
		defer cancel()
		_ = tp.Shutdown(shutdownCtx)
	}, nil
}

// Extract aplica o contexto W3C recebido pelo API Gateway. Chaves HTTP são case-insensitive.
func Extract(ctx context.Context, headers map[string]string) context.Context {
	carrier := propagation.MapCarrier{}
	for key, value := range headers {
		carrier[strings.ToLower(key)] = value
	}
	return otel.GetTextMapPropagator().Extract(ctx, carrier)
}

// StartServer inicia o span da operação de autenticação.
func StartServer(ctx context.Context, operation string) (context.Context, trace.Span) {
	return otel.Tracer("customer-auth").Start(ctx, operation, trace.WithSpanKind(trace.SpanKindServer))
}

// StartDatabase inicia um span com atributos fixos, sem SQL ou dados de cliente.
func StartDatabase(ctx context.Context, operation string) (context.Context, trace.Span) {
	return otel.Tracer("customer-auth").Start(ctx, operation,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.operation.name", "SELECT"),
		),
	)
}

// TraceID devolve apenas um trace ID real associado ao contexto corrente.
func TraceID(ctx context.Context) string {
	spanContext := trace.SpanFromContext(ctx).SpanContext()
	if !spanContext.IsValid() {
		return ""
	}
	return spanContext.TraceID().String()
}

// MarkError registra a falha sem incluir mensagem, query, CPF ou outro dado variável no span.
func MarkError(span trace.Span, errorCode string) {
	span.SetStatus(codes.Error, errorCode)
	span.SetAttributes(attribute.String("error.type", errorCode))
}

// Flush espera um período curto para exportar o span da invocação atual.
func Flush() {
	providerMu.RLock()
	tp := provider
	providerMu.RUnlock()
	if tp == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), flushTimeout)
	defer cancel()
	_ = tp.ForceFlush(ctx)
}
