// SPDX-License-Identifier: EUPL-1.2

package live

import (
	"encoding/binary"
	"encoding/hex"
	"testing"
	"time"
)

var keepalive = []byte{0x0E, 0x00, 0x36, 0, 0, 0, 0, 0, 0, 0, 0}

func frame(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return append(binary.AppendUvarint(nil, uint64(len(b)+4)), b...)
}

// hitRec: a layout-4 damage record, actor → target.
func hitRec(target, actor uint64, skill uint32, amount uint64) []byte {
	b := []byte{0x04, 0x38}
	b = binary.AppendUvarint(b, target)
	b = append(b, 0x04, 0x00)
	b = binary.AppendUvarint(b, actor)
	b = binary.LittleEndian.AppendUint32(b, skill)
	b = append(b, 0x5d, 0x02)
	b = append(b, make([]byte, 8)...)
	b = binary.AppendUvarint(b, 10000)
	b = binary.AppendUvarint(b, amount)
	return append(binary.AppendUvarint(nil, uint64(len(b)+4)), b...)
}

const grooty = 14957 // the self record below

func TestSegmentsToSnapshot(t *testing.T) {
	self := frame(t, "3336ed745e91c128370647726f6f747918051e000000011c0000007f0100007f0100001c000000d002040000000000")
	var stream []byte
	stream = append(stream, keepalive...)
	stream = append(stream, self...)
	stream = append(stream, hitRec(50001, grooty, 17_050_240, 3000)...)
	stream = append(stream, hitRec(50001, grooty, 17_050_240, 2000)...)

	p := New(nil)
	t0 := time.UnixMilli(1_000_000)
	// Three segments, the last two swapped: reassembly puts them back.
	a, b, c := stream[:20], stream[20:60], stream[60:]
	p.Segment(t0, "game", 100, a)
	p.Segment(t0.Add(time.Second), "game", 100+60, c)
	p.Segment(t0.Add(2*time.Second), "game", 100+20, b)

	s := p.Snapshot()
	if !s.You || s.Fight == nil || s.Packets != 3 {
		t.Fatalf("snapshot %+v", s)
	}
	if len(s.Fight.Rows) != 1 || s.Fight.Rows[0].Name != "Grooty" || !s.Fight.Rows[0].Self ||
		s.Fight.Rows[0].Class != "Cleric" || s.Fight.Total != 5000 {
		t.Fatalf("fight %+v", s.Fight)
	}
}

// Other connections (chat, the launcher, TLS) never sync, so they add nothing.
func TestOtherFlowsAddNothing(t *testing.T) {
	p := New(nil)
	p.Segment(time.Now(), "tls", 1, []byte{0x17, 0x03, 0x03, 0x00, 0x05, 1, 2, 3, 4, 5})
	p.Segment(time.Now(), "junk", 1, hitRec(50001, grooty, 17_050_240, 3000)) // no keepalive first
	if s := p.Snapshot(); s.Packets != 0 || s.Fight != nil {
		t.Fatalf("snapshot %+v", s)
	}
}

func TestFlowsAreCapped(t *testing.T) {
	p := New(nil)
	for i := range maxFlows + 10 {
		p.Segment(time.UnixMilli(int64(i)), string(rune('A'+i)), 1, []byte{1})
	}
	if len(p.flows) != maxFlows {
		t.Fatalf("%d flows", len(p.flows))
	}
}

func TestResetClearsTheFight(t *testing.T) {
	p := New(nil)
	p.Segment(time.Now(), "game", 1, append(append([]byte{}, keepalive...), hitRec(50001, grooty, 17_050_240, 3000)...))
	p.Reset()
	if s := p.Snapshot(); s.Fight != nil {
		t.Fatalf("snapshot %+v", s)
	}
}
