#!/usr/bin/env python3
from __future__ import annotations

import http.client
import json
import os
import socket
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


DOCKER_SOCKET = "/var/run/docker.sock"
DOCKER_CONTAINERS_ROOT = "/docker-containers"
LISTEN_ADDR = "0.0.0.0"
LISTEN_PORT = 9487


class UnixHTTPConnection(http.client.HTTPConnection):
    def __init__(self, socket_path: str) -> None:
        super().__init__("localhost")
        self.socket_path = socket_path

    def connect(self) -> None:
        sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        sock.connect(self.socket_path)
        self.sock = sock


def docker_get(path: str) -> object:
    conn = UnixHTTPConnection(DOCKER_SOCKET)
    try:
        conn.request("GET", path)
        res = conn.getresponse()
        body = res.read()
        if res.status >= 400:
            raise RuntimeError(f"Docker API {path} returned {res.status}: {body.decode('utf-8', 'replace')}")
        return json.loads(body.decode("utf-8"))
    finally:
        conn.close()


def label_value(value: object) -> str:
    text = "" if value is None else str(value)
    return text.replace("\\", "\\\\").replace("\n", "\\n").replace('"', '\\"')


def labels(items: dict[str, object]) -> str:
    return ",".join(f'{key}="{label_value(value)}"' for key, value in items.items())


def metric(name: str, labels_map: dict[str, object], value: object) -> str:
    return name + "{" + labels(labels_map) + "} " + str(value)


def remapped_log_path(log_path: object) -> str | None:
    if not isinstance(log_path, str) or not log_path:
        return None

    prefix = "/var/lib/docker/containers/"
    if not log_path.startswith(prefix):
        return None

    return os.path.join(DOCKER_CONTAINERS_ROOT, log_path.removeprefix(prefix))


def file_size(path: str | None) -> int:
    if path is None:
        return 0

    try:
        return os.stat(path).st_size
    except OSError:
        return 0


def collect_metrics() -> str:
    now = int(time.time())
    lines = [
        "# HELP docker_compose_exporter_up Whether the Docker Compose exporter can read the Docker API.",
        "# TYPE docker_compose_exporter_up gauge",
        "docker_compose_exporter_up 1",
        "# HELP docker_compose_container_state Container state for Docker Compose containers. The active state label has value 1.",
        "# TYPE docker_compose_container_state gauge",
        "# HELP docker_compose_container_health Container health status for Docker Compose containers. The active health label has value 1.",
        "# TYPE docker_compose_container_health gauge",
        "# HELP docker_compose_container_restart_count Docker restart count for Docker Compose containers.",
        "# TYPE docker_compose_container_restart_count gauge",
        "# HELP docker_compose_container_created_timestamp_seconds Docker container creation timestamp.",
        "# TYPE docker_compose_container_created_timestamp_seconds gauge",
        "# HELP docker_compose_container_log_bytes Docker JSON log file size for Docker Compose containers.",
        "# TYPE docker_compose_container_log_bytes gauge",
        "# HELP docker_compose_exporter_scrape_timestamp_seconds Last successful scrape timestamp.",
        "# TYPE docker_compose_exporter_scrape_timestamp_seconds gauge",
    ]

    containers = docker_get("/containers/json?all=1")
    if not isinstance(containers, list):
        raise RuntimeError("Docker API returned an unexpected container list payload")

    for container in containers:
        if not isinstance(container, dict):
            continue

        container_id = str(container.get("Id", ""))
        inspect = docker_get(f"/containers/{container_id}/json")
        if not isinstance(inspect, dict):
            continue

        config = inspect.get("Config") if isinstance(inspect.get("Config"), dict) else {}
        docker_labels = config.get("Labels") if isinstance(config.get("Labels"), dict) else {}
        project = docker_labels.get("com.docker.compose.project")
        service = docker_labels.get("com.docker.compose.service")
        if not project or not service:
            continue

        state_obj = inspect.get("State") if isinstance(inspect.get("State"), dict) else {}
        state = str(state_obj.get("Status") or container.get("State") or "unknown")
        health_obj = state_obj.get("Health") if isinstance(state_obj.get("Health"), dict) else None
        health = str(health_obj.get("Status")) if health_obj else "none"
        names = container.get("Names") if isinstance(container.get("Names"), list) else []
        container_name = str(names[0]).lstrip("/") if names else str(inspect.get("Name", "")).lstrip("/")

        base_labels = {
            "project": project,
            "service": service,
            "container": container_name,
            "image": container.get("Image", ""),
        }

        lines.append(metric("docker_compose_container_state", {**base_labels, "state": state}, 1))
        lines.append(metric("docker_compose_container_health", {**base_labels, "health": health}, 1))
        lines.append(metric("docker_compose_container_restart_count", base_labels, inspect.get("RestartCount", 0)))
        lines.append(metric("docker_compose_container_created_timestamp_seconds", base_labels, container.get("Created", 0)))
        lines.append(metric("docker_compose_container_log_bytes", base_labels, file_size(remapped_log_path(inspect.get("LogPath")))))

    lines.append(f"docker_compose_exporter_scrape_timestamp_seconds {now}")
    return "\n".join(lines) + "\n"


class Handler(BaseHTTPRequestHandler):
    def do_GET(self) -> None:
        if self.path == "/healthz":
            self.send_response(200)
            self.end_headers()
            self.wfile.write(b"ok\n")
            return

        if self.path != "/metrics":
            self.send_response(404)
            self.end_headers()
            return

        try:
            body = collect_metrics().encode("utf-8")
            status = 200
        except Exception as exc:
            body = (
                "# HELP docker_compose_exporter_up Whether the Docker Compose exporter can read the Docker API.\n"
                "# TYPE docker_compose_exporter_up gauge\n"
                "docker_compose_exporter_up 0\n"
                'docker_compose_exporter_error{message="' + label_value(exc) + '"} 1\n'
            ).encode("utf-8")
            status = 500

        self.send_response(status)
        self.send_header("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, format: str, *args: object) -> None:
        return


if __name__ == "__main__":
    server = ThreadingHTTPServer((LISTEN_ADDR, LISTEN_PORT), Handler)
    server.serve_forever()
