data "archive_file" "alexa_ha" {
  type        = "zip"
  source_file = "${path.module}/lambda_function.py"
  output_path = "${path.module}/alexa-ha.zip"
}

# kics-scan ignore-line
resource "aws_kms_key" "alexa_ha" {
  description             = "KMS key for the Alexa Home Assistant bridge"
  deletion_window_in_days = 30
  enable_key_rotation     = true
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid    = "EnableAccountAdministration"
        Effect = "Allow"
        Principal = {
          AWS = "arn:aws:iam::${data.aws_caller_identity.current.account_id}:root"
        }
        Action   = "kms:*"
        Resource = "*"
      },
      {
        Sid    = "AllowCloudWatchLogs"
        Effect = "Allow"
        Principal = {
          Service = "logs.${var.aws_region}.amazonaws.com"
        }
        Action = [
          "kms:Decrypt",
          "kms:Encrypt",
          "kms:DescribeKey",
          "kms:GenerateDataKey*",
          "kms:ReEncrypt*",
        ]
        Resource = "*"
        Condition = {
          ArnEquals = {
            "kms:EncryptionContext:aws:logs:arn" = "arn:aws:logs:${var.aws_region}:${data.aws_caller_identity.current.account_id}:log-group:/aws/lambda/alexa-ha-skill"
          }
        }
      },
    ]
  })
}

resource "aws_kms_alias" "alexa_ha" {
  name          = "alias/alexa-ha-bridge"
  target_key_id = aws_kms_key.alexa_ha.key_id
}

resource "aws_cloudwatch_log_group" "alexa_ha" {
  name              = "/aws/lambda/alexa-ha-skill"
  kms_key_id        = aws_kms_key.alexa_ha.arn
  retention_in_days = 365

  tags = {
    Service = "alexa-ha-bridge"
  }
}

# kics-scan ignore-line
resource "aws_sqs_queue" "alexa_ha_dlq" {
  name                              = "alexa-ha-skill-dlq"
  kms_master_key_id                 = aws_kms_key.alexa_ha.arn
  kms_data_key_reuse_period_seconds = 300
}

# kics-scan ignore-line
resource "aws_lambda_function" "alexa_ha" {
  # checkov:skip=CKV_AWS_117:This function calls the public Home Assistant Cloudflare tunnel and does not need VPC attachment.
  # checkov:skip=CKV_AWS_272:Code signing is not used for this small single-file internal bridge.
  function_name                  = "alexa-ha-skill"
  role                           = aws_iam_role.alexa_ha_lambda.arn
  handler                        = "lambda_function.lambda_handler"
  runtime                        = "python3.13"
  filename                       = data.archive_file.alexa_ha.output_path
  source_code_hash               = data.archive_file.alexa_ha.output_base64sha256
  timeout                        = 10
  memory_size                    = 128
  reserved_concurrent_executions = 5
  kms_key_arn                    = aws_kms_key.alexa_ha.arn

  dead_letter_config {
    target_arn = aws_sqs_queue.alexa_ha_dlq.arn
  }

  environment {
    variables = {
      BASE_URL                = var.ha_url
      DEBUG                   = "False"
      LONG_LIVED_ACCESS_TOKEN = var.ha_token
    }
  }

  tracing_config {
    mode = "Active"
  }

  depends_on = [aws_cloudwatch_log_group.alexa_ha]
}

resource "aws_lambda_permission" "alexa" {
  statement_id       = "AllowAlexaSkillsKit"
  action             = "lambda:InvokeFunction"
  function_name      = aws_lambda_function.alexa_ha.function_name
  principal          = "alexa-connectedhome.amazon.com"
  event_source_token = var.alexa_skill_id
}
