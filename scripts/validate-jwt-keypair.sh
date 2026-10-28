#!/usr/bin/env bash
set -euo pipefail

if [[ -z "${JWT_PRIVATE_KEY_B64:-}" ]]; then
    echo "JWT_PRIVATE_KEY_B64 é obrigatório." >&2
    exit 1
fi

command -v openssl >/dev/null || { echo "openssl não encontrado" >&2; exit 1; }
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
umask 077

if ! printf '%s' "$JWT_PRIVATE_KEY_B64" | base64 --decode > "$workdir/private.pem" 2>/dev/null; then
    echo "JWT_PRIVATE_KEY_B64 precisa estar codificada em base64 válido." >&2
    exit 1
fi

if ! openssl pkey -in "$workdir/private.pem" -check -noout >/dev/null 2>&1 ||
    ! openssl pkey -in "$workdir/private.pem" -pubout -out "$workdir/derived-public.pem" \
        >/dev/null 2>&1 ||
    ! openssl pkey -pubin -in "$workdir/derived-public.pem" -noout >/dev/null 2>&1; then
    echo "JWT_PRIVATE_KEY_B64 não contém uma chave RSA válida." >&2
    exit 1
fi

if [[ -n "${JWT_PUBLIC_KEY_B64:-}" ]]; then
    if ! printf '%s' "$JWT_PUBLIC_KEY_B64" | base64 --decode > "$workdir/public.pem" 2>/dev/null ||
        ! openssl pkey -pubin -in "$workdir/public.pem" -noout >/dev/null 2>&1 ||
        ! openssl pkey -in "$workdir/private.pem" -pubout -outform DER \
            -out "$workdir/derived-public.der" >/dev/null 2>&1 ||
        ! openssl pkey -pubin -in "$workdir/public.pem" -pubout -outform DER \
            -out "$workdir/public.der" >/dev/null 2>&1 ||
        ! cmp -s "$workdir/derived-public.der" "$workdir/public.der"; then
        echo "A chave pública não corresponde à chave privada JWT." >&2
        exit 1
    fi
else
    JWT_PUBLIC_KEY_B64="$(base64 < "$workdir/derived-public.pem" | tr -d '\n')"
    if [[ -n "${GITHUB_ENV:-}" ]]; then
        printf 'JWT_PUBLIC_KEY_B64=%s\nTF_VAR_jwt_public_key_b64=%s\n' \
            "$JWT_PUBLIC_KEY_B64" "$JWT_PUBLIC_KEY_B64" >> "$GITHUB_ENV"
    fi
fi

echo "Chave pública JWT validada para o Authorizer."
