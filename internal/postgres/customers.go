// Package postgres lê o cliente da mesma tabela que a API grava.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/15SOAT-JVGG/oficina-lambda-auth/internal/auth"
	"github.com/15SOAT-JVGG/oficina-lambda-auth/internal/cpf"
)

const (
	findByCPF = `
		SELECT customer_id, active
		FROM oficina_mecanica.customer
		WHERE document = $1 AND document_type = 'CPF'`

	pingTimeout = 2 * time.Second
)

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
func (c *Customers) FindByCPF(ctx context.Context, document cpf.CPF) (*auth.Customer, error) {
	conn, err := c.connection(ctx)
	if err != nil {
		return nil, err
	}

	var (
		id     uuid.UUID
		active bool
	)
	err = conn.QueryRow(ctx, findByCPF, document.Digits()).Scan(&id, &active)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("consultar cliente por CPF: %w", err)
	}
	return &auth.Customer{ID: id, Active: active}, nil
}

func (c *Customers) connection(ctx context.Context) (*pgx.Conn, error) {
	if c.conn != nil && c.isAlive(ctx) {
		return c.conn, nil
	}
	conn, err := pgx.Connect(ctx, c.connectionString)
	if err != nil {
		return nil, fmt.Errorf("conectar ao banco: %w", err)
	}
	c.conn = conn
	return conn, nil
}

func (c *Customers) isAlive(ctx context.Context) bool {
	pingContext, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	return c.conn.Ping(pingContext) == nil
}

// Close encerra a conexão, se houver.
func (c *Customers) Close(ctx context.Context) error {
	if c.conn == nil {
		return nil
	}
	return c.conn.Close(ctx)
}
