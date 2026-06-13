import math
import os
import re
import subprocess
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


TARGET = os.environ.get("HOME_EP_ICMP_TARGET", "8.8.8.8")
INTERVAL_SECONDS = int(os.environ.get("HOME_EP_ICMP_INTERVAL_SECONDS", "600"))
PACKET_COUNT = int(os.environ.get("HOME_EP_ICMP_PACKET_COUNT", "5"))
PACKET_TIMEOUT_SECONDS = int(os.environ.get("HOME_EP_ICMP_PACKET_TIMEOUT_SECONDS", "2"))
LISTEN_ADDR = os.environ.get("HOME_EP_ICMP_EXPORTER_LISTEN_ADDR", "0.0.0.0")
LISTEN_PORT = int(os.environ.get("HOME_EP_ICMP_EXPORTER_LISTEN_PORT", "9799"))

# 1 サイクルあたり PACKET_COUNT 発打ち、1 発でも応答すれば success=1 とする
# (= 5 回リトライ)。上流の断続的な ICMP ロスで probe が 0/1 にバタつくのを
# 吸収するのが狙い。loss 率や RTT も併せて出して劣化を観測できるようにする。
TRANSMITTED_RE = re.compile(r"(\d+) packets transmitted")
RECEIVED_RE = re.compile(r"(\d+)(?: packets)? received")
LOSS_RE = re.compile(r"([\d.]+)% packet loss")
RTT_RE = re.compile(r"(?:round-trip|rtt) min/avg/max(?:/mdev)? = ([\d.]+)/([\d.]+)/([\d.]+)")

state_lock = threading.Lock()
state = {
    "success": 0,
    "last_run_timestamp_seconds": 0.0,
    "packets_transmitted": 0,
    "packets_received": 0,
    "packet_loss_ratio": math.nan,
    "rtt_min_seconds": math.nan,
    "rtt_avg_seconds": math.nan,
    "rtt_max_seconds": math.nan,
    "error": "",
}


def measure_once():
    # busybox/iputils どちらの ping でも -c (発数) / -W (1 発あたり待ち時間) を解釈する。
    completed = subprocess.run(
        ["ping", "-c", str(PACKET_COUNT), "-W", str(PACKET_TIMEOUT_SECONDS), TARGET],
        capture_output=True,
        text=True,
        # 全発 loss でも -c 発分は待つので、その分に余裕を足したハード上限。
        timeout=PACKET_COUNT * (PACKET_TIMEOUT_SECONDS + 1) + 10,
    )
    output = completed.stdout + completed.stderr

    transmitted_match = TRANSMITTED_RE.search(output)
    received_match = RECEIVED_RE.search(output)
    loss_match = LOSS_RE.search(output)
    rtt_match = RTT_RE.search(output)

    transmitted = int(transmitted_match.group(1)) if transmitted_match else PACKET_COUNT
    received = int(received_match.group(1)) if received_match else 0
    loss_ratio = float(loss_match.group(1)) / 100.0 if loss_match else (1.0 if received == 0 else math.nan)

    result = {
        "success": 1 if received > 0 else 0,
        "last_run_timestamp_seconds": time.time(),
        "packets_transmitted": transmitted,
        "packets_received": received,
        "packet_loss_ratio": loss_ratio,
        "rtt_min_seconds": math.nan,
        "rtt_avg_seconds": math.nan,
        "rtt_max_seconds": math.nan,
        "error": "",
    }

    if rtt_match:
        result["rtt_min_seconds"] = float(rtt_match.group(1)) / 1000.0
        result["rtt_avg_seconds"] = float(rtt_match.group(2)) / 1000.0
        result["rtt_max_seconds"] = float(rtt_match.group(3)) / 1000.0

    return result


def update_state(result):
    with state_lock:
        state.update(result)


def run_loop():
    while True:
        try:
            update_state(measure_once())
        except (OSError, subprocess.SubprocessError, ValueError) as err:
            update_state({
                "success": 0,
                "last_run_timestamp_seconds": time.time(),
                "packets_received": 0,
                "packet_loss_ratio": 1.0,
                "rtt_min_seconds": math.nan,
                "rtt_avg_seconds": math.nan,
                "rtt_max_seconds": math.nan,
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
        "# HELP home_ep_icmp_ping_success Whether at least one ICMP echo reply was received in the latest home-ep ping cycle.",
        "# TYPE home_ep_icmp_ping_success gauge",
        f"home_ep_icmp_ping_success {snapshot['success']}",
        "# HELP home_ep_icmp_ping_last_run_timestamp_seconds Unix timestamp of the latest home-ep ping cycle.",
        "# TYPE home_ep_icmp_ping_last_run_timestamp_seconds gauge",
        f"home_ep_icmp_ping_last_run_timestamp_seconds {prom_value(snapshot['last_run_timestamp_seconds'])}",
        "# HELP home_ep_icmp_ping_packets_transmitted ICMP echo requests sent in the latest home-ep ping cycle.",
        "# TYPE home_ep_icmp_ping_packets_transmitted gauge",
        f"home_ep_icmp_ping_packets_transmitted {snapshot['packets_transmitted']}",
        "# HELP home_ep_icmp_ping_packets_received ICMP echo replies received in the latest home-ep ping cycle.",
        "# TYPE home_ep_icmp_ping_packets_received gauge",
        f"home_ep_icmp_ping_packets_received {snapshot['packets_received']}",
        "# HELP home_ep_icmp_ping_packet_loss_ratio Fraction of ICMP echo requests lost in the latest home-ep ping cycle.",
        "# TYPE home_ep_icmp_ping_packet_loss_ratio gauge",
        f"home_ep_icmp_ping_packet_loss_ratio {prom_value(snapshot['packet_loss_ratio'])}",
        "# HELP home_ep_icmp_ping_rtt_min_seconds Minimum round-trip time in the latest home-ep ping cycle.",
        "# TYPE home_ep_icmp_ping_rtt_min_seconds gauge",
        f"home_ep_icmp_ping_rtt_min_seconds {prom_value(snapshot['rtt_min_seconds'])}",
        "# HELP home_ep_icmp_ping_rtt_avg_seconds Mean round-trip time in the latest home-ep ping cycle.",
        "# TYPE home_ep_icmp_ping_rtt_avg_seconds gauge",
        f"home_ep_icmp_ping_rtt_avg_seconds {prom_value(snapshot['rtt_avg_seconds'])}",
        "# HELP home_ep_icmp_ping_rtt_max_seconds Maximum round-trip time in the latest home-ep ping cycle.",
        "# TYPE home_ep_icmp_ping_rtt_max_seconds gauge",
        f"home_ep_icmp_ping_rtt_max_seconds {prom_value(snapshot['rtt_max_seconds'])}",
        "# HELP home_ep_icmp_ping_interval_seconds Configured interval between home-ep ping cycles.",
        "# TYPE home_ep_icmp_ping_interval_seconds gauge",
        f"home_ep_icmp_ping_interval_seconds {INTERVAL_SECONDS}",
        "# HELP home_ep_icmp_ping_packet_count Configured ICMP echo requests per home-ep ping cycle.",
        "# TYPE home_ep_icmp_ping_packet_count gauge",
        f"home_ep_icmp_ping_packet_count {PACKET_COUNT}",
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
