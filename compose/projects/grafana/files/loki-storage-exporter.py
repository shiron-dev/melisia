#!/usr/bin/env python3
from __future__ import annotations

import os
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


LISTEN_ADDR = "0.0.0.0"
LISTEN_PORT = 9491
STORAGE_ROOT = os.environ.get("LOKI_STORAGE_ROOT", "/loki")


def label_value(value: object) -> str:
    text = "" if value is None else str(value)
    return text.replace("\\", "\\\\").replace("\n", "\\n").replace('"', '\\"')


def labels(items: dict[str, object]) -> str:
    return ",".join(f'{key}="{label_value(value)}"' for key, value in items.items())


def metric(name: str, labels_map: dict[str, object], value: object) -> str:
    return name + "{" + labels(labels_map) + "} " + str(value)


def directory_size(path: str) -> tuple[int, int, int]:
    total_bytes = 0
    file_count = 0
    error_count = 0
    stack = [path]

    while stack:
        current = stack.pop()
        try:
            with os.scandir(current) as entries:
                for entry in entries:
                    try:
                        if entry.is_dir(follow_symlinks=False):
                            stack.append(entry.path)
                        elif entry.is_file(follow_symlinks=False):
                            total_bytes += entry.stat(follow_symlinks=False).st_size
                            file_count += 1
                    except OSError:
                        error_count += 1
        except OSError:
            error_count += 1

    return total_bytes, file_count, error_count


def child_directories(path: str) -> list[str]:
    try:
        with os.scandir(path) as entries:
            return sorted(entry.path for entry in entries if entry.is_dir(follow_symlinks=False))
    except OSError:
        return []


def collect_metrics() -> str:
    now = int(time.time())
    root = os.path.abspath(STORAGE_ROOT)
    total_bytes, total_files, total_errors = directory_size(root)

    lines = [
        "# HELP loki_storage_exporter_up Whether the Loki storage exporter can read the storage root.",
        "# TYPE loki_storage_exporter_up gauge",
        "loki_storage_exporter_up 1",
        "# HELP loki_storage_bytes Recursive filesystem bytes used by Loki storage paths.",
        "# TYPE loki_storage_bytes gauge",
        "# HELP loki_storage_files Recursive file count under Loki storage paths.",
        "# TYPE loki_storage_files gauge",
        "# HELP loki_storage_scan_errors Filesystem scan errors while reading Loki storage paths.",
        "# TYPE loki_storage_scan_errors gauge",
        "# HELP loki_storage_exporter_scrape_timestamp_seconds Last successful scrape timestamp.",
        "# TYPE loki_storage_exporter_scrape_timestamp_seconds gauge",
        metric("loki_storage_bytes", {"path": root, "name": "total"}, total_bytes),
        metric("loki_storage_files", {"path": root, "name": "total"}, total_files),
        metric("loki_storage_scan_errors", {"path": root, "name": "total"}, total_errors),
    ]

    for child in child_directories(root):
        child_bytes, child_files, child_errors = directory_size(child)
        lines.append(metric("loki_storage_bytes", {"path": child, "name": os.path.basename(child)}, child_bytes))
        lines.append(metric("loki_storage_files", {"path": child, "name": os.path.basename(child)}, child_files))
        lines.append(metric("loki_storage_scan_errors", {"path": child, "name": os.path.basename(child)}, child_errors))

    lines.append(f"loki_storage_exporter_scrape_timestamp_seconds {now}")
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
                "# HELP loki_storage_exporter_up Whether the Loki storage exporter can read the storage root.\n"
                "# TYPE loki_storage_exporter_up gauge\n"
                "loki_storage_exporter_up 0\n"
                'loki_storage_exporter_error{message="' + label_value(exc) + '"} 1\n'
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
