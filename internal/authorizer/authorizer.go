// Package authorizer validates the access JWT before API Gateway forwards a request.
package authorizer

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

const (
	groupCustomer = "CUSTOMER"
	groupAdmin    = "ADMIN"
	groupMechanic = "MECHANIC"
)

// Claims is the JWT contract shared with the Quarkus API. Purpose-scoped link JWTs deliberately
// have no access groups; they are validated by the application only on their public endpoints.
type Claims struct {
	jwt.RegisteredClaims
	Groups  []string `json:"groups"`
	Purpose string   `json:"purpose,omitempty"`
}

// SimpleResponse is the API Gateway HTTP API payload 2.0 simple authorizer response.
type SimpleResponse struct {
	IsAuthorized bool           `json:"isAuthorized"`
	Context      map[string]any `json:"context,omitempty"`
}

// Event contains the Authorization identity source defined for the REQUEST authorizer.
type Event struct {
	IdentitySource []string          `json:"identitySource"`
	Headers        map[string]string `json:"headers"`
}

// Validator verifies signed access tokens using only the public key and configured issuer.
type Validator struct {
	key    *rsa.PublicKey
	issuer string
}

// ParsePublicKey reads an RSA public key encoded as PEM SubjectPublicKeyInfo.
func ParsePublicKey(pemText string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemText))
	if block == nil || block.Type != "PUBLIC KEY" {
		return nil, errors.New("JWT public key must be PEM SubjectPublicKeyInfo (PUBLIC KEY)")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("invalid JWT public key: %w", err)
	}
	key, ok := parsed.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("JWT public key must be RSA")
	}
	return key, nil
}

// New creates a validator. Both configuration values are required to avoid a permissive default.
func New(key *rsa.PublicKey, issuer string) (*Validator, error) {
	if key == nil {
		return nil, errors.New("JWT public key is required")
	}
	if strings.TrimSpace(issuer) == "" {
		return nil, errors.New("JWT issuer is required")
	}
	return &Validator{key: key, issuer: issuer}, nil
}

// Handle returns a simple allow/deny response. Invalid identities are denied without returning
// token details or claims to API Gateway logs.
func (v *Validator) Handle(_ context.Context, event Event) (SimpleResponse, error) {
	value := ""
	if len(event.IdentitySource) > 0 {
		value = event.IdentitySource[0]
	}
	if value == "" {
		for name, candidate := range event.Headers {
			if strings.EqualFold(name, "authorization") {
				value = candidate
				break
			}
		}
	}

	token := bearerToken(value)
	if token == "" {
		return SimpleResponse{IsAuthorized: false}, nil
	}

	claims := &Claims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodRS256 {
			return nil, fmt.Errorf("unexpected signing method %s", token.Method.Alg())
		}
		return v.key, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}), jwt.WithIssuer(v.issuer), jwt.WithExpirationRequired())
	if err != nil || parsed == nil || !parsed.Valid || !validAccessClaims(claims) {
		return SimpleResponse{IsAuthorized: false}, nil
	}

	return SimpleResponse{
		IsAuthorized: true,
		Context: map[string]any{
			"sub":    claims.Subject,
			"groups": strings.Join(claims.Groups, ","),
		},
	}, nil
}

func bearerToken(value string) string {
	parts := strings.Fields(value)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}

func validAccessClaims(claims *Claims) bool {
	if claims == nil || claims.Subject == "" || claims.Issuer == "" || claims.ExpiresAt == nil || claims.Purpose != "" {
		return false
	}
	for _, group := range claims.Groups {
		switch group {
		case groupCustomer, groupAdmin, groupMechanic:
			return true
		}
	}
	return false
}
