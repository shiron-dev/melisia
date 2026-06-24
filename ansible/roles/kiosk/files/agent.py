#!/usr/bin/env python3
"""home-kiosk agent.

Bridges a Raspberry Pi kiosk to Home Assistant over MQTT (discovery):
  * screen power  -> vcgencmd display_power (HA "screen" switch)
  * URL rotation  -> Chromium via the DevTools protocol (HA "rotation" switch +
                     per-slot URL/seconds entities, editable from HA)
  * input activity-> evdev watcher publishes a motion binary_sensor (HA decides
                     what to do; the device never powers the screen by itself)

Config is read from $KIOSK_CONFIG (default /etc/kiosk/config.yaml). Per-slot
playlist state is persisted to the configured state file so HA edits survive
restarts.
"""

import glob
import json
import os
import select
import subprocess
import threading
import time
from datetime import datetime, timezone

import yaml
import requests
import websocket  # websocket-client
import evdev
import paho.mqtt.client as mqtt

CONFIG_PATH = os.environ.get("KIOSK_CONFIG", "/etc/kiosk/config.yaml")


def log(*a):
    print(*a, flush=True)


# --------------------------------------------------------------------------- #
# Config + persisted state
# --------------------------------------------------------------------------- #
with open(CONFIG_PATH) as f:
    CFG = yaml.safe_load(f)

DEVICE_ID = CFG["device_id"]
PREFIX = CFG.get("discovery_prefix", "homeassistant")
CDP_PORT = int(CFG.get("cdp_port", 9222))
IDLE_TIMEOUT = int(CFG.get("idle_timeout", 30))
NSLOTS = int(CFG.get("slots", 6))
# Upper bound for clearing stale slot entities from HA when `slots` is reduced,
# so the number of URL candidates can be changed freely across deploys.
SLOT_CLEANUP_CEILING = max(NSLOTS, int(CFG.get("slots_cleanup_ceiling", 64)))
STATE_PATH = CFG["state_path"]
BASE = f"kiosk/{DEVICE_ID}"
AVAIL_TOPIC = f"{BASE}/availability"

_state_lock = threading.Lock()


def _default_state():
    slots = []
    for item in (CFG.get("default_playlist") or [])[:NSLOTS]:
        slots.append({"url": str(item.get("url", "")),
                      "seconds": int(item.get("seconds", 30))})
    while len(slots) < NSLOTS:
        slots.append({"url": "", "seconds": 30})
    return {"slots": slots, "rotation": True}


def load_state():
    try:
        with open(STATE_PATH) as fh:
            st = json.load(fh)
        slots = st.get("slots", [])
        # normalise to exactly NSLOTS
        slots = (slots + _default_state()["slots"])[:NSLOTS]
        for s in slots:
            s.setdefault("url", "")
            s["seconds"] = int(s.get("seconds", 30))
        return {"slots": slots, "rotation": bool(st.get("rotation", True))}
    except (OSError, ValueError):
        st = _default_state()
        save_state(st)
        return st


def save_state(st):
    tmp = STATE_PATH + ".tmp"
    with open(tmp, "w") as fh:
        json.dump(st, fh, indent=2)
    os.replace(tmp, STATE_PATH)


STATE = load_state()

# runtime (non-persisted)
screen_on = True
fullscreen_on = True      # Chromium window fullscreen vs normal (HA-controllable)
current_slot = 0          # index into the *active* (non-empty) slot list
activity_on = False
last_event = 0.0
wake = threading.Event()  # interrupts the rotation dwell


# --------------------------------------------------------------------------- #
# Screen power (wlopm — Wayland output power management / DPMS)
#
# Under labwc/KMS the Wayland compositor owns the output, so the legacy
# `vcgencmd display_power` firmware path is ineffective. wlopm toggles DPMS on
# the output (without reconfiguring it, so the compositor doesn't revert it).
# The agent runs as a systemd --user service, so locate the compositor's
# Wayland socket under XDG_RUNTIME_DIR.
# --------------------------------------------------------------------------- #
OUTPUT = CFG.get("output", "HDMI-A-1")


def _wayland_env():
    env = dict(os.environ)
    xdg = env.get("XDG_RUNTIME_DIR") or f"/run/user/{os.getuid()}"
    env["XDG_RUNTIME_DIR"] = xdg
    if not env.get("WAYLAND_DISPLAY"):
        socks = sorted(p for p in glob.glob(os.path.join(xdg, "wayland-*"))
                       if not p.endswith(".lock"))
        if socks:
            env["WAYLAND_DISPLAY"] = os.path.basename(socks[0])
    return env


def screen_set(on):
    global screen_on
    subprocess.run(["wlopm", "--on" if on else "--off", OUTPUT],
                   check=False, capture_output=True, env=_wayland_env())
    screen_on = on
    if not on:
        wake.set()  # let the rotation loop notice and pause


def screen_read():
    try:
        out = subprocess.run(["wlopm"], capture_output=True, text=True,
                             timeout=5, env=_wayland_env()).stdout
        for line in out.splitlines():
            parts = line.split()
            if len(parts) >= 2 and parts[0] == OUTPUT:
                return parts[1].strip().lower() == "on"
        return True
    except Exception:
        return True


# --------------------------------------------------------------------------- #
# Chromium DevTools — pre-loaded tabs
#
# Instead of re-navigating one tab on every rotation step (which shows a load
# delay on screen), keep one tab per URL open and pre-loaded. Rotation just
# switches the visible tab (Target.activateTarget — instant). Each tab is
# reloaded while it is off-screen so the displayed page is always already
# loaded/fresh.
# --------------------------------------------------------------------------- #
_CDP = f"http://127.0.0.1:{CDP_PORT}"
tab_urls = []   # URLs currently mapped to tabs, in rotation order
tab_ids = []    # Chromium targetIds aligned with tab_urls


def _browser_cmd(method, params=None):
    ws_url = requests.get(f"{_CDP}/json/version", timeout=5).json()["webSocketDebuggerUrl"]
    ws = websocket.create_connection(ws_url, timeout=10, origin=_CDP)
    try:
        ws.send(json.dumps({"id": 1, "method": method, "params": params or {}}))
        while True:
            r = json.loads(ws.recv())
            if r.get("id") == 1:
                return r.get("result", {})
    finally:
        ws.close()


def cdp_pages():
    return [t for t in requests.get(f"{_CDP}/json", timeout=5).json() if t.get("type") == "page"]


def cdp_activate(target_id):
    try:
        requests.get(f"{_CDP}/json/activate/{target_id}", timeout=5)
    except Exception as e:
        log("cdp_activate failed:", e)


def cdp_reload(target_id):
    """Reload the given tab in the background (it should be off-screen)."""
    try:
        page = next((p for p in cdp_pages() if p["id"] == target_id), None)
        if not page or not page.get("webSocketDebuggerUrl"):
            return
        ws = websocket.create_connection(page["webSocketDebuggerUrl"], timeout=10, origin=_CDP)
        try:
            ws.send(json.dumps({"id": 1, "method": "Page.reload", "params": {}}))
            ws.recv()
        finally:
            ws.close()
    except Exception as e:
        log("cdp_reload failed:", e)


def sync_tabs(urls):
    """Ensure exactly one pre-loaded tab per URL (in order). Rebuild only when
    the URL list changes (or a tab disappeared)."""
    global tab_urls, tab_ids
    try:
        existing = {p["id"] for p in cdp_pages()}
        if urls == tab_urls and tab_ids and all(t in existing for t in tab_ids):
            return
        # Create the new tabs first (so the window never has zero tabs), then
        # close every other page target (old tabs + the initial about:blank).
        new_ids = []
        for u in urls:
            new_ids.append(_browser_cmd("Target.createTarget", {"url": u})["targetId"])
        keep = set(new_ids)
        for p in cdp_pages():
            if p["id"] not in keep:
                _browser_cmd("Target.closeTarget", {"targetId": p["id"]})
        tab_urls = list(urls)
        tab_ids = new_ids
        log("tabs synced:", urls)
    except Exception as e:
        log("sync_tabs failed:", e)


_uinput = None
_uinput_lock = threading.Lock()


def _vkbd():
    """A persistent virtual keyboard (uinput) for sending F11 to Chromium.

    CDP and the Wayland virtual-keyboard protocol (wtype) don't reliably toggle
    Chromium's fullscreen on this labwc setup; a kernel-level uinput key behaves
    like a real keyboard and works. Requires /dev/uinput access (the kiosk role
    grants the `input` group via a udev rule).
    """
    global _uinput
    if _uinput is None:
        _uinput = evdev.UInput({evdev.ecodes.EV_KEY: [evdev.ecodes.KEY_F11,
                                                      evdev.ecodes.KEY_ESC]},
                               name="kiosk-virtual-kbd")
        time.sleep(1)  # let libinput/labwc pick up the new keyboard
    return _uinput


def _tap(keycode):
    with _uinput_lock:
        ui = _vkbd()
        ui.write(evdev.ecodes.EV_KEY, keycode, 1)
        ui.syn()
        time.sleep(0.05)
        ui.write(evdev.ecodes.EV_KEY, keycode, 0)
        ui.syn()


def set_fullscreen(on):
    """Toggle Chromium fullscreen by sending F11, only if state differs.

    NOTE: un-fullscreening yields a movable window whose page content is
    touch/click-operable; the browser toolbar/tabs do not render under this
    Pi's software (SwiftShader) rendering.
    """
    global fullscreen_on
    if on == fullscreen_on:
        return
    try:
        _tap(evdev.ecodes.KEY_F11)
        fullscreen_on = on
    except Exception as e:
        log("set_fullscreen failed:", e)


# --------------------------------------------------------------------------- #
# Browser auto-(re)launch control
#
# The labwc autostart relaunches Chromium whenever it exits, but only while this
# flag file exists. Rotation OFF removes the flag: a running browser is left
# untouched, and if the user closes it, it stays closed. Rotation ON restores
# the flag so the browser is kept alive again.
# --------------------------------------------------------------------------- #
AUTOSTART_FLAG = os.path.join(
    os.environ.get("XDG_RUNTIME_DIR", f"/run/user/{os.getuid()}"),
    "kiosk-browser.enabled")


def set_browser_autostart(enabled):
    try:
        if enabled:
            open(AUTOSTART_FLAG, "w").close()
        elif os.path.exists(AUTOSTART_FLAG):
            os.remove(AUTOSTART_FLAG)
    except OSError as e:
        log("set_browser_autostart failed:", e)


def active_slots():
    return [s for s in STATE["slots"] if s["url"].strip()]


# --------------------------------------------------------------------------- #
# MQTT discovery + state
# --------------------------------------------------------------------------- #
DEVICE = {
    "identifiers": [DEVICE_ID],
    "name": DEVICE_ID,
    "manufacturer": "melisia",
    "model": "Raspberry Pi kiosk",
}


def _disc(client, component, key, cfg):
    cfg = dict(cfg)
    cfg["unique_id"] = f"{DEVICE_ID}_{key}"
    cfg["object_id"] = f"{DEVICE_ID}_{key}"
    cfg["device"] = DEVICE
    cfg["availability_topic"] = AVAIL_TOPIC
    client.publish(f"{PREFIX}/{component}/{DEVICE_ID}/{key}/config",
                   json.dumps(cfg), retain=True)


def publish_discovery(client):
    _disc(client, "switch", "screen", {
        "name": "Screen", "icon": "mdi:monitor",
        "command_topic": f"{BASE}/screen/set", "state_topic": f"{BASE}/screen/state",
        "payload_on": "ON", "payload_off": "OFF"})
    _disc(client, "switch", "rotation", {
        "name": "Screensaver rotation", "icon": "mdi:rotate-3d-variant",
        "command_topic": f"{BASE}/rotation/set", "state_topic": f"{BASE}/rotation/state",
        "payload_on": "ON", "payload_off": "OFF"})
    _disc(client, "switch", "fullscreen", {
        "name": "Fullscreen", "icon": "mdi:fullscreen",
        "command_topic": f"{BASE}/fullscreen/set", "state_topic": f"{BASE}/fullscreen/state",
        "payload_on": "ON", "payload_off": "OFF"})
    _disc(client, "binary_sensor", "activity", {
        "name": "Touch activity", "device_class": "motion",
        "state_topic": f"{BASE}/activity/state",
        "payload_on": "ON", "payload_off": "OFF"})
    _disc(client, "sensor", "current_url", {
        "name": "Current URL", "icon": "mdi:web",
        "state_topic": f"{BASE}/current_url"})
    _disc(client, "sensor", "last_activity", {
        "name": "Last activity", "device_class": "timestamp",
        "state_topic": f"{BASE}/last_activity"})
    _disc(client, "button", "next", {
        "name": "Next URL", "icon": "mdi:skip-next",
        "command_topic": f"{BASE}/next/set", "payload_press": "PRESS"})
    _disc(client, "button", "previous", {
        "name": "Previous URL", "icon": "mdi:skip-previous",
        "command_topic": f"{BASE}/previous/set", "payload_press": "PRESS"})
    _disc(client, "select", "show_url", {
        "name": "Show URL", "icon": "mdi:format-list-numbered",
        "command_topic": f"{BASE}/show_url/set", "state_topic": f"{BASE}/show_url/state",
        "options": [f"URL {i + 1}" for i in range(NSLOTS)]})
    for i in range(NSLOTS):
        n = i + 1
        _disc(client, "text", f"url_{n}", {
            "name": f"URL {n}", "icon": "mdi:link",
            "command_topic": f"{BASE}/url_{n}/set", "state_topic": f"{BASE}/url_{n}/state",
            "max": 255})
        _disc(client, "number", f"seconds_{n}", {
            "name": f"URL {n} seconds", "icon": "mdi:timer-outline",
            "command_topic": f"{BASE}/seconds_{n}/set", "state_topic": f"{BASE}/seconds_{n}/state",
            "min": 1, "max": 86400, "step": 1, "mode": "box",
            "unit_of_measurement": "s"})
    # Delete stale slot entities (empty retained config) when `slots` was reduced.
    for n in range(NSLOTS + 1, SLOT_CLEANUP_CEILING + 1):
        client.publish(f"{PREFIX}/text/{DEVICE_ID}/url_{n}/config", "", retain=True)
        client.publish(f"{PREFIX}/number/{DEVICE_ID}/seconds_{n}/config", "", retain=True)


def publish_states(client):
    client.publish(f"{BASE}/screen/state", "ON" if screen_on else "OFF", retain=True)
    client.publish(f"{BASE}/rotation/state", "ON" if STATE["rotation"] else "OFF", retain=True)
    client.publish(f"{BASE}/fullscreen/state", "ON" if fullscreen_on else "OFF", retain=True)
    client.publish(f"{BASE}/activity/state", "ON" if activity_on else "OFF", retain=True)
    for i in range(NSLOTS):
        n = i + 1
        client.publish(f"{BASE}/url_{n}/state", STATE["slots"][i]["url"], retain=True)
        client.publish(f"{BASE}/seconds_{n}/state", STATE["slots"][i]["seconds"], retain=True)


def publish_current_url(client, url):
    client.publish(f"{BASE}/current_url", url, retain=True)


# --------------------------------------------------------------------------- #
# MQTT callbacks
# --------------------------------------------------------------------------- #
def on_connect(client, userdata, flags, reason_code, properties=None):
    log("MQTT connected:", reason_code)
    client.publish(AVAIL_TOPIC, "online", retain=True)
    publish_discovery(client)
    publish_states(client)
    for t in ("screen/set", "rotation/set", "fullscreen/set", "next/set", "previous/set", "show_url/set"):
        client.subscribe(f"{BASE}/{t}")
    for i in range(NSLOTS):
        n = i + 1
        client.subscribe(f"{BASE}/url_{n}/set")
        client.subscribe(f"{BASE}/seconds_{n}/set")


def on_message(client, userdata, msg):
    topic = msg.topic[len(BASE) + 1:]
    payload = msg.payload.decode(errors="replace").strip()
    global current_slot
    try:
        if topic == "screen/set":
            screen_set(payload.upper() == "ON")
            client.publish(f"{BASE}/screen/state", "ON" if screen_on else "OFF", retain=True)
        elif topic == "rotation/set":
            with _state_lock:
                STATE["rotation"] = payload.upper() == "ON"
                save_state(STATE)
            set_browser_autostart(STATE["rotation"])
            client.publish(f"{BASE}/rotation/state", "ON" if STATE["rotation"] else "OFF", retain=True)
            wake.set()
        elif topic == "fullscreen/set":
            set_fullscreen(payload.upper() == "ON")
            client.publish(f"{BASE}/fullscreen/state", "ON" if fullscreen_on else "OFF", retain=True)
        elif topic == "next/set":
            current_slot += 1
            wake.set()
        elif topic == "previous/set":
            current_slot -= 1
            wake.set()
        elif topic == "show_url/set":
            # "URL 3" -> jump directly to slot 3 (if it has a URL)
            try:
                k = int(payload.split()[-1]) - 1
            except ValueError:
                k = -1
            active_idx = [i for i, s in enumerate(STATE["slots"]) if s["url"].strip()]
            if k in active_idx:
                current_slot = active_idx.index(k)
                wake.set()
        elif topic.startswith("url_") and topic.endswith("/set"):
            i = int(topic[4:-4]) - 1
            with _state_lock:
                STATE["slots"][i]["url"] = payload
                save_state(STATE)
            client.publish(f"{BASE}/url_{i + 1}/state", payload, retain=True)
            wake.set()
        elif topic.startswith("seconds_") and topic.endswith("/set"):
            i = int(topic[8:-4]) - 1
            secs = max(1, int(float(payload)))
            with _state_lock:
                STATE["slots"][i]["seconds"] = secs
                save_state(STATE)
            client.publish(f"{BASE}/seconds_{i + 1}/state", secs, retain=True)
    except Exception as e:
        log("on_message error:", topic, e)


# --------------------------------------------------------------------------- #
# Input activity (evdev) + idle monitor
# --------------------------------------------------------------------------- #
def _open_input_devices():
    devices = []
    for path in glob.glob("/dev/input/event*"):
        try:
            devices.append(evdev.InputDevice(path))
        except OSError:
            pass
    log("watching input devices:", [d.path for d in devices])
    return {d.fd: d for d in devices}


def input_watcher(client):
    global last_event, activity_on
    fdmap = _open_input_devices()
    last_scan = time.time()
    while True:
        # Re-enumerate periodically so USB re-plugs / device add-remove are picked up.
        if time.time() - last_scan > 30:
            for d in fdmap.values():
                try:
                    d.close()
                except OSError:
                    pass
            fdmap = _open_input_devices()
            last_scan = time.time()
        if not fdmap:
            time.sleep(5)
            continue
        try:
            r, _, _ = select.select(list(fdmap), [], [], 1.0)
        except (OSError, ValueError):
            # a device fd went stale (unplug) — re-enumerate
            fdmap = _open_input_devices()
            last_scan = time.time()
            continue
        got = False
        for fd in r:
            try:
                for _ev in fdmap[fd].read():
                    pass
                got = True
            except OSError:
                fdmap.pop(fd, None)
        if got:
            last_event = time.time()
            now = datetime.now(timezone.utc).astimezone().isoformat()
            client.publish(f"{BASE}/last_activity", now, retain=True)
            if not activity_on:
                activity_on = True
                client.publish(f"{BASE}/activity/state", "ON", retain=True)


def idle_monitor(client):
    global activity_on
    while True:
        time.sleep(1)
        if activity_on and (time.time() - last_event) >= IDLE_TIMEOUT:
            activity_on = False
            client.publish(f"{BASE}/activity/state", "OFF", retain=True)


# --------------------------------------------------------------------------- #
# Rotation loop (main thread)
# --------------------------------------------------------------------------- #
def rotation_loop(client):
    global current_slot
    prev_id = None
    while True:
        wake.clear()
        if not (STATE["rotation"] and screen_on):
            wake.wait(timeout=1.0)
            continue
        slots = active_slots()
        if not slots:
            wake.wait(timeout=2.0)
            continue
        urls = [s["url"] for s in slots]
        sync_tabs(urls)            # keep one pre-loaded tab per URL
        if not tab_ids:
            wake.wait(timeout=2.0)
            continue
        current_slot %= len(tab_ids)
        tid = tab_ids[current_slot]
        cdp_activate(tid)          # instant switch to the already-loaded tab
        publish_current_url(client, urls[current_slot])
        # reflect which slot is shown in the "Show URL" select
        active_idx = [i for i, s in enumerate(STATE["slots"]) if s["url"].strip()]
        if current_slot < len(active_idx):
            client.publish(f"{BASE}/show_url/state", f"URL {active_idx[current_slot] + 1}", retain=True)
        # The tab we just left is now off-screen — reload it so it is fresh
        # by the time it comes around again.
        if prev_id and prev_id != tid and prev_id in tab_ids:
            cdp_reload(prev_id)
        prev_id = tid
        if wake.wait(timeout=max(1, slots[current_slot]["seconds"])):
            # interrupted by next/prev/edit; re-evaluate without advancing
            continue
        current_slot += 1


# --------------------------------------------------------------------------- #
def main():
    global screen_on
    screen_on = screen_read()

    client = mqtt.Client(mqtt.CallbackAPIVersion.VERSION2,
                         client_id=f"kiosk-{DEVICE_ID}")
    client.username_pw_set(CFG["mqtt"]["username"], str(CFG["mqtt"]["password"]))
    client.will_set(AVAIL_TOPIC, "offline", retain=True)
    client.on_connect = on_connect
    client.on_message = on_message

    while True:
        try:
            client.connect(CFG["mqtt"]["host"], int(CFG["mqtt"]["port"]), keepalive=30)
            break
        except Exception as e:
            log("MQTT connect retry:", e)
            time.sleep(5)

    client.loop_start()
    threading.Thread(target=input_watcher, args=(client,), daemon=True).start()
    threading.Thread(target=idle_monitor, args=(client,), daemon=True).start()

    # Keep the browser auto-relaunched only while rotation is enabled.
    set_browser_autostart(STATE["rotation"])
    client.publish(f"{BASE}/fullscreen/state", "ON" if fullscreen_on else "OFF", retain=True)

    rotation_loop(client)


if __name__ == "__main__":
    main()
