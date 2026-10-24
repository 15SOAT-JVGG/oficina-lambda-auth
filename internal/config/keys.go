package config

import (
	"encoding/base64"
	"errors"
	"strings"
)

func decodePrivateKey(encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return "", errors.New("JWT_PRIVATE_KEY_B64 não é base64 válido")
	}
	return string(decoded), nil
}
