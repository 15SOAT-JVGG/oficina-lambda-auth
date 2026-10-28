output "function_name" {
  description = "Nome da função Lambda."
  value       = aws_lambda_function.auth.function_name
}

output "function_arn" {
  description = "ARN da função Lambda."
  value       = aws_lambda_function.auth.arn
}

output "auth_url" {
  description = "URL pública da autenticação por CPF, pelo API Gateway do ambiente."
  value       = "${local.k8s.api_gateway_urls[var.environment]}/v1/auth/customer"
}

output "authorizer_function_name" {
  description = "Nome da função Lambda authorizer."
  value       = aws_lambda_function.authorizer.function_name
}

output "authorizer_function_arn" {
  description = "ARN versionado da função Lambda authorizer para integração com API Gateway."
  value       = aws_lambda_function.authorizer.qualified_arn
}

output "authorizer_contract_key" {
  description = "Chave versionada do contrato público do authorizer no bucket de configuração."
  value       = aws_s3_object.authorizer_contract.key
}
