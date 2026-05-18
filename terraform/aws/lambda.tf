data "archive_file" "alexa_ha" {
  type        = "zip"
  source_file = "${path.module}/lambda_function.py"
  output_path = "${path.module}/alexa-ha.zip"
}

resource "aws_cloudwatch_log_group" "alexa_ha" {
  name              = "/aws/lambda/alexa-ha-skill"
  retention_in_days = 14
}

resource "aws_lambda_function" "alexa_ha" {
  function_name    = "alexa-ha-skill"
  role             = aws_iam_role.alexa_ha_lambda.arn
  handler          = "lambda_function.lambda_handler"
  runtime          = "python3.13"
  filename         = data.archive_file.alexa_ha.output_path
  source_code_hash = data.archive_file.alexa_ha.output_base64sha256
  timeout          = 10
  memory_size      = 128

  environment {
    variables = {
      BASE_URL                = var.ha_url
      DEBUG                   = "False"
      LONG_LIVED_ACCESS_TOKEN = var.ha_token
    }
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
