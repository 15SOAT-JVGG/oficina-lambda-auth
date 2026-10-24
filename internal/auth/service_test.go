package auth_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/15SOAT-JVGG/oficina-lambda-auth/internal/auth"
	"github.com/15SOAT-JVGG/oficina-lambda-auth/internal/cpf"
)

const validCPF = "529.982.247-25"

type fakeCustomers struct {
	customer *auth.Customer
	err      error
	lookedUp []string
}

func (f *fakeCustomers) FindByCPF(_ context.Context, document cpf.CPF) (*auth.Customer, error) {
	f.lookedUp = append(f.lookedUp, document.Digits())
	return f.customer, f.err
}

type fakeTokens struct {
	issued []auth.Customer
	err    error
}

func (f *fakeTokens) IssueFor(customer auth.Customer) (auth.IssuedToken, error) {
	if f.err != nil {
		return auth.IssuedToken{}, f.err
	}
	f.issued = append(f.issued, customer)
	return auth.IssuedToken{Token: "signed.jwt.token", CustomerID: customer.ID, Role: "CUSTOMER", ExpiresIn: 3600}, nil
}

func TestAuthenticate(t *testing.T) {
	customerID := uuid.New()

	t.Run("emite o token para cliente ativo", func(t *testing.T) {
		customers := &fakeCustomers{customer: &auth.Customer{ID: customerID, Active: true}}
		tokens := &fakeTokens{}

		issued, err := auth.NewService(customers, tokens).Authenticate(context.Background(), validCPF)

		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if issued.Token != "signed.jwt.token" || issued.CustomerID != customerID {
			t.Errorf("token emitido inesperado: %+v", issued)
		}
		if len(customers.lookedUp) != 1 || customers.lookedUp[0] != "52998224725" {
			t.Errorf("a consulta deve usar o CPF normalizado, usou %v", customers.lookedUp)
		}
	})

	t.Run("recusa CPF inválido sem consultar o banco", func(t *testing.T) {
		customers := &fakeCustomers{}

		_, err := auth.NewService(customers, &fakeTokens{}).Authenticate(context.Background(), "111.111.111-11")

		if !errors.Is(err, cpf.ErrInvalid) {
			t.Fatalf("erro = %v, esperado cpf.ErrInvalid", err)
		}
		if len(customers.lookedUp) != 0 {
			t.Error("não deveria consultar o banco com CPF inválido")
		}
	})

	t.Run("recusa CPF sem cadastro com motivo not-found", func(t *testing.T) {
		tokens := &fakeTokens{}

		_, err := auth.NewService(&fakeCustomers{}, tokens).Authenticate(context.Background(), validCPF)

		var notAuthorized *auth.NotAuthorizedError
		if !errors.As(err, &notAuthorized) || notAuthorized.Reason != auth.ReasonNotFound {
			t.Fatalf("erro = %v, esperado NotAuthorizedError(not-found)", err)
		}
		if len(tokens.issued) != 0 {
			t.Error("não deveria emitir token")
		}
	})

	t.Run("recusa cliente inativo com motivo inactive", func(t *testing.T) {
		customers := &fakeCustomers{customer: &auth.Customer{ID: customerID, Active: false}}
		tokens := &fakeTokens{}

		_, err := auth.NewService(customers, tokens).Authenticate(context.Background(), validCPF)

		var notAuthorized *auth.NotAuthorizedError
		if !errors.As(err, &notAuthorized) || notAuthorized.Reason != auth.ReasonInactive {
			t.Fatalf("erro = %v, esperado NotAuthorizedError(inactive)", err)
		}
		if len(tokens.issued) != 0 {
			t.Error("não deveria emitir token")
		}
	})

	t.Run("propaga falha do banco", func(t *testing.T) {
		databaseDown := errors.New("conexão recusada")
		customers := &fakeCustomers{err: databaseDown}

		_, err := auth.NewService(customers, &fakeTokens{}).Authenticate(context.Background(), validCPF)

		if !errors.Is(err, databaseDown) {
			t.Fatalf("erro = %v, esperado a falha do banco", err)
		}
	})

	t.Run("propaga falha na assinatura", func(t *testing.T) {
		signingFailed := errors.New("chave inválida")
		customers := &fakeCustomers{customer: &auth.Customer{ID: customerID, Active: true}}

		_, err := auth.NewService(customers, &fakeTokens{err: signingFailed}).Authenticate(context.Background(), validCPF)

		if !errors.Is(err, signingFailed) {
			t.Fatalf("erro = %v, esperado a falha de assinatura", err)
		}
	})
}
