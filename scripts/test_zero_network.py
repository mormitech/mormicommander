#!/usr/bin/env python3
"""Unit tests of the zero-network detector's parsing (scripts/zero_network.py): strace -f lines, ss -tunapH lines and
the process tree. They run on any system:

    python3 -m unittest discover -s scripts -p 'test_*.py'
"""
import os
import sys
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import zero_network as zn  # noqa: E402

# Lines as strace -f -qq -tt (-Y) writes them with -o; the addresses are the kinds the app's processes make.
STRACE = r'''
4711  10:00:00.000001 execve("/opt/mc/mormicommander", ["/opt/mc/mormicommander", "--control=0"], 0x7ffd /* 40 vars */) = 0
4711  10:00:00.100000 connect(3, {sa_family=AF_UNIX, sun_path="/tmp/.X11-unix/X99"}, 20) = 0
4711  10:00:00.100100 connect(4, {sa_family=AF_UNIX, sun_path=@"/tmp/.X11-unix/X99"}, 21) = 0
4711  10:00:00.100200 connect(5, {sa_family=AF_UNIX, sun_path="/tmp/dbus-AbC"}, 110) = 0
4711  10:00:00.100300 connect(6, {sa_family=AF_INET, sin_port=htons(41234), sin_addr=inet_addr("127.0.0.1")}, 16) = 0
4711  10:00:00.100400 connect(7, {sa_family=AF_INET6, sin6_port=htons(8080), sin6_flowinfo=htonl(0), inet_pton(AF_INET6, "::1", &sin6_addr), sin6_scope_id=0}, 28) = -1 ECONNREFUSED (Connection refused)
4711  10:00:00.100500 sendto(8, {{len=20, type=RTM_GETADDR, flags=NLM_F_REQUEST|NLM_F_DUMP, seq=1, pid=0}, {ifa_family=AF_UNSPEC}}, 20, 0, {sa_family=AF_NETLINK, nl_pid=0, nl_groups=00000000}, 12) = 20
4712  10:00:00.200000 execve("/usr/lib/x86_64-linux-gnu/webkitgtk-6.0/WebKitWebProcess", ["/usr/lib/x86_64-linux-gnu/webkitg"..., "9", "33"], 0x55 /* 41 vars */ <unfinished ...>
4711  10:00:00.200100 sendmsg(5, {msg_name=NULL, msg_namelen=0, msg_iov=[{iov_base="l\1\0\1", iov_len=4}], msg_iovlen=1, msg_controllen=0, msg_flags=0}, MSG_NOSIGNAL) = 4
4712  10:00:00.200200 <... execve resumed>) = 0
4713  10:00:00.300000 execve("/usr/lib/x86_64-linux-gnu/webkitgtk-6.0/WebKitNetworkProcess", ["/usr/lib/x86_64-linux-gnu/webkitg"..., "11", "35"], 0x55 /* 41 vars */) = 0
4713  10:00:00.400000 connect(12, {sa_family=AF_INET, sin_port=htons(53), sin_addr=inet_addr("127.0.0.53")}, 16) = 0
4713  10:00:00.400100 sendmmsg(12, [{msg_hdr={msg_name=NULL, msg_namelen=0, msg_iov=[{iov_base="\x12\x34", iov_len=29}], msg_iovlen=1, msg_controllen=0, msg_flags=0}, msg_len=29}], 2, MSG_NOSIGNAL) = 2
4713  10:00:00.400200 connect(13, {sa_family=AF_INET, sin_port=htons(443), sin_addr=inet_addr("93.184.215.14")}, 16 <unfinished ...>
4711  10:00:00.400300 connect(14, {sa_family=AF_UNIX, sun_path="/run/user/1001/bus"}, 110) = 0
4713  10:00:00.400400 <... connect resumed>) = -1 EINPROGRESS (Operation now in progress)
4713  10:00:00.500000 connect(15, {sa_family=AF_INET6, sin6_port=htons(443), sin6_flowinfo=htonl(0), inet_pton(AF_INET6, "2606:2800:21f:cb07:6820:80da:af6b:8b2c", &sin6_addr), sin6_scope_id=0}, 28) = -1 ENETUNREACH (Network is unreachable)
4713  10:00:00.500100 sendto(16, "\x12\x34\x01\x00", 29, MSG_NOSIGNAL, {sa_family=AF_INET, sin_port=htons(53), sin_addr=inet_addr("8.8.8.8")}, 16) = 29
4713  10:00:00.500200 sendmsg(17, {msg_name={sa_family=AF_INET, sin_port=htons(5353), sin_addr=inet_addr("224.0.0.251")}, msg_namelen=16, msg_iov=[{iov_base="\0\0", iov_len=2}], msg_iovlen=1, msg_controllen=0, msg_flags=0}, 0) = 2
4713  10:00:00.500300 connect(18, {sa_family=AF_UNIX, sun_path="/run/systemd/resolve/io.systemd.Resolve"}, 42) = 0
4713  10:00:00.500400 connect(19, {sa_family=AF_INET6, sin6_port=htons(80), sin6_flowinfo=htonl(0), inet_pton(AF_INET6, "::ffff:127.0.0.1", &sin6_addr), sin6_scope_id=0}, 28) = 0
4713  10:00:00.500500 connect(20, {sa_family=AF_UNSPEC, sa_data="\0\0\0\0\0\0\0\0\0\0\0\0\0\0"}, 16) = 0
4713  10:00:00.500600 connect(21, {sa_family=AF_PACKET, sll_protocol=htons(ETH_P_ALL), sll_ifindex=if_nametoindex("eth0")}, 20) = 0
4714  10:00:00.600000 execve("/usr/bin/claude", ["claude", "--version"], 0x55 /* 41 vars */) = -1 ENOENT (No such file or directory)
4715<WebKitNetworkP> 10:00:00.700000 connect(22, {sa_family=AF_INET, sin_port=htons(80), sin_addr=inet_addr("192.0.2.1")}, 16) = -1 EINPROGRESS (Operation now in progress)
4716  connect(23, {sa_family=AF_INET, sin_port=htons(9), sin_addr=inet_addr("10.0.0.1")}, 16) = 0
+++ exited with 0 +++
garbage line that is not strace output
'''.lstrip("\n").splitlines()


class StraceParsing(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.events, cls.procs = zn.parse_strace(STRACE)
        cls.findings = [e for e in cls.events if e["finding"]]

    def find(self, **kw):
        return [e for e in self.events if all(e.get(k) == v for k, v in kw.items())]

    def test_processes_named_from_execve(self):
        self.assertEqual(self.procs[4711], "/opt/mc/mormicommander")
        self.assertTrue(self.procs[4712].endswith("WebKitWebProcess"), "an execve split by another line is joined back")
        self.assertTrue(self.procs[4713].endswith("WebKitNetworkProcess"))
        self.assertNotIn(4714, self.procs, "a failed execve names nothing")

    def test_unix_netlink_unspec_and_loopback_pass(self):
        allowed = [e for e in self.events if not e["finding"]]
        kinds = {(e["family"], e["addr"]) for e in allowed}
        self.assertIn(("AF_UNIX", "/tmp/.X11-unix/X99"), kinds)
        self.assertIn(("AF_UNIX", "@/tmp/.X11-unix/X99"), kinds, "abstract sockets keep their @")
        self.assertIn(("AF_UNIX", "/run/user/1001/bus"), kinds)
        self.assertIn(("AF_INET", "127.0.0.1"), kinds)
        self.assertIn(("AF_INET6", "::1"), kinds)
        self.assertIn(("AF_INET6", "::ffff:127.0.0.1"), kinds)
        self.assertTrue(self.find(family="AF_NETLINK"))
        self.assertTrue(self.find(family="AF_UNSPEC"))
        self.assertFalse(self.find(call="sendmsg", pid=4711), "sendmsg without msg_name has no destination")

    def test_dns_is_a_finding_even_on_loopback(self):
        e = self.find(addr="127.0.0.53", port=53)
        self.assertEqual(len(e), 1)
        self.assertEqual(e[0]["finding"], "DNS lookup (port 53)")
        e = self.find(addr="8.8.8.8", call="sendto")
        self.assertEqual(e[0]["finding"], "DNS lookup (port 53)")
        e = self.find(addr="/run/systemd/resolve/io.systemd.Resolve")
        self.assertIn("systemd-resolved", e[0]["finding"])

    def test_remote_connections_are_findings(self):
        e = self.find(addr="93.184.215.14")
        self.assertEqual(e[0]["port"], 443)
        self.assertEqual(e[0]["result"], "unfinished", "the address comes from the first half of a split call")
        self.assertEqual(e[0]["finding"], "connection to another machine")
        e = self.find(addr="2606:2800:21f:cb07:6820:80da:af6b:8b2c")
        self.assertEqual(e[0]["result"], "-1 ENETUNREACH", "a failed attempt still counts")
        self.assertTrue(e[0]["finding"])
        self.assertTrue(self.find(addr="224.0.0.251", call="sendmsg")[0]["finding"], "mDNS multicast")
        self.assertTrue(self.find(family="AF_PACKET")[0]["finding"].startswith("unexpected socket family"))

    def test_comm_and_no_time(self):
        e = self.find(addr="192.0.2.1")
        self.assertEqual(e[0]["comm"], "WebKitNetworkP")
        self.assertEqual(e[0]["time"], "10:00:00.700000")
        e = self.find(addr="10.0.0.1")
        self.assertEqual((e[0]["pid"], e[0]["time"]), (4716, ""))

    def test_the_findings_are_exactly_these(self):
        got = sorted((e["addr"], e["port"]) for e in self.findings)
        want = sorted([("127.0.0.53", 53), ("93.184.215.14", 443), ("2606:2800:21f:cb07:6820:80da:af6b:8b2c", 443),
                       ("8.8.8.8", 53), ("224.0.0.251", 5353), ("/run/systemd/resolve/io.systemd.Resolve", None),
                       ("", None), ("192.0.2.1", 80), ("10.0.0.1", 9)])
        self.assertEqual(got, want)

    def test_describe(self):
        e = self.find(addr="192.0.2.1")[0]
        line = zn.describe_event(e, self.procs)
        self.assertIn("pid 4715 (WebKitNetworkP) connect AF_INET 192.0.2.1:80 -> -1 EINPROGRESS", line)
        e = self.find(addr="93.184.215.14")[0]
        self.assertIn("(WebKitNetworkProcess)", zn.describe_event(e, self.procs))


class Addresses(unittest.TestCase):
    def test_local(self):
        for a in ("127.0.0.1", "127.8.9.10", "::1", "[::1]", "::ffff:127.0.0.1", "0.0.0.0", "::", "127.0.0.53%lo", "*"):
            self.assertTrue(zn.is_local_address(a), a)
        for a in ("192.0.2.1", "10.0.0.1", "8.8.8.8", "2606:4700::1", "::ffff:8.8.8.8", "fe80::1%eth0", "example.com"):
            self.assertFalse(zn.is_local_address(a), a)

    def test_split_hostport(self):
        self.assertEqual(zn.split_hostport("127.0.0.1:53"), ("127.0.0.1", "53"))
        self.assertEqual(zn.split_hostport("[::1]:8080"), ("::1", "8080"))
        self.assertEqual(zn.split_hostport("127.0.0.53%lo:53"), ("127.0.0.53", "53"))
        self.assertEqual(zn.split_hostport("[fe80::1]%eth0:22"), ("fe80::1", "22"))
        self.assertEqual(zn.split_hostport("*:*"), ("*", "*"))
        self.assertEqual(zn.split_hostport("0.0.0.0:*"), ("0.0.0.0", "*"))
        self.assertEqual(zn.split_hostport("*"), ("*", "*"))


SS = '''
tcp   LISTEN    0      4096       127.0.0.1:41321        0.0.0.0:*     users:(("mormicommander",pid=4711,fd=9))
tcp   ESTAB     0      0          127.0.0.1:41321      127.0.0.1:50000 users:(("mormicommander",pid=4711,fd=12))
tcp   SYN-SENT  0      1           10.1.0.4:39876      192.0.2.1:9     users:(("python3",pid=5001,fd=3))
udp   ESTAB     0      0          127.0.0.1:45000     127.0.0.53:53    users:(("WebKitNetworkP",pid=4713,fd=20))
udp   UNCONN    0      0            0.0.0.0:5353         0.0.0.0:*     users:(("WebKitNetworkP",pid=4713,fd=21))
tcp   LISTEN    0      128            [::]:22               [::]:*     users:(("sshd",pid=900,fd=3))
tcp   ESTAB     0      0     [::ffff:127.0.0.1]:8080  [::ffff:127.0.0.1]:41000 users:(("WebKitWebProces",pid=4712,fd=30))
tcp   ESTAB     0      0           10.1.0.4:22        10.1.0.9:51000   users:(("sshd",pid=901,fd=4))
tcp   ESTAB     0      0           10.1.0.4:48000   93.184.215.14:443  users:(("WebKitNetworkP",pid=4713,fd=22),("WebKitNetworkP",pid=4713,fd=23))
udp   UNCONN    0      0          127.0.0.1:323          0.0.0.0:*
'''


class SSParsing(unittest.TestCase):
    def test_parse(self):
        entries = zn.parse_ss(SS)
        self.assertEqual(len(entries), 10)
        self.assertEqual(entries[0]["procs"], [("mormicommander", 4711)])
        self.assertEqual(entries[8]["procs"], [("WebKitNetworkP", 4713), ("WebKitNetworkP", 4713)])
        self.assertEqual(entries[9]["procs"], [])

    def test_findings_only_for_the_tree(self):
        entries = zn.parse_ss(SS)
        got = [(e["peer"], why, comm, pid) for e, why, comm, pid in zn.ss_findings(entries, {4711, 4712, 4713, 5001})]
        self.assertEqual(got, [
            ("192.0.2.1:9", "connection to another machine", "python3", 5001),
            ("127.0.0.53:53", "DNS lookup (port 53)", "WebKitNetworkP", 4713),
            ("0.0.0.0:*", "socket open to every network (0.0.0.0:5353)", "WebKitNetworkP", 4713),
            ("93.184.215.14:443", "connection to another machine", "WebKitNetworkP", 4713),
        ])
        self.assertEqual(zn.ss_findings(entries, {4711, 4712}), [], "loopback listener and connections are allowed")


class ProcTree(unittest.TestCase):
    def test_descendants(self):
        stats = {
            1: "1 (systemd) S 0 1 1",
            100: "100 (dbus-run-sessi) S 1 100 100",
            101: "101 (xvfb-run) S 100 100 100",
            102: "102 (strace) S 101 100 100",
            4711: "4711 (mormi commander) S 102 100 100",   # a space in comm
            4712: "4712 (WebKitWebProces) S 4711 100 100",
            4713: "4713 ((sd-x) ) S 4711 100 100",          # a parenthesis in comm
            4720: "4720 (bwrap) S 4712 100 100",
            999: "999 (other) S 1 999 999",
            5: "garbage",
        }
        self.assertEqual(zn.proc_tree(4711, stats), {4711, 4712, 4713, 4720})
        self.assertEqual(zn.proc_tree(102, stats), {102, 4711, 4712, 4713, 4720})
        self.assertEqual(zn.proc_tree(42, stats), {42})


class Fixtures(unittest.TestCase):
    def test_png_is_valid(self):
        data = zn.png(4, 3, (1, 2, 3))
        self.assertTrue(data.startswith(b"\x89PNG\r\n\x1a\n"))
        self.assertIn(b"IHDR", data[:16])
        self.assertTrue(data.endswith(b"IEND\xaeB`\x82"))

    def test_remote_files_point_at_the_web(self):
        for text in (zn.REMOTE_MD, zn.REMOTE_HTML, zn.REMOTE_SVG):
            self.assertIn("https://example.com/", text)


if __name__ == "__main__":
    unittest.main()
