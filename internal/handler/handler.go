// Package handler adapta o evento do API Gateway HTTP (payload 2.0) ao caso de uso de autenticação.
// As respostas de erro seguem o formato do ApiErrorResponseDto da API, para que o cliente trate um único
// contrato de erro, venha ele da função ou do cluster.
package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"

	"github.com/15SOAT-JVGG/oficina-lambda-auth/internal/auth"
	"github.com/15SOAT-JVGG/oficina-lambda-auth/internal/cpf"
	"github.com/15SOAT-JVGG/oficina-lambda-auth/internal/telemetry"
)

const (
	path = "/v1/auth/customer"

	codeInvalidRequest = "INVALID_REQUEST"
	codeInvalidCPF     = "INVALID_CPF"
	codeNotAuthorized  = "CUSTOMER_NOT_AUTHORIZED"
	codeInternalError  = "INTERNAL_ERROR"

	typeValidation   = "VALIDATION"
	typeUnauthorized = "UNAUTHORIZED"
)

var errMalformedRequest = errors.New("requisição malformada")

// Authenticator é o caso de uso que o handler aciona.
type Authenticator interface {
	Authenticate(ctx context.Context, rawCPF string) (auth.IssuedToken, error)
}

// Handler traduz o evento do API Gateway em chamada ao caso de uso e o resultado em resposta HTTP.
type Handler struct {
	authenticator Authenticator
	logger        *slog.Logger
}

// New cria o handler.
func New(authenticator Authenticator, logger *slog.Logger) *Handler {
	return &Handler{authenticator: authenticator, logger: logger}
}

type loginRequest struct {
	CPF *string `json:"cpf"`
}

type loginResponse struct {
	Token      string `json:"token"`
	CustomerID string `json:"customerId"`
	Role       string `json:"role"`
	ExpiresIn  int64  `json:"expiresIn"`
}

type errorResponse struct {
	Status    int    `json:"status"`
	Type      string `json:"type,omitempty"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	Path      string `json:"path"`
	RequestID string `json:"requestId"`
	TraceID   string `json:"traceId,omitempty"`
	Timestamp string `json:"timestamp"`
}

// Handle responde ao POST /v1/auth/customer. O erro devolvido é sempre nil: toda falha vira resposta HTTP.
func (h *Handler) Handle(ctx context.Context, event events.APIGatewayV2HTTPRequest) (response events.APIGatewayV2HTTPResponse, returnedErr error) {
	startedAt := time.Now()
	ctx = telemetry.Extract(ctx, event.Headers)
	ctx, span := telemetry.StartServer(ctx, "POST /v1/auth/customer")
	defer func() {
		span.End()
		telemetry.Flush()
	}()
	requestID := event.RequestContext.RequestID
	if strings.TrimSpace(requestID) == "" {
		requestID = uuid.NewString()
	}
	awsRequestID := ""
	if lambdaContext, ok := lambdacontext.FromContext(ctx); ok {
		awsRequestID = lambdaContext.AwsRequestID
	}
	logger := h.logger.With("requestId", requestID, "awsRequestId", awsRequestID, "operation", "customer-authenticate")
	if traceID := telemetry.TraceID(ctx); traceID != "" {
		logger = logger.With("traceId", traceID)
	}
	statusCode := 0
	result := "failure"
	errorCode := ""
	defer func() {
		if returnedErr != nil {
			result = "error"
		}
		logger.Info("request_completed",
			"statusCode", statusCode,
			"outcome", result,
			"errorCode", errorCode,
			"durationMs", time.Since(startedAt).Milliseconds(),
		)
		span.SetAttributes(
			attribute.Int("http.response.status_code", statusCode),
			attribute.String("app.outcome", result),
		)
	}()

	rawCPF, err := readCPF(event)
	if err != nil {
		logger.Warn("customer_auth_rejected", "reason", "malformed-request")
		statusCode, errorCode = http.StatusBadRequest, codeInvalidRequest
		response = failure(ctx, statusCode, typeValidation, errorCode, `Corpo esperado: {"cpf": "..."}`, requestID)
		return response, nil
	}

	issued, err := h.authenticator.Authenticate(ctx, rawCPF)

	var notAuthorized *auth.NotAuthorizedError
	switch {
	case err == nil:
		result = "success"
		logger.Info("customer_authenticated")
		statusCode = http.StatusOK
		response = success(issued, requestID)
		return response, nil
	case errors.Is(err, cpf.ErrInvalid):
		logger.Warn("customer_auth_rejected", "reason", "invalid-cpf")
		statusCode, errorCode = http.StatusBadRequest, codeInvalidCPF
		response = failure(ctx, statusCode, typeValidation, errorCode, err.Error(), requestID)
		return response, nil
	case errors.As(err, &notAuthorized):
		logger.Warn("customer_auth_rejected", "reason", notAuthorized.Reason)
		statusCode, errorCode = http.StatusUnauthorized, codeNotAuthorized
		response = failure(ctx, statusCode, typeUnauthorized, errorCode, notAuthorized.Error(), requestID)
		return response, nil
	default:
		logger.Error("customer_auth_failed", "error", err.Error())
		telemetry.MarkError(span, codeInternalError)
		statusCode, errorCode = http.StatusInternalServerError, codeInternalError
		response = failure(ctx, statusCode, "", errorCode, "Falha ao autenticar o cliente", requestID)
		return response, nil
	}
}

func readCPF(event events.APIGatewayV2HTTPRequest) (string, error) {
	body := event.Body
	if strings.TrimSpace(body) == "" {
		return "", errMalformedRequest
	}
	if event.IsBase64Encoded {
		decoded, err := base64.StdEncoding.DecodeString(body)
		if err != nil {
			return "", errMalformedRequest
		}
		body = string(decoded)
	}

	var request loginRequest
	if err := json.Unmarshal([]byte(body), &request); err != nil {
		return "", errMalformedRequest
	}
	if request.CPF == nil {
		return "", errMalformedRequest
	}
	return *request.CPF, nil
}

func success(issued auth.IssuedToken, requestID string) events.APIGatewayV2HTTPResponse {
	return respond(http.StatusOK, loginResponse{
		Token:      issued.Token,
		CustomerID: issued.CustomerID.String(),
		Role:       issued.Role,
		ExpiresIn:  issued.ExpiresIn,
	}, requestID)
}

func failure(ctx context.Context, status int, errorType, code, message, requestID string) events.APIGatewayV2HTTPResponse {
	return respond(status, errorResponse{
		Status:    status,
		Type:      errorType,
		Code:      code,
		Message:   message,
		Path:      path,
		RequestID: requestID,
		TraceID:   telemetry.TraceID(ctx),
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
	}, requestID)
}

func respond(status int, body any, requestID string) events.APIGatewayV2HTTPResponse {
	payload, err := json.Marshal(body)
	if err != nil {
		// Os tipos de resposta são estruturas fixas: falhar aqui é bug de programação, não de entrada.
		panic("serializar resposta: " + err.Error())
	}
	headers := map[string]string{
		"Content-Type": "application/json",
		// RFC 6749 §5.1: resposta com token não pode ser guardada por navegador, CDN ou proxy.
		"Cache-Control": "no-store",
		"Pragma":        "no-cache",
	}
	if requestID != "" {
		headers["X-Request-ID"] = requestID
	}
	return events.APIGatewayV2HTTPResponse{
		StatusCode: status,
		Headers:    headers,
		Body:       string(payload),
	}
}
