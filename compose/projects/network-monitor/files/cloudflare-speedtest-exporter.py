import json
import math
import os
import random
import statistics
import threading
import time
import urllib.error
import urllib.parse
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


INTERVAL_SECONDS = int(os.environ.get("CLOUDFLARE_SPEEDTEST_INTERVAL_SECONDS", "600"))
DOWNLOAD_BYTES = int(os.environ.get("CLOUDFLARE_SPEEDTEST_DOWNLOAD_BYTES", "25000000"))
UPLOAD_BYTES = int(os.environ.get("CLOUDFLARE_SPEEDTEST_UPLOAD_BYTES", "10000000"))
TARGET_BASE_URL = os.environ.get("CLOUDFLARE_SPEEDTEST_TARGET_BASE_URL", "https://speed.cloudflare.com").rstrip("/")
HTTP_TIMEOUT_SECONDS = int(os.environ.get("CLOUDFLARE_SPEEDTEST_HTTP_TIMEOUT_SECONDS", "90"))
LISTEN_ADDR = os.environ.get("CLOUDFLARE_SPEEDTEST_EXPORTER_LISTEN_ADDR", "0.0.0.0")
LISTEN_PORT = int(os.environ.get("CLOUDFLARE_SPEEDTEST_EXPORTER_LISTEN_PORT", "9798"))

state_lock = threading.Lock()
state = {
    "success": 0,
    "last_run_timestamp_seconds": 0.0,
    "download_bits_per_second": math.nan,
    "upload_bits_per_second": math.nan,
    "latency_seconds": math.nan,
    "jitter_seconds": math.nan,
    "download_bytes": 0,
    "upload_bytes": 0,
    "error": "",
}


def timed_request(url, data=None, read_response=True):
    started = time.monotonic()
    request = urllib.request.Request(url, data=data, headers={"User-Agent": "home-ep-cloudflare-speedtest-exporter/1.0"})
    with urllib.request.urlopen(request, timeout=HTTP_TIMEOUT_SECONDS) as response:
        first_byte_at = time.monotonic()
        total = 0
        if read_response:
            while True:
                chunk = response.read(1024 * 256)
                if not chunk:
                    break
                total += len(chunk)
        else:
            response.read()
            total = len(data or b"")
    finished = time.monotonic()

    return {
        "bytes": total,
        "duration_seconds": max(finished - started, 0.000001),
        "ttfb_seconds": max(first_byte_at - started, 0.0),
    }


def measure_latency():
    samples = []
    query = urllib.parse.urlencode({"_": str(random.random())})
    for _ in range(5):
        result = timed_request(f"{TARGET_BASE_URL}/__down?bytes=1&{query}")
        samples.append(result["ttfb_seconds"])

    latency = statistics.median(samples)
    jitter = statistics.mean(abs(sample - latency) for sample in samples)

    return latency, jitter


def measure_once():
    latency, jitter = measure_latency()

    query = urllib.parse.urlencode({"bytes": DOWNLOAD_BYTES, "_": str(random.random())})
    download = timed_request(f"{TARGET_BASE_URL}/__down?{query}")

    upload_payload = b"0" * UPLOAD_BYTES
    upload = timed_request(f"{TARGET_BASE_URL}/__up", data=upload_payload, read_response=False)

    return {
        "success": 1,
        "last_run_timestamp_seconds": time.time(),
        "download_bits_per_second": download["bytes"] * 8 / download["duration_seconds"],
        "upload_bits_per_second": upload["bytes"] * 8 / upload["duration_seconds"],
        "latency_seconds": latency,
        "jitter_seconds": jitter,
        "download_bytes": download["bytes"],
        "upload_bytes": upload["bytes"],
        "error": "",
    }


def update_state(result):
    with state_lock:
        state.update(result)


def run_loop():
    while True:
        try:
            update_state(measure_once())
        except (OSError, urllib.error.URLError, urllib.error.HTTPError, TimeoutError, json.JSONDecodeError) as err:
            update_state({
                "success": 0,
                "last_run_timestamp_seconds": time.time(),
                "error": str(err).replace("\\", "\\\\").replace('"', '\\"'),
            })

        time.sleep(INTERVAL_SECONDS)


def prom_value(value):
    if isinstance(value, float) and math.isnan(value):
        return "NaN"

    return str(value)


def metrics_body():
    with state_lock:
        snapshot = dict(state)

    lines = [
        "# HELP home_ep_cloudflare_speedtest_success Whether the latest home-ep Cloudflare Speed Test completed successfully.",
        "# TYPE home_ep_cloudflare_speedtest_success gauge",
        f"home_ep_cloudflare_speedtest_success {snapshot['success']}",
        "# HELP home_ep_cloudflare_speedtest_last_run_timestamp_seconds Unix timestamp of the latest home-ep Cloudflare Speed Test run.",
        "# TYPE home_ep_cloudflare_speedtest_last_run_timestamp_seconds gauge",
        f"home_ep_cloudflare_speedtest_last_run_timestamp_seconds {prom_value(snapshot['last_run_timestamp_seconds'])}",
        "# HELP home_ep_cloudflare_speedtest_download_bits_per_second Latest measured Cloudflare Speed Test download throughput.",
        "# TYPE home_ep_cloudflare_speedtest_download_bits_per_second gauge",
        f"home_ep_cloudflare_speedtest_download_bits_per_second {prom_value(snapshot['download_bits_per_second'])}",
        "# HELP home_ep_cloudflare_speedtest_upload_bits_per_second Latest measured Cloudflare Speed Test upload throughput.",
        "# TYPE home_ep_cloudflare_speedtest_upload_bits_per_second gauge",
        f"home_ep_cloudflare_speedtest_upload_bits_per_second {prom_value(snapshot['upload_bits_per_second'])}",
        "# HELP home_ep_cloudflare_speedtest_latency_seconds Median time to first byte across Cloudflare Speed Test latency probes.",
        "# TYPE home_ep_cloudflare_speedtest_latency_seconds gauge",
        f"home_ep_cloudflare_speedtest_latency_seconds {prom_value(snapshot['latency_seconds'])}",
        "# HELP home_ep_cloudflare_speedtest_jitter_seconds Mean absolute deviation from median Cloudflare Speed Test latency.",
        "# TYPE home_ep_cloudflare_speedtest_jitter_seconds gauge",
        f"home_ep_cloudflare_speedtest_jitter_seconds {prom_value(snapshot['jitter_seconds'])}",
        "# HELP home_ep_cloudflare_speedtest_download_bytes Bytes read during the latest Cloudflare Speed Test download test.",
        "# TYPE home_ep_cloudflare_speedtest_download_bytes gauge",
        f"home_ep_cloudflare_speedtest_download_bytes {snapshot['download_bytes']}",
        "# HELP home_ep_cloudflare_speedtest_upload_bytes Bytes written during the latest Cloudflare Speed Test upload test.",
        "# TYPE home_ep_cloudflare_speedtest_upload_bytes gauge",
        f"home_ep_cloudflare_speedtest_upload_bytes {snapshot['upload_bytes']}",
        "# HELP home_ep_cloudflare_speedtest_interval_seconds Configured Cloudflare Speed Test interval.",
        "# TYPE home_ep_cloudflare_speedtest_interval_seconds gauge",
        f"home_ep_cloudflare_speedtest_interval_seconds {INTERVAL_SECONDS}",
    ]

    return "\n".join(lines) + "\n"


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == "/healthz":
            self.send_response(200)
            self.end_headers()
            self.wfile.write(b"ok\n")

            return

        if self.path != "/metrics":
            self.send_response(404)
            self.end_headers()

            return

        body = metrics_body().encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, fmt, *args):
        return


if __name__ == "__main__":
    worker = threading.Thread(target=run_loop, daemon=True)
    worker.start()
    ThreadingHTTPServer((LISTEN_ADDR, LISTEN_PORT), Handler).serve_forever()
