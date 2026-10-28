package authorizer

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testIssuer = "oficina-api"

func TestHandle(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	validator, err := New(&key.PublicKey, testIssuer)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		claims    Claims
		signing   *rsa.PrivateKey
		algorithm jwt.SigningMethod
		identity  string
		allowed   bool
	}{
		{name: "customer access token", claims: accessClaims(), signing: key, algorithm: jwt.SigningMethodRS256, identity: "Bearer %s", allowed: true},
		{name: "staff access token", claims: Claims{RegisteredClaims: registeredClaims(), Groups: []string{"MECHANIC"}}, signing: key, algorithm: jwt.SigningMethodRS256, identity: "Bearer %s", allowed: true},
		{name: "missing identity", claims: accessClaims(), signing: key, algorithm: jwt.SigningMethodRS256, identity: "", allowed: false},
		{name: "missing subject", claims: Claims{RegisteredClaims: jwt.RegisteredClaims{Issuer: testIssuer, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}, Groups: []string{"CUSTOMER"}}, signing: key, algorithm: jwt.SigningMethodRS256, identity: "Bearer %s", allowed: false},
		{name: "missing expiration", claims: Claims{RegisteredClaims: jwt.RegisteredClaims{Issuer: testIssuer, Subject: "customer-1"}, Groups: []string{"CUSTOMER"}}, signing: key, algorithm: jwt.SigningMethodRS256, identity: "Bearer %s", allowed: false},
		{name: "wrong issuer", claims: Claims{RegisteredClaims: jwt.RegisteredClaims{Issuer: "other", Subject: "customer-1", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}, Groups: []string{"CUSTOMER"}}, signing: key, algorithm: jwt.SigningMethodRS256, identity: "Bearer %s", allowed: false},
		{name: "expired", claims: expiredClaims(), signing: key, algorithm: jwt.SigningMethodRS256, identity: "Bearer %s", allowed: false},
		{name: "not yet valid", claims: notYetValidClaims(), signing: key, algorithm: jwt.SigningMethodRS256, identity: "Bearer %s", allowed: false},
		{name: "purpose scoped tracking link", claims: purposeClaims("work-order-tracking"), signing: key, algorithm: jwt.SigningMethodRS256, identity: "Bearer %s", allowed: false},
		{name: "unknown group", claims: Claims{RegisteredClaims: registeredClaims(), Groups: []string{"GUEST"}}, signing: key, algorithm: jwt.SigningMethodRS256, identity: "Bearer %s", allowed: false},
		{name: "wrong signing key", claims: accessClaims(), signing: otherKey(t), algorithm: jwt.SigningMethodRS256, identity: "Bearer %s", allowed: false},
		{name: "wrong algorithm", claims: accessClaims(), signing: key, algorithm: jwt.SigningMethodHS256, identity: "Bearer %s", allowed: false},
		{name: "malformed bearer scheme", claims: accessClaims(), signing: key, algorithm: jwt.SigningMethodRS256, identity: "Basic %s", allowed: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			signed := jwt.NewWithClaims(test.algorithm, &test.claims)
			var signingKey any = test.signing
			if test.algorithm == jwt.SigningMethodHS256 {
				signingKey = []byte("not-an-rsa-key")
			}
			raw, signErr := signed.SignedString(signingKey)
			if signErr != nil {
				t.Fatal(signErr)
			}
			identity := test.identity
			if identity != "" {
				identity = formatIdentity(identity, raw)
			}
			response, handleErr := validator.Handle(context.Background(), Event{IdentitySource: []string{identity}})
			if handleErr != nil {
				t.Fatalf("Handle() error = %v", handleErr)
			}
			if response.IsAuthorized != test.allowed {
				t.Fatalf("IsAuthorized = %t, want %t", response.IsAuthorized, test.allowed)
			}
		})
	}
}

func TestHandleReadsAuthorizationHeaderCaseInsensitively(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	validator, _ := New(&key.PublicKey, testIssuer)
	response, err := validator.Handle(context.Background(), Event{
		Headers: map[string]string{"aUtHoRiZaTiOn": "Bearer " + signClaims(t, key, accessClaims())},
	})
	if err != nil || !response.IsAuthorized {
		t.Fatalf("Handle() = (%+v, %v), want authorized", response, err)
	}
}

func TestParsePublicKey(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	pemText := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
	if _, err := ParsePublicKey(pemText); err != nil {
		t.Fatalf("ParsePublicKey(valid) error = %v", err)
	}
	if _, err := ParsePublicKey("not a key"); err == nil {
		t.Fatal("ParsePublicKey(invalid) should fail")
	}
}

func accessClaims() Claims {
	return Claims{RegisteredClaims: registeredClaims(), Groups: []string{"CUSTOMER"}}
}

func registeredClaims() jwt.RegisteredClaims {
	return jwt.RegisteredClaims{
		Issuer:    testIssuer,
		Subject:   "customer-1",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		NotBefore: jwt.NewNumericDate(time.Now().Add(-time.Minute)),
	}
}

func expiredClaims() Claims {
	claims := accessClaims()
	claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Minute))
	return claims
}

func notYetValidClaims() Claims {
	claims := accessClaims()
	claims.NotBefore = jwt.NewNumericDate(time.Now().Add(time.Hour))
	return claims
}

func purposeClaims(purpose string) Claims {
	claims := accessClaims()
	claims.Purpose = purpose
	return claims
}

func otherKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func signClaims(t *testing.T, key *rsa.PrivateKey, claims Claims) string {
	t.Helper()
	raw, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func formatIdentity(format, token string) string {
	return strings.Fields(format)[0] + " " + token
}
