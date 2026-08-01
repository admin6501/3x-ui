package service

import "testing"

func TestParseAccessLogLine(t *testing.T) {
	cases := []struct {
		name    string
		line    string
		ok      bool
		email   string
		ip      string
		dest    string
		network string
	}{
		{
			name:    "tcp with email",
			line:    "2026/08/01 12:00:00.123456 from 1.2.3.4:51000 accepted tcp:example.com:443 [inbound-443 -> direct] email: alice@example.com",
			ok:      true,
			email:   "alice@example.com",
			ip:      "1.2.3.4",
			dest:    "example.com",
			network: "tcp",
		},
		{
			name:    "udp keeps its network label",
			line:    "2026/08/01 12:00:01.000000 from 5.6.7.8:2000 accepted udp:dns.google:53 [in -> out] email: bob@example.com",
			ok:      true,
			email:   "bob@example.com",
			ip:      "5.6.7.8",
			dest:    "dns.google",
			network: "udp",
		},
		{
			name:  "ipv6 source and destination strip brackets",
			line:  "2026/08/01 12:00:02.000000 from [2001:db8::1]:40000 accepted tcp:[2606:4700::1]:443 [in -> out] email: carol@example.com",
			ok:    true,
			email: "carol@example.com",
			ip:    "2001:db8::1",
			dest:  "2606:4700::1",
		},
		{
			name: "no email is skipped (not a client connection)",
			line: "2026/08/01 12:00:03.000000 from 9.9.9.9:1 accepted tcp:example.org:443 [in -> out]",
			ok:   false,
		},
		{
			name: "api traffic is skipped",
			line: "2026/08/01 12:00:04.000000 from 127.0.0.1:1 accepted tcp:127.0.0.1:62789 [api -> api] email: x",
			ok:   false,
		},
		{
			name: "blank line",
			line: "   ",
			ok:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			row, ok := parseAccessLogLine(tc.line)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v (row: %+v)", ok, tc.ok, row)
			}
			if !tc.ok {
				return
			}
			if row.Email != tc.email {
				t.Errorf("email = %q, want %q", row.Email, tc.email)
			}
			if row.IP != tc.ip {
				t.Errorf("ip = %q, want %q", row.IP, tc.ip)
			}
			if row.Dest != tc.dest {
				t.Errorf("dest = %q, want %q", row.Dest, tc.dest)
			}
			if tc.network != "" && row.Network != tc.network {
				t.Errorf("network = %q, want %q", row.Network, tc.network)
			}
			if row.Timestamp == 0 {
				t.Error("timestamp should be parsed, got 0")
			}
		})
	}
}

// TestParseAccessLogLine_MissingTimestampFallsBackToNow guards the branch that
// stamps a row whose date failed to parse, so a malformed prefix does not drop
// an otherwise valid connection.
func TestParseAccessLogLine_MissingTimestampFallsBackToNow(t *testing.T) {
	row, ok := parseAccessLogLine("garbage from 1.1.1.1:1 accepted tcp:host:443 [in -> out] email: e@x")
	if !ok {
		t.Fatal("expected the row to parse despite a bad timestamp")
	}
	if row.Timestamp == 0 {
		t.Error("expected a fallback timestamp, got 0")
	}
}
