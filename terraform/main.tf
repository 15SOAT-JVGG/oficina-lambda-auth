locals {
  function_name            = "oficina-${var.environment}-customer-auth"
  authorizer_function_name = "oficina-${var.environment}-jwt-authorizer"
  auth_route               = "POST /v1/auth/customer"

  db  = data.terraform_remote_state.db.outputs
  k8s = data.terraform_remote_state.k8s.outputs
}

# O banco e a rede vêm do state do oficina-infra-db (um por ambiente); o API Gateway, do state do
# oficina-infra-k8s (um único, com um gateway por ambiente). A função não declara rede própria:
# usa a VPC default, a mesma do cluster e do RDS.
data "terraform_remote_state" "db" {
  backend = "s3"
  config = {
    bucket = var.state_bucket
    key    = "oficina-infra-db/${var.environment}/terraform.tfstate"
    region = var.aws_region
  }
}

data "terraform_remote_state" "k8s" {
  backend = "s3"
  config = {
    bucket = var.state_bucket
    key    = "oficina-infra-k8s/terraform.tfstate"
    region = var.aws_region
  }
}

data "aws_iam_role" "lab" {
  name = var.lab_role_name
}

data "aws_caller_identity" "current" {}

# O binário é compilado fora do Terraform (pipeline ou `go build`); aqui só vira o zip que a Lambda
# recebe. O source_code_hash faz uma nova compilação virar um novo deploy.
data "archive_file" "function" {
  type        = "zip"
  source_file = "${path.module}/../build/auth/bootstrap"
  output_path = "${path.module}/../build/function.zip"
}

data "archive_file" "authorizer" {
  type        = "zip"
  source_file = "${path.module}/../build/authorizer/bootstrap"
  output_path = "${path.module}/../build/authorizer.zip"
}

# A função só fala com o Postgres, dentro da VPC. Sem NAT Gateway ela não alcança a internet, e
# não precisa: não chama nenhum serviço externo.
resource "aws_security_group" "function" {
  name = local.function_name
  # A descrição de Security Group no EC2 só aceita ASCII; acento aqui derruba o apply.
  description = "Lambda de autenticacao por CPF (${var.environment})"
  vpc_id      = local.db.vpc_id
}

resource "aws_vpc_security_group_egress_rule" "postgres" {
  security_group_id = aws_security_group.function.id
  description       = "PostgreSQL dentro da VPC"
  cidr_ipv4         = local.db.vpc_cidr_block
  from_port         = 5432
  to_port           = 5432
  ip_protocol       = "tcp"
}

resource "aws_cloudwatch_log_group" "function" {
  name              = "/aws/lambda/${local.function_name}"
  retention_in_days = var.log_retention_days
}

resource "aws_lambda_function" "auth" {
  function_name = local.function_name
  description   = "Autentica o cliente pelo CPF e emite o JWT aceito pela API (${var.environment})"
  role          = data.aws_iam_role.lab.arn

  runtime       = "provided.al2023"
  handler       = "bootstrap"
  architectures = ["x86_64"]
  publish       = true

  filename         = data.archive_file.function.output_path
  source_code_hash = data.archive_file.function.output_base64sha256

  memory_size = var.lambda_memory_mb
  timeout     = var.lambda_timeout_seconds

  vpc_config {
    subnet_ids         = local.db.subnet_ids
    security_group_ids = [aws_security_group.function.id]
  }

  # Segredos em variável de ambiente ficam no state, que está num bucket criptografado e fora do
  # repositório. O Secrets Manager seria o destino natural, mas não há como garantir o acesso a ele
  # no Learner Lab.
  environment {
    variables = {
      DB_HOST                = local.db.db_address
      DB_PORT                = tostring(local.db.db_port)
      DB_NAME                = local.db.db_name
      DB_USERNAME            = local.db.db_username
      DB_PASSWORD            = var.db_password
      DB_SSLMODE             = "require"
      JWT_PRIVATE_KEY_B64    = var.jwt_private_key_b64
      JWT_ISSUER             = var.jwt_issuer
      JWT_EXPIRATION_MINUTES = tostring(var.jwt_expiration_minutes)
    }
  }

  depends_on = [aws_cloudwatch_log_group.function]
}

# A rota é mais específica que o proxy ANY /{proxy+} do cluster, então o gateway a escolhe para
# este caminho e deixa o resto seguir para a aplicação.
resource "aws_apigatewayv2_integration" "auth" {
  api_id                 = local.k8s.api_gateway_ids[var.environment]
  integration_type       = "AWS_PROXY"
  integration_uri        = aws_lambda_function.auth.invoke_arn
  payload_format_version = "2.0"
}

resource "aws_apigatewayv2_route" "auth" {
  api_id    = local.k8s.api_gateway_ids[var.environment]
  route_key = local.auth_route
  target    = "integrations/${aws_apigatewayv2_integration.auth.id}"
}

resource "aws_lambda_permission" "api_gateway" {
  statement_id  = "AllowApiGatewayInvoke"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.auth.function_name
  principal     = "apigateway.amazonaws.com"
  source_arn    = "${local.k8s.api_gateway_execution_arns[var.environment]}/*/*/v1/auth/customer"
}

# O Authorizer só verifica a assinatura localmente. Ele não recebe configuração de banco nem é
# anexado à VPC, então também funciona sem NAT Gateway.
resource "aws_cloudwatch_log_group" "authorizer" {
  name              = "/aws/lambda/${local.authorizer_function_name}"
  retention_in_days = var.log_retention_days
}

resource "aws_lambda_function" "authorizer" {
  function_name = local.authorizer_function_name
  description   = "Valida JWT RS256 antes do encaminhamento pela API (${var.environment})"
  role          = data.aws_iam_role.lab.arn

  runtime       = "provided.al2023"
  handler       = "bootstrap"
  architectures = ["x86_64"]
  publish       = true

  filename         = data.archive_file.authorizer.output_path
  source_code_hash = data.archive_file.authorizer.output_base64sha256

  memory_size = 128
  timeout     = 5

  environment {
    variables = {
      JWT_PUBLIC_KEY_B64 = var.jwt_public_key_b64
      JWT_ISSUER         = var.jwt_issuer
    }
  }

  depends_on = [aws_cloudwatch_log_group.authorizer]
}

resource "aws_lambda_permission" "api_gateway_authorizer" {
  statement_id  = "AllowApiGatewayInvokeAuthorizer"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.authorizer.function_name
  principal     = "apigateway.amazonaws.com"
  source_arn    = "${local.k8s.api_gateway_execution_arns[var.environment]}/authorizers/*"
}

# O contrato publicado não contém segredos. A configuração de upstream é publicada pelo workflow
# da aplicação; este objeto publica apenas a identidade/revisão do Authorizer.
resource "aws_s3_object" "authorizer_contract" {
  bucket       = var.state_bucket
  key          = "oficina/config/v1/${var.environment}/authorizer.json"
  content_type = "application/json"
  content = jsonencode({
    contract_version = 1
    account_id       = data.aws_caller_identity.current.account_id
    region           = var.aws_region
    environment      = var.environment
    cluster_name     = local.k8s.cluster_name
    lambda_arn       = aws_lambda_function.authorizer.qualified_arn
    source_sha       = var.source_revision
  })
}
