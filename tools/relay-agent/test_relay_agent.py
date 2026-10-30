import unittest

import relay_agent as ra

LIST = """Canada (ca)
\tCalgary (yyc) @ 51.03701°N, -114.05831°W
\t\tca-yyc-wg-201 (38.240.225.36, 2606:9580:438:32::b01f) - WireGuard, hosted by techfutures
\tMontreal (mtr) @ 45.50530°N, -73.55250°W
\t\tca-mtr-ovpn-001 (37.120.237.66) - OpenVPN, hosted by M247
\t\tca-mtr-wg-001 (146.70.198.66, 2a0d:5600:9:c::f001) - WireGuard, hosted by M247
\t\tca-mtr-wg-004 (188.241.176.194, 2a0d:5600:9:16::f001) - WireGuard, hosted by M247
\tToronto (tor) @ 43.66667°N, -79.41667°W
\t\tca-tor-wg-001 (178.249.214.1, 2001:db8::1) - WireGuard, hosted by DataPacket

Sweden (se)
\tStockholm (sto) @ 59.3°N, 18.0°E
\t\tse-sto-wg-001 (185.65.134.1) - WireGuard, hosted by 31173
"""

STATUS = "Tunnel status: Connected to WireGuard 188.241.176.194:40580 over UDP\n"
GET = "Current constraints: Any tunnel protocol ... in city mtr, ca, hostname ca-mtr-wg-004 using any provider\n"


class ParseTests(unittest.TestCase):
    def test_status(self):
        st = ra.parse_status(STATUS, GET)
        self.assertTrue(st["connected"])
        self.assertEqual(st["ip"], "188.241.176.194")
        self.assertEqual(st["relay"], "ca-mtr-wg-004")
        self.assertEqual((st["city"], st["country"]), ("mtr", "ca"))
        self.assertFalse(ra.parse_status("Tunnel status: Disconnected\n", "")["connected"])

    def test_relays_wireguard_only_with_location(self):
        relays = ra.parse_relays(LIST)
        names = [r["hostname"] for r in relays]
        self.assertEqual(names, ["ca-yyc-wg-201", "ca-mtr-wg-001", "ca-mtr-wg-004", "ca-tor-wg-001", "se-sto-wg-001"])
        mtr = [r for r in relays if r["hostname"] == "ca-mtr-wg-001"][0]
        self.assertEqual((mtr["country"], mtr["city"], mtr["ip"], mtr["provider"]), ("ca", "mtr", "146.70.198.66", "M247"))


class ChooseTests(unittest.TestCase):
    def setUp(self):
        self.relays = ra.parse_relays(LIST)
        self.current = {"relay": "ca-mtr-wg-004", "ip": "188.241.176.194", "country": "ca", "city": "mtr"}

    def test_prefers_new_block_in_pool_and_skips_avoided(self):
        pick = ra.choose_relay(self.relays, self.current, [], ["ca"], ["146.70.198."], [])
        self.assertIn(pick["hostname"], {"ca-yyc-wg-201", "ca-tor-wg-001"})

    def test_city_pool_and_exclusions(self):
        pick = ra.choose_relay(self.relays, self.current, [], ["ca/mtr"], [], [])
        self.assertEqual(pick["hostname"], "ca-mtr-wg-001", "the only other WireGuard relay in Montreal")
        self.assertIsNone(ra.choose_relay(self.relays, self.current, ["ca-mtr-wg-001"], ["ca/mtr"], [], []))

    def test_least_recently_used_wins(self):
        history = ["ca-tor-wg-001"]  # used most recently
        pick = ra.choose_relay(self.relays, self.current, [], ["ca"], ["146.70.198."], history)
        self.assertEqual(pick["hostname"], "ca-yyc-wg-201")


if __name__ == "__main__":
    unittest.main()
