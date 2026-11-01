// Command auth é a função Lambda que autentica o cliente da oficina pelo CPF e devolve o JWT.
package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/lambda"

	"github.com/15SOAT-JVGG/oficina-lambda-auth/internal/auth"
	"github.com/15SOAT-JVGG/oficina-lambda-auth/internal/config"
	"github.com/15SOAT-JVGG/oficina-lambda-auth/internal/handler"
	"github.com/15SOAT-JVGG/oficina-lambda-auth/internal/postgres"
	"github.com/15SOAT-JVGG/oficina-lambda-auth/internal/telemetry"
	"github.com/15SOAT-JVGG/oficina-lambda-auth/internal/token"
)

func main() {
	logger := slog.New(handler.NewServiceLogHandler(
		os.Stdout,
		"customer-auth",
		os.Getenv("APP_ENVIRONMENT"),
		os.Getenv("APP_VERSION"),
	))

	cfg, err := config.FromEnv(os.LookupEnv)
	if err != nil {
		logger.Error("configuration_invalid", "error", err.Error())
		os.Exit(1)
	}
	shutdownTelemetry, err := telemetry.Configure(
		context.Background(),
		os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		"customer-auth",
		os.Getenv("APP_ENVIRONMENT"),
		os.Getenv("APP_VERSION"),
	)
	if err != nil {
		// A indisponibilidade/configuração do exporter nunca impede a autenticação.
		logger.Warn("telemetry_initialization_failed", "reason", "otlp-exporter-unavailable")
		shutdownTelemetry = func() {}
	}
	defer shutdownTelemetry()
	privateKey, err := token.ParsePrivateKey(cfg.JWTPrivateKeyPEM)
	if err != nil {
		logger.Error("configuration_invalid", "error", err.Error())
		os.Exit(1)
	}

	service := auth.NewService(
		postgres.NewCustomers(cfg.ConnectionString()),
		token.NewIssuer(privateKey, cfg.JWTIssuer, cfg.TokenLifetime, time.Now),
	)
	lambda.Start(handler.New(service, logger).Handle)
}
