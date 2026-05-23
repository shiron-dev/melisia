data "aws_caller_identity" "current" {}

data "aws_iam_policy_document" "lambda_assume_role" {
  statement {
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["lambda.amazonaws.com"]
    }
  }
}

# kics-scan ignore-line
resource "aws_iam_role" "alexa_ha_lambda" {
  name               = "alexa-ha-lambda-role"
  assume_role_policy = data.aws_iam_policy_document.lambda_assume_role.json
}

# tfsec:ignore:aws-iam-no-policy-wildcards Log stream ARNs require a wildcard suffix under the fixed Lambda log group.
data "aws_iam_policy_document" "alexa_ha_lambda" {
  statement {
    actions = [
      "logs:CreateLogStream",
      "logs:PutLogEvents",
    ]
    resources = ["${aws_cloudwatch_log_group.alexa_ha.arn}:*"]
  }

  statement {
    actions = [
      "kms:Decrypt",
      "kms:Encrypt",
      "kms:GenerateDataKey",
    ]
    resources = [aws_kms_key.alexa_ha.arn]
  }

  statement {
    actions   = ["sqs:SendMessage"]
    resources = [aws_sqs_queue.alexa_ha_dlq.arn]
  }

  statement {
    actions = [
      "xray:PutTelemetryRecords",
      "xray:PutTraceSegments",
    ]
    resources = ["*"]
  }
}

resource "aws_iam_role_policy" "alexa_ha_lambda" {
  name   = "alexa-ha-lambda-policy"
  role   = aws_iam_role.alexa_ha_lambda.id
  policy = data.aws_iam_policy_document.alexa_ha_lambda.json
}
