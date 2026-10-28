variable "aws_region" {
  description = "Região AWS onde os recursos são criados."
  type        = string
  default     = "us-east-1"
}

variable "environment" {
  description = "Nome do ambiente (homolog ou production)."
  type        = string

  validation {
    condition     = contains(["homolog", "production"], var.environment)
    error_message = "O ambiente deve ser 'homolog' ou 'production'."
  }
}

variable "state_bucket" {
  description = "Bucket S3 onde ficam os states dos repositórios de infraestrutura, lidos para descobrir banco, rede e API Gateway."
  type        = string
}

variable "db_password" {
  description = "Senha do banco do ambiente. Fornecida via TF_VAR_db_password (secret DB_PASSWORD do Environment), nunca versionada."
  type        = string
  sensitive   = true

  validation {
    condition     = length(var.db_password) > 0
    error_message = "A senha do banco não pode ser vazia."
  }
}

variable "jwt_private_key_b64" {
  description = "Chave privada RS256 (PEM PKCS#8) em base64. Precisa ser a mesma da API do ambiente. Fornecida via TF_VAR_jwt_private_key_b64."
  type        = string
  sensitive   = true

  validation {
    condition     = length(var.jwt_private_key_b64) > 0
    error_message = "A chave privada do JWT não pode ser vazia."
  }
}

variable "jwt_public_key_b64" {
  description = "Chave pública RSA (SubjectPublicKeyInfo PEM) em base64, correspondente à chave usada pela API."
  type        = string
  sensitive   = true

  validation {
    condition     = length(var.jwt_public_key_b64) > 0
    error_message = "A chave pública do JWT não pode ser vazia."
  }
}

variable "source_revision" {
  description = "SHA de origem usado no contrato de configuração publicado no S3."
  type        = string
  default     = "unknown"
}

variable "jwt_issuer" {
  description = "Issuer do JWT. Precisa ser igual ao JWT_ISSUER da API, ou ela recusa os tokens."
  type        = string
  default     = "oficina-api"
}

variable "jwt_expiration_minutes" {
  description = "Validade, em minutos, do token emitido ao cliente."
  type        = number
  default     = 60
}

variable "lambda_memory_mb" {
  description = "Memória da função."
  type        = number
  default     = 128
}

variable "lambda_timeout_seconds" {
  description = "Tempo máximo de execução da função."
  type        = number
  default     = 10
}

variable "log_retention_days" {
  description = "Dias de retenção dos logs da função no CloudWatch."
  type        = number
  default     = 7
}



variable "lab_role_name" {
  description = "Role pré-existente do AWS Academy usada pela função (não é possível criar IAM no Learner Lab)."
  type        = string
  default     = "LabRole"
}
