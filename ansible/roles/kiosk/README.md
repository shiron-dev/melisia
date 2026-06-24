# kiosk role

Turns a Raspberry Pi (`home-kiosk`) into a Home-Assistant-controlled kiosk.

## What it does

- Runs **Chromium in kiosk mode** under the existing labwc/Wayland autologin
  session (launched from `~/.config/labwc/autostart`, kept alive by a restart
  loop, DevTools port exposed on localhost for the agent).
- Runs **`kiosk-agent`** (a `systemd --user` service, lingering) that bridges the
  device to Home Assistant over MQTT using **MQTT discovery** — entities appear
  automatically under a `home-kiosk` device.

## Home Assistant entities (auto-discovered)

| Entity | Purpose |
|---|---|
| `switch.home_kiosk_screen` | Display power (`wlopm` DPMS on the output) |
| `switch.home_kiosk_screensaver_rotation` | Enable/disable URL cycling **and** browser auto-relaunch (see below) |
| `switch.home_kiosk_fullscreen` | Toggle Chromium fullscreen via a uinput F11 keypress (ON=fullscreen, OFF=windowed/operable) |
| `binary_sensor.home_kiosk_touch_activity` | `motion` — ON on touch/input, OFF after `idle_timeout` |
| `sensor.home_kiosk_current_url` | URL currently shown |
| `sensor.home_kiosk_last_activity` | Timestamp of last input |
| `button.home_kiosk_next_url` / `..._previous_url` | Manual advance |
| `text.home_kiosk_url_1..N` | Per-slot URL (editable) |
| `number.home_kiosk_url_N_seconds` | Per-slot dwell time (editable) |

The device **never powers the screen on touch by itself** — it only reports
`binary_sensor.home_kiosk_touch_activity`. Screen-off and wake-on-touch are
implemented as Home Assistant automations (below).

## Rotation ⇒ browser auto-relaunch

Chromium is launched by the labwc `autostart` restart-loop, but **only while the
agent's flag file (`$XDG_RUNTIME_DIR/kiosk-browser.enabled`) exists**. The agent
creates/removes that flag from the `Screensaver rotation` switch:

- **rotation ON** — flag present → the browser is kept alive (relaunched if it exits).
- **rotation OFF** — flag absent → a running browser is **left as-is** (not killed);
  if the user closes it, it is **not** relaunched. (At boot with rotation off, the
  browser is simply not launched.)

## Transport

The agent connects to the home-ep mosquitto broker's **authenticated LAN
listener** (`192.168.1.61:1884`, user `kiosk`). The internal `1883` listener
stays anonymous/docker-only. Password: hash in
`compose/hosts/home-ep/home-assistant/env.secrets.yml`, plaintext in
`ansible/group_vars/home_kiosk/kiosk.secrets.yml` — they must match.

## Configuration

- Defaults: `defaults/main.yml`.
- Site values + secrets: `ansible/group_vars/home_kiosk/kiosk.yml` and
  `kiosk.secrets.yml`.
- **Number of URL candidates is variable**: set `kiosk_slots` (default 6) in
  `group_vars/home_kiosk/kiosk.yml` and re-run the role. Increasing adds
  `url_N`/`seconds_N` entities; decreasing removes the now-stale ones from HA
  (the agent clears them up to `slots_cleanup_ceiling`, default 64). Empty slots
  are simply skipped in the rotation.
- The playlist is **seeded** from `kiosk_default_playlist` on first run and then
  persisted to `/var/lib/kiosk/state.json`; afterwards edit it from HA (the
  `text`/`number` slot entities). Re-running the role does not clobber HA edits.

## Example HA automations (manage in the HA UI)

Wake the screen when the panel is touched:

```yaml
alias: home-kiosk wake on touch
triggers:
  - trigger: state
    entity_id: binary_sensor.home_kiosk_touch_activity
    to: "on"
actions:
  - action: switch.turn_on
    target: { entity_id: switch.home_kiosk_screen }
```

Sleep the screen after 5 minutes of no touch:

```yaml
alias: home-kiosk sleep when idle
triggers:
  - trigger: state
    entity_id: binary_sensor.home_kiosk_touch_activity
    to: "off"
    for: { minutes: 5 }
actions:
  - action: switch.turn_off
    target: { entity_id: switch.home_kiosk_screen }
```

## Notes

- **Hardware**: currently a **Raspberry Pi 4 (aarch64)**. The Pi 4 GPU (V3D,
  GLES 3.x) runs Chromium with **hardware acceleration** (`--use-angle=gles`,
  the rpi default), so dashboards and the browser UI render properly. No
  SwiftShader workaround is needed (that was only required on the GLES2-only
  Pi 3B). Chromium runs `--ozone-platform=wayland`; stderr is logged to
  `$XDG_RUNTIME_DIR/kiosk-chromium.log`.
- **Fullscreen mode**: Chromium is launched with **`--start-fullscreen
  --window-size=<width>,<height>`** (the output resolution, default 1920×1080
  via `kiosk_screen_width`/`kiosk_screen_height`), **not** `--kiosk`. It boots
  full-screen filling the output, but because `--kiosk` is not used, the HA
  `fullscreen` switch / F11 toggle **works**: F11 drops to a same-size windowed
  (operable) view and back. The explicit `--window-size` is required — plain
  `--start-fullscreen` clamped the window to ~945×1060 on this labwc (Chromium
  had no initial size), which is why `--kiosk` was previously used as a
  workaround. Page content is touch/click-operable in either state.
- First deploy restarts `lightdm` to apply the autostart (brief screen flash).
- Screen power uses `wlopm --off/--on <output>` (DPMS via the
  wlr-output-power-management protocol). `vcgencmd display_power` does **not**
  work under labwc/KMS, and `wlr-randr --off` disables the whole output (can be
  reverted by the compositor) — `wlopm` is the correct primitive.
- The agent inherits the graphical session env (`WAYLAND_DISPLAY`) via the
  labwc autostart's `systemctl --user import-environment`; it also auto-detects
  the Wayland socket as a fallback.
- **Boot reliability**: device-specific PARTUUIDs (`group_vars`) and the HDMI
  output name must be re-checked when the SD card or Pi changes. If a Pi 4
  "boots once after flashing but black-screens on every reboot", that's a
  **failing/counterfeit SD card or a stale bootloader EEPROM config**, not this
  role — reset the EEPROM (rpi-imager bootloader recovery image) and use a
  genuine SD card.
