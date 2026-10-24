// Package auth contém o caso de uso de autenticação do cliente pelo CPF e as portas de que ele depende.
package auth

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/15SOAT-JVGG/oficina-lambda-auth/internal/cpf"
)

// Motivos de recusa. Vão só para o log: quem chama sempre recebe a mesma resposta, para que testar
// CPFs não revele quem é cliente da oficina.
const (
	ReasonNotFound = "not-found"
	ReasonInactive = "inactive"
)

// Customer é o recorte do cliente de que a autenticação precisa.
type Customer struct {
	ID     uuid.UUID
	Active bool
}

// IssuedToken é o token entregue ao cliente, com o que a resposta HTTP precisa informar.
type IssuedToken struct {
	Token      string
	CustomerID uuid.UUID
	Role       string
	ExpiresIn  int64
}

// NotAuthorizedError indica CPF sem cadastro ou cliente inativo.
type NotAuthorizedError struct {
	Reason string
}

func (e *NotAuthorizedError) Error() string {
	return "Cliente não autorizado"
}

// CustomerLookup consulta o cliente na base. Devolve nil, nil quando o CPF não tem cadastro.
type CustomerLookup interface {
	FindByCPF(ctx context.Context, document cpf.CPF) (*Customer, error)
}

// TokenIssuer assina o token que a API já sabe validar.
type TokenIssuer interface {
	IssueFor(customer Customer) (IssuedToken, error)
}

// Service orquestra validação do CPF, consulta do cliente e emissão do token.
type Service struct {
	customers CustomerLookup
	tokens    TokenIssuer
}

// NewService monta o caso de uso com as portas de saída.
func NewService(customers CustomerLookup, tokens TokenIssuer) *Service {
	return &Service{customers: customers, tokens: tokens}
}

// Authenticate valida o CPF e, se o cliente existe e está ativo, emite o token.
func (s *Service) Authenticate(ctx context.Context, rawCPF string) (IssuedToken, error) {
	document, err := cpf.Parse(rawCPF)
	if err != nil {
		return IssuedToken{}, err
	}

	customer, err := s.customers.FindByCPF(ctx, document)
	if err != nil {
		return IssuedToken{}, fmt.Errorf("consultar cliente: %w", err)
	}
	if customer == nil {
		return IssuedToken{}, &NotAuthorizedError{Reason: ReasonNotFound}
	}
	if !customer.Active {
		return IssuedToken{}, &NotAuthorizedError{Reason: ReasonInactive}
	}

	issued, err := s.tokens.IssueFor(*customer)
	if err != nil {
		return IssuedToken{}, fmt.Errorf("emitir token: %w", err)
	}
	return issued, nil
}
