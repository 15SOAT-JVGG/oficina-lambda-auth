package cpf_test

import (
	"errors"
	"testing"

	"github.com/15SOAT-JVGG/oficina-lambda-auth/internal/cpf"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		digits  string
		wantErr bool
	}{
		{name: "aceita CPF só com dígitos", raw: "52998224725", digits: "52998224725"},
		{name: "aceita CPF formatado", raw: "529.982.247-25", digits: "52998224725"},
		{name: "aceita CPF com espaços", raw: " 529 982 247 25 ", digits: "52998224725"},
		{name: "aceita CPF cujo resto é menor que 2", raw: "12345678909", digits: "12345678909"},
		{name: "recusa CPF vazio", raw: "", wantErr: true},
		{name: "recusa CPF só com letras", raw: "abc", wantErr: true},
		{name: "recusa CPF curto", raw: "5299822472", wantErr: true},
		{name: "recusa CPF longo", raw: "529982247251", wantErr: true},
		{name: "recusa CNPJ", raw: "11222333000181", wantErr: true},
		{name: "recusa dígitos repetidos", raw: "111.111.111-11", wantErr: true},
		{name: "recusa primeiro dígito verificador errado", raw: "52998224735", wantErr: true},
		{name: "recusa segundo dígito verificador errado", raw: "52998224726", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := cpf.Parse(tt.raw)

			if tt.wantErr {
				if !errors.Is(err, cpf.ErrInvalid) {
					t.Fatalf("Parse(%q) erro = %v, esperado ErrInvalid", tt.raw, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse(%q) erro inesperado: %v", tt.raw, err)
			}
			if got.Digits() != tt.digits {
				t.Errorf("Digits() = %q, esperado %q", got.Digits(), tt.digits)
			}
		})
	}
}

func TestStringMascaraOCPF(t *testing.T) {
	parsed, err := cpf.Parse("529.982.247-25")
	if err != nil {
		t.Fatal(err)
	}

	if got, want := parsed.String(), "***.982.***-25"; got != want {
		t.Errorf("String() = %q, esperado %q", got, want)
	}
}
