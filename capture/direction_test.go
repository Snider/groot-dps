// SPDX-License-Identifier: EUPL-1.2

package capture

import (
	"net"
	"testing"
)

func TestInboundByLocalAddress(t *testing.T) {
	d := Direction{Local: []net.IP{net.ParseIP("192.168.1.20")}}
	server, me := net.ParseIP("203.0.113.5"), net.ParseIP("192.168.1.20")
	// Even when the server's port is the higher one.
	if !d.Inbound(server, me, 60000, 50123) || d.Inbound(me, server, 50123, 60000) {
		t.Fatal("wrong way round")
	}
	other := net.ParseIP("192.168.1.99") // someone else on the LAN
	if d.Inbound(server, other, 7777, 50123) {
		t.Fatal("not our traffic")
	}
}

// Capture files don't say which address was ours: public → private is inbound.
func TestInboundWithoutLocalAddresses(t *testing.T) {
	var d Direction
	server, me := net.ParseIP("203.0.113.5"), net.ParseIP("192.168.1.20")
	if !d.Inbound(server, me, 60000, 50123) || d.Inbound(me, server, 50123, 60000) {
		t.Fatal("public/private")
	}
	// Both private (a VPN, a LAN server): the server's port is the lower one.
	a, b := net.ParseIP("10.0.0.1"), net.ParseIP("10.0.0.2")
	if !d.Inbound(a, b, 7777, 50123) || d.Inbound(b, a, 50123, 7777) {
		t.Fatal("port fallback")
	}
}
