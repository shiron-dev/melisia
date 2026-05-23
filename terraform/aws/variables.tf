variable "aws_region" {
  description = "AWS region for Lambda"
  type        = string
  default     = "us-west-2"
}

variable "ha_url" {
  description = "Home Assistant URL via Cloudflare tunnel (e.g. https://home.melisia.net)"
  type        = string
  sensitive   = true
}

variable "ha_token" {
  description = "Home Assistant Long-lived Access Token"
  type        = string
  sensitive   = true
}

variable "alexa_skill_id" {
  description = "Alexa Skill ID (amzn1.ask.skill.xxxxx)"
  type        = string
}
