package handler_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/google/uuid"

	"github.com/15SOAT-JVGG/oficina-lambda-auth/internal/auth"
	"github.com/15SOAT-JVGG/oficina-lambda-auth/internal/cpf"
	"github.com/15SOAT-JVGG/oficina-lambda-auth/internal/handler"
)

const requestID = "req-123"

type fakeAuthenticator struct {
	issued   auth.IssuedToken
	err      error
	received string
}

func (f *fakeAuthenticator) Authenticate(_ context.Context, rawCPF string) (auth.IssuedToken, error) {
	f.received = rawCPF
	return f.issued, f.err
}

func newHandler(authenticator handler.Authenticator) (*handler.Handler, *bytes.Buffer) {
	logs := &bytes.Buffer{}
	return handler.New(authenticator, slog.New(handler.NewLogHandler(logs))), logs
}

func request(body string) events.APIGatewayV2HTTPRequest {
	return events.APIGatewayV2HTTPRequest{
		Body:           body,
		RequestContext: events.APIGatewayV2HTTPRequestContext{RequestID: requestID},
	}
}

func decode(t *testing.T, response events.APIGatewayV2HTTPResponse) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal([]byte(response.Body), &body); err != nil {
		t.Fatalf("o corpo não é JSON: %q", response.Body)
	}
	return body
}

func TestHandle(t *testing.T) {
	customerID := uuid.New()

	t.Run("200 devolve o token e o contrato da resposta", func(t *testing.T) {
		authenticator := &fakeAuthenticator{issued: auth.IssuedToken{
			Token: "signed.jwt.token", CustomerID: customerID, Role: "CUSTOMER", ExpiresIn: 3600,
		}}
		h, logs := newHandler(authenticator)

		response, err := h.Handle(context.Background(), request(`{"cpf":"529.982.247-25"}`))

		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if response.StatusCode != 200 || response.Headers["Content-Type"] != "application/json" {
			t.Fatalf("resposta inesperada: %d %v", response.StatusCode, response.Headers)
		}
		body := decode(t, response)
		if body["token"] != "signed.jwt.token" || body["customerId"] != customerID.String() ||
			body["role"] != "CUSTOMER" || body["expiresIn"] != float64(3600) {
			t.Errorf("corpo inesperado: %v", body)
		}
		if authenticator.received != "529.982.247-25" {
			t.Errorf("CPF repassado = %q", authenticator.received)
		}
		if strings.Contains(logs.String(), "529") {
			t.Errorf("o CPF não pode aparecer no log: %s", logs)
		}
		if !strings.Contains(logs.String(), `"event":"customer_authenticated"`) {
			t.Errorf("faltou o log de sucesso: %s", logs)
		}
	})

	t.Run("200 lê corpo em base64", func(t *testing.T) {
		authenticator := &fakeAuthenticator{issued: auth.IssuedToken{Token: "t", CustomerID: customerID, Role: "CUSTOMER"}}
		h, _ := newHandler(authenticator)
		event := request(base64.StdEncoding.EncodeToString([]byte(`{"cpf":"52998224725"}`)))
		event.IsBase64Encoded = true

		response, _ := h.Handle(context.Background(), event)

		if response.StatusCode != 200 || authenticator.received != "52998224725" {
			t.Errorf("status = %d, cpf = %q", response.StatusCode, authenticator.received)
		}
	})

	t.Run("200 ignora campos desconhecidos", func(t *testing.T) {
		authenticator := &fakeAuthenticator{issued: auth.IssuedToken{Token: "t", CustomerID: customerID, Role: "CUSTOMER"}}
		h, _ := newHandler(authenticator)

		response, _ := h.Handle(context.Background(), request(`{"cpf":"52998224725","extra":1}`))

		if response.StatusCode != 200 {
			t.Errorf("status = %d", response.StatusCode)
		}
	})

	malformed := map[string]events.APIGatewayV2HTTPRequest{
		"corpo ausente":        request(""),
		"corpo em branco":      request("   "),
		"JSON inválido":        request("{cpf"),
		"sem o campo cpf":      request(`{}`),
		"cpf nulo":             request(`{"cpf":null}`),
		"cpf que não é string": request(`{"cpf":123}`),
		"base64 inválido": func() events.APIGatewayV2HTTPRequest {
			event := request("%%%")
			event.IsBase64Encoded = true
			return event
		}(),
	}
	for name, event := range malformed {
		t.Run("400 INVALID_REQUEST com "+name, func(t *testing.T) {
			authenticator := &fakeAuthenticator{}
			h, _ := newHandler(authenticator)

			response, _ := h.Handle(context.Background(), event)

			body := decode(t, response)
			if response.StatusCode != 400 || body["code"] != "INVALID_REQUEST" || body["type"] != "VALIDATION" {
				t.Errorf("resposta inesperada: %d %v", response.StatusCode, body)
			}
			if authenticator.received != "" {
				t.Error("não deveria chamar a autenticação")
			}
		})
	}

	t.Run("400 INVALID_CPF", func(t *testing.T) {
		h, logs := newHandler(&fakeAuthenticator{err: cpf.ErrInvalid})

		response, _ := h.Handle(context.Background(), request(`{"cpf":"111.111.111-11"}`))

		body := decode(t, response)
		if response.StatusCode != 400 || body["code"] != "INVALID_CPF" || body["type"] != "VALIDATION" {
			t.Errorf("resposta inesperada: %d %v", response.StatusCode, body)
		}
		if !strings.Contains(logs.String(), `"reason":"invalid-cpf"`) {
			t.Errorf("faltou o motivo no log: %s", logs)
		}
	})

	t.Run("401 CUSTOMER_NOT_AUTHORIZED é igual para cliente inexistente e inativo", func(t *testing.T) {
		var messages []string
		for _, reason := range []string{auth.ReasonNotFound, auth.ReasonInactive} {
			h, logs := newHandler(&fakeAuthenticator{err: &auth.NotAuthorizedError{Reason: reason}})

			response, _ := h.Handle(context.Background(), request(`{"cpf":"52998224725"}`))

			body := decode(t, response)
			if response.StatusCode != 401 || body["code"] != "CUSTOMER_NOT_AUTHORIZED" || body["type"] != "UNAUTHORIZED" {
				t.Fatalf("resposta inesperada: %d %v", response.StatusCode, body)
			}
			if !strings.Contains(logs.String(), `"reason":"`+reason+`"`) {
				t.Errorf("o motivo %q deveria ficar no log: %s", reason, logs)
			}
			messages = append(messages, response.Body)
		}
		if decodeMessage(t, messages[0]) != decodeMessage(t, messages[1]) {
			t.Error("as duas recusas devem ser indistinguíveis para quem chama")
		}
	})

	t.Run("500 INTERNAL_ERROR esconde a causa e a registra no log", func(t *testing.T) {
		h, logs := newHandler(&fakeAuthenticator{err: errors.New("conexão recusada em 10.0.0.5")})

		response, _ := h.Handle(context.Background(), request(`{"cpf":"52998224725"}`))

		body := decode(t, response)
		if response.StatusCode != 500 || body["code"] != "INTERNAL_ERROR" {
			t.Fatalf("resposta inesperada: %d %v", response.StatusCode, body)
		}
		if _, hasType := body["type"]; hasType {
			t.Error("o erro interno não declara type")
		}
		if strings.Contains(response.Body, "10.0.0.5") {
			t.Error("a causa não pode vazar na resposta")
		}
		if !strings.Contains(logs.String(), "conexão recusada em 10.0.0.5") || !strings.Contains(logs.String(), `"level":"ERROR"`) {
			t.Errorf("a causa deveria ir ao log: %s", logs)
		}
	})

	t.Run("o erro segue o formato ApiErrorResponseDto da API", func(t *testing.T) {
		h, _ := newHandler(&fakeAuthenticator{err: cpf.ErrInvalid})

		response, _ := h.Handle(context.Background(), request(`{"cpf":"1"}`))

		body := decode(t, response)
		for _, field := range []string{"status", "type", "code", "message", "path", "traceId", "timestamp"} {
			if _, ok := body[field]; !ok {
				t.Errorf("faltou o campo %q: %v", field, body)
			}
		}
		if body["path"] != "/v1/auth/customer" || body["traceId"] != requestID || body["status"] != float64(400) {
			t.Errorf("campos inesperados: %v", body)
		}
	})
}

func decodeMessage(t *testing.T, raw string) string {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatal(err)
	}
	return body["message"].(string)
}
