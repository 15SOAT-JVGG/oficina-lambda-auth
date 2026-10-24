// Package cpf valida o CPF e o normaliza para o formato que a API grava na coluna document.
package cpf

import (
	"errors"
	"strings"
)

const (
	length              = 11
	firstCheckPosition  = 9
	secondCheckPosition = 10
	modulus             = 11
)

// ErrInvalid indica CPF com tamanho, repetição de dígitos ou dígito verificador inválido.
var ErrInvalid = errors.New("CPF inválido")

// CPF é um CPF já validado, reduzido aos 11 dígitos.
type CPF struct {
	digits string
}

// Parse remove a formatação e confere os dígitos verificadores, a mesma regra do Document da API.
func Parse(raw string) (CPF, error) {
	digits := onlyDigits(raw)
	if len(digits) != length || allEqual(digits) || !checkDigitsMatch(digits) {
		return CPF{}, ErrInvalid
	}
	return CPF{digits: digits}, nil
}

// Digits devolve os 11 dígitos, que é o valor gravado na coluna document.
func (c CPF) Digits() string {
	return c.digits
}

// String mascara o CPF de propósito: é dado pessoal e não deve aparecer inteiro em log.
func (c CPF) String() string {
	return "***." + c.digits[3:6] + ".***-" + c.digits[9:]
}

func onlyDigits(raw string) string {
	var digits strings.Builder
	for _, char := range raw {
		if char >= '0' && char <= '9' {
			digits.WriteRune(char)
		}
	}
	return digits.String()
}

func allEqual(digits string) bool {
	return strings.Count(digits, digits[:1]) == len(digits)
}

func checkDigitsMatch(digits string) bool {
	return checkDigit(digits, firstCheckPosition) == digitAt(digits, firstCheckPosition) &&
		checkDigit(digits, secondCheckPosition) == digitAt(digits, secondCheckPosition)
}

func checkDigit(digits string, size int) int {
	sum := 0
	weight := size + 1
	for index := 0; index < size; index++ {
		sum += digitAt(digits, index) * weight
		weight--
	}
	remainder := sum % modulus
	if remainder < 2 {
		return 0
	}
	return modulus - remainder
}

func digitAt(digits string, index int) int {
	return int(digits[index] - '0')
}
