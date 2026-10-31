// SPDX-License-Identifier: EUPL-1.2

package capture

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcapgo"
)

func tcpFrame(t *testing.T, srcPort, dstPort uint16, seq uint32, payload []byte) []byte {
	t.Helper()
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{1, 2, 3, 4, 5, 6}, DstMAC: net.HardwareAddr{6, 5, 4, 3, 2, 1}, EthernetType: layers.EthernetTypeIPv4}
	ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolTCP, SrcIP: net.IP{203, 0, 113, 5}, DstIP: net.IP{192, 168, 1, 20}}
	tcp := &layers.TCP{SrcPort: layers.TCPPort(srcPort), DstPort: layers.TCPPort(dstPort), Seq: seq, ACK: true, PSH: true, Window: 512}
	tcp.SetNetworkLayerForChecksum(ip)
	buf := gopacket.NewSerializeBuffer()
	if err := gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, tcp, gopacket.Payload(payload)); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type got struct {
	flow    string
	seq     uint32
	payload []byte
}

func collect(out *[]got) Handler {
	return func(_ time.Time, flow string, seq uint32, payload []byte) {
		*out = append(*out, got{flow, seq, bytes.Clone(payload)})
	}
}

func TestFrameKeepsServerToClientData(t *testing.T) {
	var out []got
	Frame(tcpFrame(t, 7777, 50123, 42, []byte("game")), layers.LinkTypeEthernet, time.Now(), collect(&out))
	Frame(tcpFrame(t, 50123, 7777, 9, []byte("from us")), layers.LinkTypeEthernet, time.Now(), collect(&out))
	Frame(tcpFrame(t, 7777, 50123, 46, nil), layers.LinkTypeEthernet, time.Now(), collect(&out)) // a bare ACK
	if len(out) != 1 || out[0].flow != "203.0.113.5:7777>192.168.1.20:50123" || out[0].seq != 42 || string(out[0].payload) != "game" {
		t.Fatalf("got %+v", out)
	}
}

func TestReplayReadsAPcap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.pcap")
	fh, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := pcapgo.NewWriter(fh)
	w.WriteFileHeader(65535, layers.LinkTypeEthernet)
	for i, p := range [][]byte{[]byte("ab"), []byte("cd")} {
		f := tcpFrame(t, 7777, 50123, uint32(1+2*i), p)
		w.WritePacket(gopacket.CaptureInfo{Timestamp: time.UnixMilli(int64(1000 * i)), CaptureLength: len(f), Length: len(f)}, f)
	}
	fh.Close()

	var out []got
	if err := Replay(context.Background(), path, false, collect(&out)); err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || string(out[0].payload)+string(out[1].payload) != "abcd" {
		t.Fatalf("got %+v", out)
	}
}
