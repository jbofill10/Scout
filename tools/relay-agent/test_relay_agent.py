import json
import unittest
from unittest import mock

import relay_agent as ra

# `mullvad relay list` of app 2026.5 (all relays WireGuard, ownership in brackets) plus a
# pre-2026 OpenVPN line, which the parser must still skip.
LIST = """Canada (ca)
\tCalgary (yyc) @ 51.03701°N, -114.05831°W
\t\tca-yyc-wg-201 (38.240.225.36, 2606:9580:438:32::f001) - hosted by techfutures (rented)
\tMontreal (mtr) @ 45.50530°N, -73.55250°W
\t\tca-mtr-wg-001 (146.70.198.66, 2a0d:5600:9:c::f001) - hosted by M247 (rented)
\t\tca-mtr-wg-004 (188.241.176.194, 2a0d:5600:9:16::f001) - hosted by M247 (rented)
\t\tca-mtr-wg-307 (23.234.123.2, 2607:9000:f00:8::f001) - hosted by Tzulo (rented)
\tToronto (tor) @ 43.66667°N, -79.41667°W
\t\tca-tor-wg-001 (178.249.214.2, 2a02:6ea0:de08:1::f001) - hosted by DataPacket (rented)

Denmark (dk)
\tCopenhagen (cph) @ 55.7°N, 12.6°E
\t\tdk-cph-wg-001 (45.129.56.67, 2a03:1b20:8:f011::f001) - hosted by 31173 (Mullvad-owned)
\t\tdk-cph-ovpn-001 (45.129.56.60) - OpenVPN, hosted by 31173
"""


def status_json(state, hostname=None, endpoint="188.241.176.194:40580", exit_ip="188.241.176.4"):
    data = {"state": state}
    if hostname:
        data["details"] = {
            "endpoint": {"address": endpoint, "protocol": "udp", "quantum_resistant": True,
                         "tunnel_interface": "wg0-mullvad", "daita": False},
            "location": {"latitude": 45.5, "longitude": -73.5, "ipv4": exit_ip, "ipv6": None,
                         "mullvad_exit_ip": True, "hostname": hostname, "city": "Montreal", "country": "Canada"},
        }
    return json.dumps(data)


STATUS = status_json("connected", "ca-mtr-wg-004")
GET = "Generic constraints\n    Location:               city mtr, ca\n    Provider(s):            any\n"


class ParseTests(unittest.TestCase):
    def test_status(self):
        st = ra.parse_status(STATUS, GET)
        self.assertTrue(st["connected"])
        self.assertEqual(st["ip"], "188.241.176.194")
        self.assertEqual(st["exit_ip"], "188.241.176.4")
        self.assertEqual(st["relay"], "ca-mtr-wg-004")
        self.assertEqual((st["city"], st["country"]), ("mtr", "ca"))
        self.assertEqual(st["constraints"], "city mtr, ca")

    def test_status_not_connected(self):
        st = ra.parse_status(status_json("disconnected"), "")
        self.assertEqual((st["connected"], st["state"], st["relay"], st["country"]), (False, "disconnected", "", ""))
        self.assertEqual(ra.parse_status("Tunnel status: Connected\n", "")["state"], "unknown")
        self.assertEqual(ra.parse_status("", None)["state"], "unknown")

    def test_relays_wireguard_only_with_location(self):
        relays = ra.parse_relays(LIST)
        names = [r["hostname"] for r in relays]
        self.assertEqual(names, ["ca-yyc-wg-201", "ca-mtr-wg-001", "ca-mtr-wg-004", "ca-mtr-wg-307", "ca-tor-wg-001",
                                 "dk-cph-wg-001"])
        mtr = [r for r in relays if r["hostname"] == "ca-mtr-wg-001"][0]
        self.assertEqual((mtr["country"], mtr["city"], mtr["ip"], mtr["provider"]), ("ca", "mtr", "146.70.198.66", "M247"))
        owned = [r for r in relays if r["hostname"] == "dk-cph-wg-001"][0]
        self.assertEqual((owned["country"], owned["city"], owned["provider"]), ("dk", "cph", "31173"))

    def test_relays_older_cli_format(self):
        relays = ra.parse_relays("Sweden (se)\n\tStockholm (sto) @ 59.3°N, 18.0°E\n"
                                 "\t\tse-sto-wg-001 (185.65.134.1) - WireGuard, hosted by 31173\n"
                                 "\t\tse-sto-ovpn-001 (185.65.134.2) - OpenVPN, hosted by 31173\n")
        self.assertEqual([r["hostname"] for r in relays], ["se-sto-wg-001"])


class ChooseTests(unittest.TestCase):
    def setUp(self):
        self.relays = ra.parse_relays(LIST)
        self.current = {"relay": "ca-mtr-wg-004", "ip": "188.241.176.194", "country": "ca", "city": "mtr"}

    def test_prefers_new_block_in_pool_and_skips_avoided(self):
        pick = ra.choose_relay(self.relays, self.current, [], ["ca"], ["146.70.198."], [])
        self.assertIn(pick["hostname"], {"ca-yyc-wg-201", "ca-mtr-wg-307", "ca-tor-wg-001"})

    def test_city_pool_and_exclusions(self):
        pick = ra.choose_relay(self.relays, self.current, [], ["ca/mtr"], ["146.70.198."], [])
        self.assertEqual(pick["hostname"], "ca-mtr-wg-307", "the only other allowed relay in Montreal")
        self.assertIsNone(ra.choose_relay(self.relays, self.current, ["ca-mtr-wg-307"], ["ca/mtr"], ["146.70.198."], []))

    def test_least_recently_used_wins(self):
        history = ["ca-tor-wg-001", "ca-mtr-wg-307"]  # most recent first
        pick = ra.choose_relay(self.relays, self.current, [], ["ca"], ["146.70.198."], history)
        self.assertEqual(pick["hostname"], "ca-yyc-wg-201")

    def test_dead_relays_are_skipped(self):
        pick = ra.choose_relay(self.relays, self.current, [], ["ca/mtr"], ["146.70.198."], [], dead={"ca-mtr-wg-307"})
        self.assertIsNone(pick)


class WaitTests(unittest.TestCase):
    """wait_connected() must not take the old tunnel, still up right after `relay set`, for the new one."""

    def run_wait(self, statuses, target, timeout=0.05):
        seq = list(statuses)
        with mock.patch.object(ra, "current_status", lambda: seq.pop(0) if len(seq) > 1 else seq[0]), \
                mock.patch.object(ra.time, "sleep", lambda s: None):
            return ra.wait_connected(target, timeout=timeout)

    def test_waits_for_the_target_relay(self):
        old = ra.parse_status(STATUS, GET)
        new = ra.parse_status(status_json("connected", "ca-mtr-wg-307", "23.234.123.2:1", "23.234.123.125"), GET)
        self.assertEqual(self.run_wait([old, ra.parse_status(status_json("connecting"), GET), new], "ca-mtr-wg-307")["relay"],
                         "ca-mtr-wg-307")

    def test_gives_up_on_the_old_relay(self):
        old = ra.parse_status(STATUS, GET)
        st = self.run_wait([old], "ca-mtr-wg-307")
        self.assertEqual((st["connected"], st["relay"]), (True, "ca-mtr-wg-004"))

    def test_any_relay_when_no_target(self):
        old = ra.parse_status(STATUS, GET)
        self.assertEqual(self.run_wait([old], None)["relay"], "ca-mtr-wg-004")


class RotateTests(unittest.TestCase):
    """rotate() against a fake CLI: the set calls it makes and what it answers when a relay stays down."""

    def setUp(self):
        ra._state.update({"last_rotation_at": 0.0, "history": [], "dead": {}})
        self.calls = []
        self.after = []  # what wait_connected returns, in order

        def run(*args, timeout=30):
            self.calls.append(args)
            if args == ("status", "--json"):
                return STATUS
            if args == ("relay", "get"):
                return GET
            if args == ("relay", "list"):
                return LIST
            if args[:3] == ("relay", "set", "location"):
                return ""
            raise AssertionError(args)

        patches = [mock.patch.object(ra, "run", run),
                   mock.patch.object(ra, "wait_connected", lambda target=None, timeout=None: self.after.pop(0)),
                   mock.patch.object(ra, "POOL", ["ca"]), mock.patch.object(ra, "AVOID", ["146.70.198."]),
                   mock.patch.object(ra, "ATTEMPTS", 2), mock.patch.object(ra, "COOLDOWN", 0)]
        for p in patches:
            p.start()
            self.addCleanup(p.stop)

    def sets(self):
        return [c[3] for c in self.calls if c[:3] == ("relay", "set", "location")]

    def test_first_relay_up(self):
        self.after = [ra.parse_status(status_json("connected", "ca-mtr-wg-307", "23.234.123.2:1", "23.234.123.125"), GET)]
        result = ra.rotate([], "test")
        self.assertEqual(result["relay"]["hostname"], "ca-mtr-wg-307")
        self.assertEqual(result["status"]["relay"], "ca-mtr-wg-307")
        self.assertEqual(result["failed"], [])
        self.assertEqual(self.sets(), ["ca-mtr-wg-307"])
        self.assertEqual(ra._state["history"], ["ca-mtr-wg-307"])

    def test_second_relay_after_first_stays_down(self):
        self.after = [ra.parse_status(status_json("connecting"), GET),
                      ra.parse_status(status_json("connected", "ca-tor-wg-001", "178.249.214.2:1", "178.249.214.9"), GET)]
        result = ra.rotate([], "test")
        self.assertEqual(result["relay"]["hostname"], "ca-tor-wg-001")
        self.assertEqual(result["failed"], ["ca-mtr-wg-307"])
        self.assertEqual(self.sets(), ["ca-mtr-wg-307", "ca-tor-wg-001"])
        self.assertEqual(ra.dead_relays(), {"ca-mtr-wg-307"})

    def test_all_down_restores_previous(self):
        self.after = [ra.parse_status(status_json("connecting"), GET), ra.parse_status(status_json("connecting"), GET),
                      ra.parse_status(STATUS, GET)]
        with self.assertRaises(ra.RotateFailed) as ctx:
            ra.rotate([], "test")
        self.assertIn("ca-mtr-wg-307, ca-tor-wg-001", str(ctx.exception))
        self.assertEqual(self.sets(), ["ca-mtr-wg-307", "ca-tor-wg-001", "ca-mtr-wg-004"])
        self.assertEqual(ra.dead_relays(), {"ca-mtr-wg-307", "ca-tor-wg-001"})
        self.assertGreater(ra._state["last_rotation_at"], 0, "a failed rotation still starts the cooldown")

    def test_empty_pool_is_refused_not_failed(self):
        with mock.patch.object(ra, "POOL", ["ca/mtr"]), mock.patch.object(ra, "AVOID", ["146.70.198.", "ca-mtr-wg-307"]):
            with self.assertRaises(RuntimeError) as ctx:
                ra.rotate([], "test")
        self.assertNotIsInstance(ctx.exception, ra.RotateFailed)
        self.assertEqual(self.sets(), [])


if __name__ == "__main__":
    unittest.main()
