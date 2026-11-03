#!/usr/bin/env python3
"""Scout relay agent: lets the torrenter rotate the host's Mullvad relay.

Runs on the host (the Mullvad CLI is not reachable from inside a container)
as a small HTTP service with a shared bearer token. Standard library only.
Needs the Mullvad app's CLI of 2023 or later (`mullvad status --json`,
`mullvad relay set location <hostname>`); written against 2026.5.

Endpoints
  GET  /status             current tunnel state and relay
  GET  /relays             relays in the configured pool
  POST /rotate             switch to another relay in the pool; body may carry
                           {"exclude": ["hostname", ...], "reason": "..."}.
                           409 on cooldown or an empty pool; 502 when no relay
                           tried came up (the previous one is put back)

Environment
  RELAY_AGENT_TOKEN   required; clients send "Authorization: Bearer <token>"
  RELAY_AGENT_BIND    address:port to listen on (default 0.0.0.0:18642)
  RELAY_POOL          comma list of "country" or "country/city" codes the
                      rotation may pick from (default: the current country)
  RELAY_AVOID         comma list of hostnames or CIDR-like prefixes
                      ("146.70.198.") never to pick
  RELAY_COOLDOWN_S    seconds between rotations (default 60)
  RELAY_ATTEMPTS      relays to try per rotation before giving up (default 2)
  RELAY_CONNECT_S     seconds to wait for each relay's tunnel (default 15)
  RELAY_DEAD_S        seconds a relay that failed to come up is skipped (default 3600)
  MULLVAD_BIN         path to the mullvad CLI (default mullvad)
"""
import ipaddress
import json
import os
import re
import subprocess
import sys
import threading
import time
from http.server import BaseHTTPRequestHandler, HTTPServer

TOKEN = os.environ.get("RELAY_AGENT_TOKEN", "")
BIND = os.environ.get("RELAY_AGENT_BIND", "0.0.0.0:18642")
POOL = [p.strip().lower() for p in os.environ.get("RELAY_POOL", "").split(",") if p.strip()]
AVOID = [a.strip().lower() for a in os.environ.get("RELAY_AVOID", "").split(",") if a.strip()]
COOLDOWN = int(os.environ.get("RELAY_COOLDOWN_S", "60"))
ATTEMPTS = max(1, int(os.environ.get("RELAY_ATTEMPTS", "2")))
CONNECT_S = int(os.environ.get("RELAY_CONNECT_S", "15"))
DEAD_S = int(os.environ.get("RELAY_DEAD_S", "3600"))
MULLVAD = os.environ.get("MULLVAD_BIN", "mullvad")

_lock = threading.Lock()
# history: most recent hostnames first; dead: hostname -> when it failed to come up
_state = {"last_rotation_at": 0.0, "history": [], "dead": {}}


class RotateFailed(RuntimeError):
    """Every relay tried stayed down; the previous one is back."""


def log(msg):
    print(time.strftime("%Y-%m-%dT%H:%M:%S"), msg, flush=True)


def run(*args, timeout=30):
    out = subprocess.run([MULLVAD, *args], capture_output=True, text=True, timeout=timeout)
    if out.returncode != 0:
        raise RuntimeError(f"{' '.join(args)}: {out.stderr.strip() or out.stdout.strip()}")
    return out.stdout


HOST_RE = re.compile(r"^(?P<country>[a-z]{2})-(?P<city>[a-z0-9]+)-")
LOCATION_RE = re.compile(r"^\s*Location:\s*(?P<value>.+?)\s*$", re.M)


def parse_status(status_json, get_out):
    """Parse `mullvad status --json` and `mullvad relay get` into one dict.

    The relay's country and city come from its hostname (cc-city-wg-nnn): the
    constraints name a city only while one is set."""
    try:
        data = json.loads(status_json or "")
    except ValueError:
        data = {}
    if not isinstance(data, dict):
        data = {}
    state = str(data.get("state") or "unknown").lower()
    details = data.get("details") if isinstance(data.get("details"), dict) else {}
    endpoint = (details.get("endpoint") or {}).get("address") or ""
    location = details.get("location") or {}
    relay = location.get("hostname") or ""
    hm = HOST_RE.match(relay)
    lm = LOCATION_RE.search(get_out or "")
    return {
        "connected": state == "connected",
        "state": state,
        "endpoint": endpoint,
        "ip": endpoint.rsplit(":", 1)[0].strip("[]") if endpoint else "",
        "exit_ip": location.get("ipv4") or "",
        "relay": relay,
        "city": hm.group("city") if hm else "",
        "country": hm.group("country") if hm else "",
        "constraints": lm.group("value") if lm else "",
    }


COUNTRY_RE = re.compile(r"^(?P<name>.+?) \((?P<code>[a-z]{2})\)\s*$")
CITY_LINE_RE = re.compile(r"^\t(?P<name>.+?) \((?P<code>[a-z0-9]+)\)")
# "ca-mtr-wg-001 (146.70.198.66, 2a0d:...) - hosted by M247 (rented)"; older CLIs put
# "WireGuard, " or "OpenVPN, " before "hosted by" and had no ownership.
RELAY_RE = re.compile(
    r"^\t\t(?P<host>[a-z0-9-]+) \((?P<ip>[\d.]+)(?:, [^)]*)?\) - (?:(?P<proto>\w+), )?hosted by (?P<provider>.+?)"
    r"(?: \((?P<ownership>[^)]+)\))?\s*$"
)


def parse_relays(list_out):
    """Parse `mullvad relay list` into a list of relay dicts (WireGuard only)."""
    relays = []
    country = city = ""
    for line in (list_out or "").splitlines():
        if not line.strip():
            continue
        if not line.startswith("\t"):
            m = COUNTRY_RE.match(line)
            if m:
                country = m.group("code")
            continue
        if line.startswith("\t\t"):
            m = RELAY_RE.match(line)
            if not m:
                continue
            wireguard = m.group("proto").lower() == "wireguard" if m.group("proto") else "-wg-" in m.group("host")
            if wireguard:
                relays.append({
                    "hostname": m.group("host"), "ip": m.group("ip"), "provider": m.group("provider"),
                    "country": country, "city": city,
                })
            continue
        m = CITY_LINE_RE.match(line)
        if m:
            city = m.group("code")
    return relays


def in_pool(relay, pool):
    if not pool:
        return True
    for entry in pool:
        country, _, city = entry.partition("/")
        if relay["country"] == country and (not city or relay["city"] == city):
            return True
    return False


def avoided(relay, avoid):
    for a in avoid:
        if relay["hostname"] == a or relay["ip"].startswith(a):
            return True
    return False


def block_of(ip):
    try:
        return str(ipaddress.ip_network(ip + "/24", strict=False))
    except ValueError:
        return ip


def choose_relay(relays, current, exclude, pool, avoid, history, dead=()):
    """Pick the next relay: in the pool, not avoided, dead, current or excluded,
    preferring a different /24 (bans are usually per block) and the least
    recently used."""
    current_block = block_of(current.get("ip", "")) if current.get("ip") else ""
    excluded = set(exclude or []) | set(dead) | {current.get("relay", "")}
    candidates = [r for r in relays if in_pool(r, pool) and not avoided(r, avoid) and r["hostname"] not in excluded]
    if not candidates:
        return None

    def rank(r):
        same_block = 1 if block_of(r["ip"]) == current_block else 0
        recency = history.index(r["hostname"]) if r["hostname"] in history else len(history) + 1
        # lower is better: new block first, then least recently used, then name
        return (same_block, -recency, r["hostname"])

    candidates.sort(key=rank)
    return candidates[0]


def dead_relays():
    """Hostnames that failed to come up within the last DEAD_S seconds."""
    now = time.time()
    _state["dead"] = {h: t for h, t in _state["dead"].items() if now - t < DEAD_S}
    return set(_state["dead"])


def current_status():
    return parse_status(run("status", "--json"), run("relay", "get"))


def wait_connected(target=None, timeout=None):
    """Poll until the tunnel is up on `target` (any relay when None) or the time is up.

    `relay set` returns before the daemon drops the old tunnel, so right after it the
    status still says connected to the previous relay: without the target that would
    pass as success."""
    deadline = time.time() + (CONNECT_S if timeout is None else timeout)
    while True:
        st = current_status()
        if st["connected"] and (target is None or st["relay"] == target):
            return st
        if time.time() >= deadline:
            return st
        time.sleep(0.5)


def rotate(exclude, reason):
    with _lock:
        since = time.time() - _state["last_rotation_at"]
        if since < COOLDOWN:
            raise RuntimeError(f"rotated {int(since)}s ago; cooldown is {COOLDOWN}s")
        before = current_status()
        pool = POOL or ([before["country"]] if before["country"] else [])
        relays = parse_relays(run("relay", "list"))
        tried = []
        for _ in range(ATTEMPTS):
            pick = choose_relay(relays, before, list(exclude or []) + tried, pool, AVOID, _state["history"], dead_relays())
            if pick is None:
                break
            log(f"rotating {before.get('relay') or '?'} -> {pick['hostname']} ({pick['ip']}) reason={reason!r}")
            run("relay", "set", "location", pick["hostname"])
            _state["last_rotation_at"] = time.time()
            _state["history"].insert(0, pick["hostname"])
            del _state["history"][10:]
            after = wait_connected(pick["hostname"])
            if after["connected"] and after["relay"] == pick["hostname"]:
                return {"previous": before, "relay": pick, "status": after, "reason": reason, "failed": tried}
            log(f"{pick['hostname']} did not come up in {CONNECT_S}s (state {after['state']}); skipping it for {DEAD_S}s")
            _state["dead"][pick["hostname"]] = time.time()
            tried.append(pick["hostname"])
        if not tried:
            raise RuntimeError("no relay left to pick in the pool")
        # Do not leave the host on a relay that never came up.
        if before.get("relay"):
            run("relay", "set", "location", before["relay"])
            log(f"put {before['relay']} back: {wait_connected(before['relay'])['state']}")
        raise RotateFailed(f"no relay came up (tried {', '.join(tried)})")


class Handler(BaseHTTPRequestHandler):
    server_version = "scout-relay-agent/2"

    def _send(self, code, payload):
        body = json.dumps(payload).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def _authed(self):
        if not TOKEN:
            self._send(503, {"error": "RELAY_AGENT_TOKEN is not set"})
            return False
        if self.headers.get("Authorization", "") != f"Bearer {TOKEN}":
            self._send(401, {"error": "unauthorized"})
            return False
        return True

    def do_GET(self):
        if not self._authed():
            return
        try:
            if self.path == "/status":
                st = current_status()
                st["last_rotation_at"] = _state["last_rotation_at"] or None
                st["history"] = list(_state["history"])
                st["dead"] = sorted(dead_relays())
                st["pool"] = POOL
                self._send(200, st)
            elif self.path.startswith("/relays"):
                relays = parse_relays(run("relay", "list"))
                before = current_status()
                pool = POOL or ([before["country"]] if before["country"] else [])
                self._send(200, [r for r in relays if in_pool(r, pool) and not avoided(r, AVOID)])
            else:
                self._send(404, {"error": "not found"})
        except Exception as exc:  # noqa: BLE001 - every failure must become a response
            log(f"error: {exc}")
            self._send(500, {"error": str(exc)})

    def do_POST(self):
        if not self._authed():
            return
        if self.path != "/rotate":
            self._send(404, {"error": "not found"})
            return
        try:
            n = int(self.headers.get("Content-Length", "0") or 0)
            body = json.loads(self.rfile.read(n) or b"{}") if n else {}
            result = rotate(body.get("exclude") or [], body.get("reason") or "")
            self._send(200, result)
        except RotateFailed as exc:
            log(f"rotate failed: {exc}")
            self._send(502, {"error": str(exc)})
        except RuntimeError as exc:
            log(f"rotate refused: {exc}")
            self._send(409, {"error": str(exc)})
        except Exception as exc:  # noqa: BLE001
            log(f"error: {exc}")
            self._send(500, {"error": str(exc)})

    def log_message(self, *args):
        pass


def main():
    if not TOKEN:
        print("RELAY_AGENT_TOKEN must be set", file=sys.stderr)
    host, _, port = BIND.rpartition(":")
    srv = HTTPServer((host or "0.0.0.0", int(port)), Handler)
    log(f"listening on {BIND}; pool={POOL or 'current country'} avoid={AVOID} cooldown={COOLDOWN}s "
        f"attempts={ATTEMPTS} connect={CONNECT_S}s")
    srv.serve_forever()


if __name__ == "__main__":
    main()
