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
