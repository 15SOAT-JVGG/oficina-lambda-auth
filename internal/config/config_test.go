package config_test

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/15SOAT-JVGG/oficina-lambda-auth/internal/config"
)

const pemText = "-----BEGIN PRIVATE KEY-----\nabc\n-----END PRIVATE KEY-----\n"

func env(values map[string]string) config.Lookup {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}

func completeEnv() map[string]string {
	return map[string]string{
		"DB_HOST":             "db.internal",
		"DB_NAME":             "oficina",
		"DB_USERNAME":         "oficina_admin",
		"DB_PASSWORD":         "s3cret",
		"JWT_PRIVATE_KEY_B64": base64.StdEncoding.EncodeToString([]byte(pemText)),
	}
}

func TestFromEnv(t *testing.T) {
	t.Run("aplica os defaults", func(t *testing.T) {
		cfg, err := config.FromEnv(env(completeEnv()))
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if cfg.DBPort != "5432" || cfg.DBSSLMode != "require" || cfg.JWTIssuer != "oficina-api" {
			t.Errorf("defaults inesperados: %+v", cfg)
		}
		if cfg.TokenLifetime != 60*time.Minute {
			t.Errorf("TokenLifetime = %v, esperado 60m", cfg.TokenLifetime)
		}
		if cfg.JWTPrivateKeyPEM != pemText {
			t.Errorf("a chave deveria ser decodificada de base64, veio %q", cfg.JWTPrivateKeyPEM)
		}
	})

	t.Run("respeita os valores informados", func(t *testing.T) {
		values := completeEnv()
		values["DB_PORT"] = "6543"
		values["DB_SSLMODE"] = "disable"
		values["JWT_ISSUER"] = "outro"
		values["JWT_EXPIRATION_MINUTES"] = "15"

		cfg, err := config.FromEnv(env(values))
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if cfg.DBPort != "6543" || cfg.DBSSLMode != "disable" || cfg.JWTIssuer != "outro" || cfg.TokenLifetime != 15*time.Minute {
			t.Errorf("valores ignorados: %+v", cfg)
		}
	})

	t.Run("lista todas as variáveis obrigatórias ausentes", func(t *testing.T) {
		_, err := config.FromEnv(env(map[string]string{}))
		if err == nil {
			t.Fatal("esperado erro")
		}
		for _, name := range []string{"DB_HOST", "DB_NAME", "DB_USERNAME", "DB_PASSWORD", "JWT_PRIVATE_KEY_B64"} {
			if !strings.Contains(err.Error(), name) {
				t.Errorf("o erro deveria citar %s: %v", name, err)
			}
		}
	})

	t.Run("trata valor em branco como ausente", func(t *testing.T) {
		values := completeEnv()
		values["DB_HOST"] = "   "

		if _, err := config.FromEnv(env(values)); err == nil || !strings.Contains(err.Error(), "DB_HOST") {
			t.Errorf("esperado erro citando DB_HOST: %v", err)
		}
	})

	t.Run("recusa chave que não é base64", func(t *testing.T) {
		values := completeEnv()
		values["JWT_PRIVATE_KEY_B64"] = "%%%"

		if _, err := config.FromEnv(env(values)); err == nil || !strings.Contains(err.Error(), "base64") {
			t.Errorf("esperado erro de base64: %v", err)
		}
	})

	t.Run("recusa validade que não é inteiro positivo", func(t *testing.T) {
		for _, invalid := range []string{"abc", "0", "-5"} {
			values := completeEnv()
			values["JWT_EXPIRATION_MINUTES"] = invalid

			if _, err := config.FromEnv(env(values)); err == nil {
				t.Errorf("JWT_EXPIRATION_MINUTES=%q deveria ser recusado", invalid)
			}
		}
	})
}

func TestConnectionString(t *testing.T) {
	cfg := config.Config{
		DBHost: "db.internal", DBPort: "5432", DBName: "oficina",
		DBUsername: "oficina_admin", DBPassword: "p@ss/w:rd?", DBSSLMode: "require",
	}

	got := cfg.ConnectionString()

	want := "postgres://oficina_admin:p%40ss%2Fw%3Ard%3F@db.internal:5432/oficina?sslmode=require"
	if got != want {
		t.Errorf("ConnectionString() = %q, esperado %q", got, want)
	}
}
