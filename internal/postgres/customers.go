// Package postgres lê o cliente da mesma tabela que a API grava.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel/attribute"

	"github.com/15SOAT-JVGG/oficina-lambda-auth/internal/auth"
	"github.com/15SOAT-JVGG/oficina-lambda-auth/internal/cpf"
	"github.com/15SOAT-JVGG/oficina-lambda-auth/internal/telemetry"
)

const findByCPF = `
	SELECT customer_id, active
	FROM oficina_mecanica.customer
	WHERE document = $1 AND document_type = 'CPF'`

// Customers mantém uma única conexão entre invocações do mesmo ambiente de execução: abrir TLS com o
// RDS a cada login dobraria a latência, e pool seria desperdício porque a Lambda entrega uma
// invocação por vez a cada ambiente.
type Customers struct {
	connectionString string
	conn             *pgx.Conn
}

// NewCustomers cria o adaptador. A conexão só é aberta na primeira consulta.
func NewCustomers(connectionString string) *Customers {
	return &Customers{connectionString: connectionString}
}

// FindByCPF devolve o cliente com aquele CPF, ou nil, nil quando não há cadastro.
//
// Não há ping antes da consulta: seria uma ida e volta a mais em todo login quente. Se a conexão
// guardada morreu entre invocações (RDS reiniciou, idle timeout), a própria consulta falha e o pgx
// marca a conexão como fechada; aí reabrimos uma vez e repetimos.
func (c *Customers) FindByCPF(ctx context.Context, document cpf.CPF) (*auth.Customer, error) {
	customer, err := c.find(ctx, document)
	if err != nil && c.connectionLost() {
		customer, err = c.find(ctx, document)
	}
	return customer, err
}

func (c *Customers) find(ctx context.Context, document cpf.CPF) (*auth.Customer, error) {
	ctx, span := telemetry.StartDatabase(ctx, "customer.lookup")
	defer span.End()

	conn, err := c.connection(ctx)
	if err != nil {
		telemetry.MarkError(span, "database.connection_error")
		return nil, err
	}

	var (
		id     uuid.UUID
		active bool
	)
	err = conn.QueryRow(ctx, findByCPF, document.Digits()).Scan(&id, &active)
	if errors.Is(err, pgx.ErrNoRows) {
		span.SetAttributes(attribute.String("db.result", "not_found"))
		return nil, nil
	}
	if err != nil {
		telemetry.MarkError(span, "database.query_error")
		return nil, fmt.Errorf("consultar cliente por CPF: %w", err)
	}
	span.SetAttributes(attribute.String("db.result", "found"))
	return &auth.Customer{ID: id, Active: active}, nil
}

func (c *Customers) connectionLost() bool {
	return c.conn != nil && c.conn.IsClosed()
}

func (c *Customers) connection(ctx context.Context) (*pgx.Conn, error) {
	if c.conn != nil && !c.conn.IsClosed() {
		return c.conn, nil
	}
	if c.conn != nil {
		// Já está fechada do lado do pgx; Close só libera o que restou e não devolve erro útil aqui.
		_ = c.conn.Close(ctx)
		c.conn = nil
	}
	conn, err := pgx.Connect(ctx, c.connectionString)
	if err != nil {
		return nil, fmt.Errorf("conectar ao banco: %w", err)
	}
	c.conn = conn
	return conn, nil
}

// Close encerra a conexão, se houver.
func (c *Customers) Close(ctx context.Context) error {
	if c.conn == nil {
		return nil
	}
	return c.conn.Close(ctx)
}
