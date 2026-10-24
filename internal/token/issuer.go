// Package token emite o JWT no contrato que a API da oficina já valida.
package token

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/15SOAT-JVGG/oficina-lambda-auth/internal/auth"
)

// CustomerRole é o papel que separa o cliente das rotas administrativas da API.
const CustomerRole = "CUSTOMER"

const pkcs8PEMType = "PRIVATE KEY"

type claims struct {
	jwt.RegisteredClaims
	Groups []string `json:"groups"`
}

// Issuer assina com a mesma chave privada RS256 da API, com o mesmo issuer, o id do cliente em sub e o
// papel em groups (MicroProfile JWT). Para a API o token é indistinguível de um que ela mesma emitiu.
type Issuer struct {
	key      *rsa.PrivateKey
	issuer   string
	lifetime time.Duration
	now      func() time.Time
}

// NewIssuer cria o emissor. O relógio é injetado para tornar a validade verificável em teste.
func NewIssuer(key *rsa.PrivateKey, issuer string, lifetime time.Duration, now func() time.Time) *Issuer {
	return &Issuer{key: key, issuer: issuer, lifetime: lifetime, now: now}
}

// IssueFor assina um token para o cliente.
func (i *Issuer) IssueFor(customer auth.Customer) (auth.IssuedToken, error) {
	issuedAt := i.now()
	signed, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    i.issuer,
			Subject:   customer.ID.String(),
			ID:        uuid.NewString(),
			IssuedAt:  jwt.NewNumericDate(issuedAt),
			ExpiresAt: jwt.NewNumericDate(issuedAt.Add(i.lifetime)),
		},
		Groups: []string{CustomerRole},
	}).SignedString(i.key)
	if err != nil {
		return auth.IssuedToken{}, fmt.Errorf("assinar o token do cliente: %w", err)
	}
	return auth.IssuedToken{
		Token:      signed,
		CustomerID: customer.ID,
		Role:       CustomerRole,
		ExpiresIn:  int64(i.lifetime.Seconds()),
	}, nil
}

// ParsePrivateKey lê a chave no formato que a API usa: PKCS#8 em PEM.
func ParsePrivateKey(pemText string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemText))
	if block == nil || block.Type != pkcs8PEMType {
		return nil, errors.New("a chave privada precisa estar em PEM PKCS#8 (" + pkcs8PEMType + ")")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("chave privada ilegível: %w", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("a chave privada precisa ser RSA")
	}
	return key, nil
}
