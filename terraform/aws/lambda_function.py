"""
Copyright 2019 Jason Hu <awaregit at gmail.com>
Modified 2020 Matthew Hilton <matthilton2005@gmail.com>
Refactor and Modernised 2025 Matthew Hilton <matthilton2005@gmail.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
"""

import json
import logging
import os
import ssl
from typing import Any
from urllib import error, request


def _env_flag(name: str) -> bool:
    return os.environ.get(name, "").lower() in ("1", "true", "yes", "on")


def _redact_tokens(value: Any) -> Any:
    if isinstance(value, dict):
        return {
            key: "***REDACTED***" if key.lower() == "token" else _redact_tokens(item)
            for key, item in value.items()
        }

    if isinstance(value, list):
        return [_redact_tokens(item) for item in value]

    return value


_debug = _env_flag("DEBUG")

# Configure logging with enhanced formatting
_log_format = (
    "%(asctime)s - %(name)s - %(levelname)s - [%(filename)s:%(lineno)d] - %(message)s"
)
_formatter = logging.Formatter(_log_format)

# Console handler
_console_handler = logging.StreamHandler()
_console_handler.setFormatter(_formatter)

# Logger setup
_logger = logging.getLogger("HomeAssistant-SmartHome")
_logger.setLevel(logging.DEBUG if _debug else logging.INFO)
_logger.addHandler(_console_handler)


def lambda_handler(event: dict[str, Any], context: Any) -> dict[str, Any]:
    """Handle incoming Alexa directive.

    Args:
        event: The Alexa directive event payload
        context: AWS Lambda context object

    Returns:
        Response payload for Alexa
    """

    _logger.info("Processing Alexa request")
    _logger.debug("Event payload: %s", json.dumps(_redact_tokens(event), indent=2))

    try:
        base_url = os.environ.get("BASE_URL")
        if base_url is None:
            _logger.error("BASE_URL environment variable not set")
            raise ValueError("BASE_URL environment variable must be set")
        base_url = base_url.rstrip("/")
        _logger.debug("Base URL: %s", base_url)

        directive = event.get("directive")
        if directive is None:
            _logger.error("Malformed request: missing directive")
            raise ValueError("Request missing required directive field")

        payload_version = directive.get("header", {}).get("payloadVersion")
        if payload_version != "3":
            _logger.error("Unsupported payloadVersion: %s", payload_version)
            raise ValueError(f"Only payloadVersion 3 is supported, got {payload_version}")

        scope = directive.get("endpoint", {}).get("scope")
        if scope is None:
            # token is in grantee for Linking directive
            scope = directive.get("payload", {}).get("grantee")
        if scope is None:
            # token is in payload for Discovery directive
            scope = directive.get("payload", {}).get("scope")

        if scope is None:
            _logger.error("Malformed request: missing scope/token")
            raise ValueError("Request missing scope in endpoint or payload")

        scope_type = scope.get("type")
        if scope_type != "BearerToken":
            _logger.error("Unsupported scope type: %s", scope_type)
            raise ValueError(f"Only BearerToken scope is supported, got {scope_type}")

        if scope.get("token") is None:
            _logger.error("Malformed request: missing bearer token")
            raise ValueError("Request missing bearer token")

        ha_token = os.environ.get("LONG_LIVED_ACCESS_TOKEN")
        if ha_token is None:
            _logger.error("LONG_LIVED_ACCESS_TOKEN environment variable not set")
            raise ValueError("Authentication token is required")

        verify_ssl = not _env_flag("NOT_VERIFY_SSL")
        _logger.debug("SSL verification enabled: %s", verify_ssl)

        ssl_context = None if verify_ssl else ssl._create_unverified_context()
        request_body = json.dumps(event).encode("utf-8")
        ha_request = request.Request(
            f"{base_url}/api/alexa/smart_home",
            data=request_body,
            headers={
                "Authorization": f"Bearer {ha_token}",
                "Content-Type": "application/json",
            },
            method="POST",
        )

        _logger.info("Sending request to Home Assistant")
        try:
            with request.urlopen(
                ha_request, context=ssl_context, timeout=10
            ) as response:
                response_status = response.status
                response_text = response.read().decode("utf-8")
        except error.HTTPError as exc:
            response_status = exc.code
            response_text = exc.read().decode("utf-8")

        _logger.debug("Response status: %s", response_status)

        if response_status >= 400:
            _logger.error(
                "Home Assistant returned error %s: %s", response_status, response_text
            )

            error_type = (
                "INVALID_AUTHORIZATION_CREDENTIAL"
                if response_status in (401, 403)
                else "INTERNAL_ERROR"
            )
            return {
                "event": {
                    "payload": {
                        "type": error_type,
                        "message": response_text,
                    }
                }
            }

        response_data = json.loads(response_text)
        _logger.info("Successfully processed Alexa request")
        _logger.debug("Response: %s", json.dumps(response_data, indent=2))
        return response_data

    except (ValueError, KeyError, json.JSONDecodeError) as exc:
        _logger.exception("Error processing request: %s", str(exc))
        return {
            "event": {
                "payload": {
                    "type": "INVALID_REQUEST",
                    "message": str(exc),
                }
            }
        }
    except Exception as exc:
        _logger.exception("Unexpected error: %s", str(exc))
        return {
            "event": {
                "payload": {
                    "type": "INTERNAL_ERROR",
                    "message": "An unexpected error occurred",
                }
            }
        }
