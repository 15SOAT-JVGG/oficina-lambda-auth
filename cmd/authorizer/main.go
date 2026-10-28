// Command authorizer validates customer and staff access tokens for API Gateway.
package main

import (
	"encoding/base64"
	"log/slog"
	"os"
	"strings"

	"github.com/aws/aws-lambda-go/lambda"

	"github.com/15SOAT-JVGG/oficina-lambda-auth/internal/authorizer"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	publicKeyB64 := strings.TrimSpace(os.Getenv("JWT_PUBLIC_KEY_B64"))
	issuer := strings.TrimSpace(os.Getenv("JWT_ISSUER"))
	publicKeyPEM, err := base64.StdEncoding.DecodeString(publicKeyB64)
	if err != nil {
		logger.Error("configuration_invalid", "error", "JWT_PUBLIC_KEY_B64 must be valid standard base64")
		os.Exit(1)
	}
	key, err := authorizer.ParsePublicKey(string(publicKeyPEM))
	if err != nil {
		logger.Error("configuration_invalid", "error", err.Error())
		os.Exit(1)
	}
	validator, err := authorizer.New(key, issuer)
	if err != nil {
		logger.Error("configuration_invalid", "error", err.Error())
		os.Exit(1)
	}
	lambda.Start(validator.Handle)
}
