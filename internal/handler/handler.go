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

	"github.com/15SOAT-JVGG/oficina-lambda-auth/internal/auth"
	"github.com/15SOAT-JVGG/oficina-lambda-auth/internal/cpf"
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
	TraceID   string `json:"traceId"`
	Timestamp string `json:"timestamp"`
}

// Handle responde ao POST /v1/auth/customer. O erro devolvido é sempre nil: toda falha vira resposta HTTP.
func (h *Handler) Handle(ctx context.Context, event events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	requestID := event.RequestContext.RequestID
	logger := h.logger.With("requestId", requestID)

	rawCPF, err := readCPF(event)
	if err != nil {
		logger.Warn("customer_auth_rejected", "reason", "malformed-request")
		return failure(http.StatusBadRequest, typeValidation, codeInvalidRequest, `Corpo esperado: {"cpf": "..."}`, requestID), nil
	}

	issued, err := h.authenticator.Authenticate(ctx, rawCPF)

	var notAuthorized *auth.NotAuthorizedError
	switch {
	case err == nil:
		logger.Info("customer_authenticated", "customerId", issued.CustomerID.String())
		return success(issued), nil
	case errors.Is(err, cpf.ErrInvalid):
		logger.Warn("customer_auth_rejected", "reason", "invalid-cpf")
		return failure(http.StatusBadRequest, typeValidation, codeInvalidCPF, err.Error(), requestID), nil
	case errors.As(err, &notAuthorized):
		logger.Warn("customer_auth_rejected", "reason", notAuthorized.Reason)
		return failure(http.StatusUnauthorized, typeUnauthorized, codeNotAuthorized, notAuthorized.Error(), requestID), nil
	default:
		logger.Error("customer_auth_failed", "error", err.Error())
		return failure(http.StatusInternalServerError, "", codeInternalError, "Falha ao autenticar o cliente", requestID), nil
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

func success(issued auth.IssuedToken) events.APIGatewayV2HTTPResponse {
	return respond(http.StatusOK, loginResponse{
		Token:      issued.Token,
		CustomerID: issued.CustomerID.String(),
		Role:       issued.Role,
		ExpiresIn:  issued.ExpiresIn,
	})
}

func failure(status int, errorType, code, message, requestID string) events.APIGatewayV2HTTPResponse {
	return respond(status, errorResponse{
		Status:    status,
		Type:      errorType,
		Code:      code,
		Message:   message,
		Path:      path,
		TraceID:   requestID,
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func respond(status int, body any) events.APIGatewayV2HTTPResponse {
	payload, err := json.Marshal(body)
	if err != nil {
		// Os tipos de resposta são estruturas fixas: falhar aqui é bug de programação, não de entrada.
		panic("serializar resposta: " + err.Error())
	}
	return events.APIGatewayV2HTTPResponse{
		StatusCode: status,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       string(payload),
	}
}
