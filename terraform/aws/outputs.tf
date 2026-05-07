output "lambda_arn" {
  description = "Lambda ARN to configure in Alexa Developer Console (Smart Home > Default endpoint)"
  value       = aws_lambda_function.alexa_ha.arn
}
