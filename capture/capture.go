// SPDX-License-Identifier: EUPL-1.2

// Package capture reads TCP segments from a capture file or, live, from Npcap.
// Read-only: it never sends a packet.
package capture

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcapgo"
)

// Handler gets each server → client TCP segment that carries data.
type Handler func(at time.Time, flow string, seq uint32, payload []byte)

// Frame decodes one captured frame and hands it on if it's a server → client
// TCP segment with data.
func Frame(data []byte, lt layers.LinkType, at time.Time, dir Direction, h Handler) {
	pk := gopacket.NewPacket(data, lt, gopacket.DecodeOptions{Lazy: true, NoCopy: true})
	t, _ := pk.Layer(layers.LayerTypeTCP).(*layers.TCP)
	if t == nil || pk.NetworkLayer() == nil || len(t.Payload) == 0 {
		return
	}
	src, dst := pk.NetworkLayer().NetworkFlow().Endpoints()
	srcIP, dstIP := net.IP(src.Raw()), net.IP(dst.Raw())
	if !dir.Inbound(srcIP, dstIP, uint16(t.SrcPort), uint16(t.DstPort)) {
		return
	}
	h(at, fmt.Sprintf("%v:%d>%v:%d", src, t.SrcPort, dst, t.DstPort), t.Seq, t.Payload)
}

type reader interface {
	ReadPacketData() ([]byte, gopacket.CaptureInfo, error)
	LinkType() layers.LinkType
}

// Open reads a .pcapng or .pcap file.
func Open(path string) (reader, func(), error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	closeFn := func() { fh.Close() }
	if ng, err := pcapgo.NewNgReader(fh, pcapgo.DefaultNgReaderOptions); err == nil {
		return ng, closeFn, nil
	}
	if _, err := fh.Seek(0, io.SeekStart); err != nil {
		closeFn()
		return nil, nil, err
	}
	pc, err := pcapgo.NewReader(fh)
	if err != nil {
		closeFn()
		return nil, nil, fmt.Errorf("%s: not a pcap or pcapng file: %w", path, err)
	}
	return pc, closeFn, nil
}

// Replay feeds a capture file to h. With realtime it keeps the capture's
// pace (for watching the window); otherwise as fast as it reads.
func Replay(ctx context.Context, path string, realtime bool, h Handler) error {
	r, closeFn, err := Open(path)
	if err != nil {
		return err
	}
	defer closeFn()
	var first, start time.Time
	for ctx.Err() == nil {
		data, ci, err := r.ReadPacketData()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		at := ci.Timestamp
		if realtime {
			if first.IsZero() {
				first, start = at, time.Now()
			}
			select {
			case <-time.After(time.Until(start.Add(at.Sub(first)))):
			case <-ctx.Done():
				return nil
			}
			at = time.Now()
		}
		Frame(data, r.LinkType(), at, Direction{}, h)
	}
	return nil
}
