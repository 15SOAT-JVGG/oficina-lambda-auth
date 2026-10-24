// Package config lê a configuração da função das variáveis de ambiente preenchidas pelo Terraform.
package config

import (
	"errors"
	"fmt"
	"math"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultPort              = "5432"
	defaultSSLMode           = "require"
	defaultIssuer            = "oficina-api"
	defaultExpirationMinutes = 60
	// maxExpirationMinutes é o maior valor que ainda cabe em time.Duration sem estourar.
	maxExpirationMinutes = int(math.MaxInt64 / int64(time.Minute))
)

// Config reúne tudo o que a função precisa para falar com o banco e assinar tokens.
type Config struct {
	DBHost     string
	DBPort     string
	DBName     string
	DBUsername string
	DBPassword string
	DBSSLMode  string

	JWTPrivateKeyPEM string
	JWTIssuer        string
	TokenLifetime    time.Duration
}

// Lookup resolve uma variável de ambiente; os.LookupEnv satisfaz a assinatura.
type Lookup func(key string) (string, bool)

// FromEnv lê e valida a configuração. A chave privada chega em base64 porque variável de ambiente
// não carrega quebra de linha de PEM com segurança.
func FromEnv(lookup Lookup) (Config, error) {
	// present trata valor só com espaços como ausente, mas devolve o valor original: a normalização
	// fica a cargo de quem chama, porque a senha do banco não pode ser alterada.
	present := func(key string) (string, bool) {
		value, ok := lookup(key)
		return value, ok && strings.TrimSpace(value) != ""
	}
	get := func(key, fallback string) string {
		if value, ok := present(key); ok {
			return strings.TrimSpace(value)
		}
		return fallback
	}

	var problems []error
	requiredRaw := func(key string) string {
		value, ok := present(key)
		if !ok {
			problems = append(problems, fmt.Errorf("variável %s é obrigatória", key))
		}
		return value
	}
	required := func(key string) string {
		return strings.TrimSpace(requiredRaw(key))
	}

	cfg := Config{
		DBHost:     required("DB_HOST"),
		DBPort:     get("DB_PORT", defaultPort),
		DBName:     required("DB_NAME"),
		DBUsername: required("DB_USERNAME"),
		DBPassword: requiredRaw("DB_PASSWORD"),
		DBSSLMode:  get("DB_SSLMODE", defaultSSLMode),
		JWTIssuer:  get("JWT_ISSUER", defaultIssuer),
	}

	privateKey, err := decodePrivateKey(required("JWT_PRIVATE_KEY_B64"))
	if err != nil {
		problems = append(problems, err)
	}
	cfg.JWTPrivateKeyPEM = privateKey

	minutes, err := strconv.Atoi(get("JWT_EXPIRATION_MINUTES", strconv.Itoa(defaultExpirationMinutes)))
	if err != nil || minutes <= 0 || minutes > maxExpirationMinutes {
		problems = append(problems, fmt.Errorf("JWT_EXPIRATION_MINUTES precisa ser um inteiro entre 1 e %d", maxExpirationMinutes))
	}
	cfg.TokenLifetime = time.Duration(minutes) * time.Minute

	if len(problems) > 0 {
		return Config{}, errors.Join(problems...)
	}
	return cfg, nil
}

// ConnectionString monta a URL do Postgres escapando usuário e senha, que podem ter caracteres reservados.
func (c Config) ConnectionString() string {
	connection := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(c.DBUsername, c.DBPassword),
		Host:     net.JoinHostPort(c.DBHost, c.DBPort),
		Path:     c.DBName,
		RawQuery: url.Values{"sslmode": {c.DBSSLMode}}.Encode(),
	}
	return connection.String()
}
