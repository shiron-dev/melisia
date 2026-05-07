data "aws_iam_policy_document" "lambda_assume_role" {
  statement {
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["lambda.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "alexa_ha_lambda" {
  name               = "alexa-ha-lambda-role"
  assume_role_policy = data.aws_iam_policy_document.lambda_assume_role.json
}

data "aws_iam_policy_document" "alexa_ha_lambda" {
  statement {
    actions = [
      "logs:CreateLogGroup",
      "logs:CreateLogStream",
      "logs:PutLogEvents",
    ]
    resources = ["arn:aws:logs:*:*:*"]
  }
}

resource "aws_iam_role_policy" "alexa_ha_lambda" {
  name   = "alexa-ha-lambda-policy"
  role   = aws_iam_role.alexa_ha_lambda.id
  policy = data.aws_iam_policy_document.alexa_ha_lambda.json
}
