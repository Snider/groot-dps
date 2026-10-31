// SPDX-License-Identifier: EUPL-1.2

package capture

import "net"

// Direction tells server → client segments from the rest.
type Direction struct {
	Local []net.IP // this PC's addresses; empty for capture files
}

// Inbound: is this segment coming to us from a server?
func (d Direction) Inbound(src, dst net.IP, srcPort, dstPort uint16) bool {
	if len(d.Local) > 0 {
		return d.local(dst) && !d.local(src)
	}
	switch sp, dp := private(src), private(dst); {
	case !sp && dp:
		return true
	case sp && !dp:
		return false
	}
	return srcPort < dstPort // the server's port is below the client's ephemeral one
}

func (d Direction) local(ip net.IP) bool {
	for _, l := range d.Local {
		if l.Equal(ip) {
			return true
		}
	}
	return false
}

func private(ip net.IP) bool { return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() }
