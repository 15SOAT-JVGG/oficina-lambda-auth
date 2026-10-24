//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/15SOAT-JVGG/oficina-lambda-auth/internal/cpf"
	"github.com/15SOAT-JVGG/oficina-lambda-auth/internal/postgres"
)

// O schema reproduz as colunas que a API mapeia em CustomerJpaEntity e que a migration V3 completa com
// active. É esse o contrato entre a função e o banco da API.
const schema = `
	CREATE SCHEMA oficina_mecanica;
	CREATE TABLE oficina_mecanica.customer (
		customer_id   UUID PRIMARY KEY,
		first_name    VARCHAR(255) NOT NULL,
		phone_number  VARCHAR(255) NOT NULL,
		document      VARCHAR(255) NOT NULL UNIQUE,
		document_type VARCHAR(255) NOT NULL,
		active        BOOLEAN NOT NULL DEFAULT TRUE
	);`

func mustCPF(t *testing.T, raw string) cpf.CPF {
	t.Helper()
	parsed, err := cpf.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func insertCustomer(t *testing.T, conn *pgx.Conn, id uuid.UUID, document, documentType string, active bool) {
	t.Helper()
	_, err := conn.Exec(context.Background(), `
		INSERT INTO oficina_mecanica.customer (customer_id, first_name, phone_number, document, document_type, active)
		VALUES ($1, 'Cliente', '+5511999999999', $2, $3, $4)`, id, document, documentType, active)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCustomers(t *testing.T) {
	ctx := context.Background()
	container, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("oficina"), tcpostgres.WithUsername("oficina_admin"), tcpostgres.WithPassword("secret"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("subir o Postgres: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	connectionString, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	admin, err := pgx.Connect(ctx, connectionString)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(ctx) })
	if _, err := admin.Exec(ctx, schema); err != nil {
		t.Fatal(err)
	}

	activeID, inactiveID := uuid.New(), uuid.New()
	insertCustomer(t, admin, activeID, "52998224725", "CPF", true)
	insertCustomer(t, admin, inactiveID, "12345678909", "CPF", false)
	insertCustomer(t, admin, uuid.New(), "11222333000181", "CNPJ", true)

	t.Run("encontra o cliente ativo pelo CPF normalizado", func(t *testing.T) {
		customers := postgres.NewCustomers(connectionString)
		t.Cleanup(func() { _ = customers.Close(ctx) })

		customer, err := customers.FindByCPF(ctx, mustCPF(t, "529.982.247-25"))

		if err != nil || customer == nil {
			t.Fatalf("customer = %v, err = %v", customer, err)
		}
		if customer.ID != activeID || !customer.Active {
			t.Errorf("cliente inesperado: %+v", customer)
		}
	})

	t.Run("devolve o cliente inativo com active falso", func(t *testing.T) {
		customers := postgres.NewCustomers(connectionString)
		t.Cleanup(func() { _ = customers.Close(ctx) })

		customer, err := customers.FindByCPF(ctx, mustCPF(t, "123.456.789-09"))

		if err != nil || customer == nil {
			t.Fatalf("customer = %v, err = %v", customer, err)
		}
		if customer.ID != inactiveID || customer.Active {
			t.Errorf("cliente inesperado: %+v", customer)
		}
	})

	t.Run("devolve nil para CPF sem cadastro", func(t *testing.T) {
		customers := postgres.NewCustomers(connectionString)
		t.Cleanup(func() { _ = customers.Close(ctx) })

		customer, err := customers.FindByCPF(ctx, mustCPF(t, "111.444.777-35"))

		if err != nil || customer != nil {
			t.Errorf("customer = %v, err = %v; esperado nil, nil", customer, err)
		}
	})

	t.Run("reabre a conexão derrubada entre invocações", func(t *testing.T) {
		customers := postgres.NewCustomers(connectionString)
		t.Cleanup(func() { _ = customers.Close(ctx) })
		if _, err := customers.FindByCPF(ctx, mustCPF(t, "529.982.247-25")); err != nil {
			t.Fatal(err)
		}
		if _, err := admin.Exec(ctx, `
			SELECT pg_terminate_backend(pid) FROM pg_stat_activity
			WHERE datname = 'oficina' AND pid <> pg_backend_pid()`); err != nil {
			t.Fatal(err)
		}

		customer, err := customers.FindByCPF(ctx, mustCPF(t, "529.982.247-25"))

		if err != nil || customer == nil || customer.ID != activeID {
			t.Errorf("customer = %v, err = %v; esperado reconexão transparente", customer, err)
		}
	})

	t.Run("falha quando o banco está inalcançável", func(t *testing.T) {
		customers := postgres.NewCustomers("postgres://x:y@127.0.0.1:1/oficina?sslmode=disable&connect_timeout=1")

		_, err := customers.FindByCPF(ctx, mustCPF(t, "529.982.247-25"))

		if err == nil {
			t.Error("esperado erro de conexão")
		}
	})

	t.Run("Close sem conexão aberta não falha", func(t *testing.T) {
		if err := postgres.NewCustomers(connectionString).Close(ctx); err != nil {
			t.Errorf("erro inesperado: %v", err)
		}
	})
}
