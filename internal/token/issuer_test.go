package token_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/15SOAT-JVGG/oficina-lambda-auth/internal/auth"
	"github.com/15SOAT-JVGG/oficina-lambda-auth/internal/token"
)

const issuerName = "oficina-api"

var fixedNow = time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)

func newKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func pkcs8PEM(t *testing.T, key *rsa.PrivateKey) string {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

func TestIssueFor(t *testing.T) {
	key := newKey(t)
	customer := auth.Customer{ID: uuid.New(), Active: true}
	issuer := token.NewIssuer(key, issuerName, time.Hour, func() time.Time { return fixedNow })

	issued, err := issuer.IssueFor(customer)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	parsed, err := jwt.Parse(issued.Token, func(*jwt.Token) (any, error) { return &key.PublicKey, nil },
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithIssuer(issuerName),
		jwt.WithTimeFunc(func() time.Time { return fixedNow }),
	)
	if err != nil {
		t.Fatalf("a API não aceitaria o token: %v", err)
	}
	claims := parsed.Claims.(jwt.MapClaims)

	t.Run("identifica o cliente em sub", func(t *testing.T) {
		if claims["sub"] != customer.ID.String() {
			t.Errorf("sub = %v, esperado %v", claims["sub"], customer.ID)
		}
	})
	t.Run("concede o papel CUSTOMER em groups", func(t *testing.T) {
		groups, _ := claims["groups"].([]any)
		if len(groups) != 1 || groups[0] != "CUSTOMER" {
			t.Errorf("groups = %v, esperado [CUSTOMER]", claims["groups"])
		}
	})
	t.Run("vale uma hora a partir da emissão", func(t *testing.T) {
		issuedAt, _ := claims.GetIssuedAt()
		expiresAt, _ := claims.GetExpirationTime()
		if !issuedAt.Time.Equal(fixedNow) || expiresAt.Sub(issuedAt.Time) != time.Hour {
			t.Errorf("iat = %v, exp = %v", issuedAt, expiresAt)
		}
	})
	t.Run("carrega um jti único", func(t *testing.T) {
		other, err := issuer.IssueFor(customer)
		if err != nil {
			t.Fatal(err)
		}
		first, _, _ := jwt.NewParser().ParseUnverified(issued.Token, jwt.MapClaims{})
		second, _, _ := jwt.NewParser().ParseUnverified(other.Token, jwt.MapClaims{})
		if first.Claims.(jwt.MapClaims)["jti"] == second.Claims.(jwt.MapClaims)["jti"] {
			t.Error("dois tokens não podem repartir o jti")
		}
	})
	t.Run("declara typ JWT e algoritmo RS256", func(t *testing.T) {
		if parsed.Header["typ"] != "JWT" || parsed.Header["alg"] != "RS256" {
			t.Errorf("header = %v", parsed.Header)
		}
	})
	t.Run("informa o contrato da resposta", func(t *testing.T) {
		if issued.CustomerID != customer.ID || issued.Role != "CUSTOMER" || issued.ExpiresIn != 3600 {
			t.Errorf("resposta inesperada: %+v", issued)
		}
	})
}

func TestTokenDeOutraChaveNaoVale(t *testing.T) {
	issuer := token.NewIssuer(newKey(t), issuerName, time.Hour, time.Now)
	issued, err := issuer.IssueFor(auth.Customer{ID: uuid.New(), Active: true})
	if err != nil {
		t.Fatal(err)
	}
	apiKey := newKey(t)

	_, err = jwt.Parse(issued.Token, func(*jwt.Token) (any, error) { return &apiKey.PublicKey, nil })

	if err == nil {
		t.Fatal("a API deveria recusar token assinado com outra chave")
	}
}

func TestParsePrivateKey(t *testing.T) {
	key := newKey(t)

	t.Run("lê chave PKCS#8", func(t *testing.T) {
		parsed, err := token.ParsePrivateKey(pkcs8PEM(t, key))
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if !parsed.Equal(key) {
			t.Error("a chave lida difere da original")
		}
	})
	t.Run("recusa texto que não é PEM", func(t *testing.T) {
		if _, err := token.ParsePrivateKey("não é uma chave"); err == nil {
			t.Error("esperado erro")
		}
	})
	t.Run("recusa PKCS#1, que a API não usa", func(t *testing.T) {
		pkcs1 := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
		if _, err := token.ParsePrivateKey(pkcs1); err == nil {
			t.Error("esperado erro")
		}
	})
	t.Run("recusa PEM com conteúdo ilegível", func(t *testing.T) {
		broken := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("lixo")}))
		if _, err := token.ParsePrivateKey(broken); err == nil {
			t.Error("esperado erro")
		}
	})
	t.Run("recusa chave que não é RSA", func(t *testing.T) {
		if _, err := token.ParsePrivateKey(ecKeyPEM(t)); err == nil {
			t.Error("esperado erro")
		}
	})
}
