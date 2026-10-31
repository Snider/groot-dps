// SPDX-License-Identifier: EUPL-1.2

package live

import (
	"encoding/binary"
	"encoding/hex"
	"testing"
	"time"

	"groot/dps/meter"
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

// A long session stays small: old fights get pruned as segments arrive.
func TestLongSessionsArePruned(t *testing.T) {
	p := New(nil)
	t0 := time.UnixMilli(0)
	seq := uint32(1)
	push := func(at time.Time, b []byte) {
		p.Segment(at, "game", seq, b)
		seq += uint32(len(b))
	}
	push(t0, keepalive)
	for i := range 120 { // a mob a minute for two hours
		push(t0.Add(time.Duration(i)*time.Minute), hitRec(uint64(60_000+i), grooty, 17_050_240, 10))
	}
	p.With(func(m *meter.Meter) {
		kept := 0
		for i := range 120 {
			if m.Fight(uint32(60_000+i)) != nil {
				kept++
			}
		}
		if kept > meter.KeepMS/60_000+2 {
			t.Errorf("%d of 120 fights kept", kept)
		}
	})
}

// A fight you were in is handed over once when it ends: killed, or gone quiet.
func TestFinishedFightsAreHandedOverOnce(t *testing.T) {
	self := frame(t, "3336ed745e91c128370647726f6f747918051e000000011c0000007f0100007f0100001c000000d002040000000000")
	var got []FightView
	p := New(nil)
	p.OnFinished = func(_ time.Time, v FightView) { got = append(got, v) }
	t0 := time.UnixMilli(0)
	seq := uint32(1)
	push := func(at time.Duration, b []byte) {
		p.Segment(t0.Add(at), "game", seq, b)
		seq += uint32(len(b))
	}
	push(0, append(append([]byte{}, keepalive...), self...))
	push(0, hitRec(50001, grooty, 17_050_240, 100))
	push(10*time.Second, hitRec(50001, grooty, 17_050_240, 100))
	push(12*time.Second, frame(t, "4236d18603"+"0003"))        // 50001 killed
	push(13*time.Second, hitRec(50002, 9999, 11_020_100, 100)) // a stranger's fight
	push(20*time.Second, hitRec(50002, 9999, 11_020_100, 100))
	push(90*time.Second, keepalive)
	if len(got) != 1 || got[0].Target != 50001 || !got[0].Killed || got[0].Total != 200 {
		t.Fatalf("got %+v", got)
	}
	push(200*time.Second, keepalive)
	if len(got) != 1 {
		t.Fatalf("handed over again: %d", len(got))
	}
}

// Too short to count: not kept.
func TestBlipsArentHandedOver(t *testing.T) {
	self := frame(t, "3336ed745e91c128370647726f6f747918051e000000011c0000007f0100007f0100001c000000d002040000000000")
	n := 0
	p := New(nil)
	p.OnFinished = func(time.Time, FightView) { n++ }
	b := append(append(append([]byte{}, keepalive...), self...), hitRec(50001, grooty, 17_050_240, 100)...)
	p.Segment(time.UnixMilli(0), "game", 1, b)
	p.Segment(time.UnixMilli(120_000), "game", 1+uint32(len(b)), keepalive)
	if n != 0 {
		t.Fatalf("%d handed over", n)
	}
}

func TestFlushHandsOverAFightInProgress(t *testing.T) {
	self := frame(t, "3336ed745e91c128370647726f6f747918051e000000011c0000007f0100007f0100001c000000d002040000000000")
	n := 0
	p := New(nil)
	p.OnFinished = func(time.Time, FightView) { n++ }
	b := append(append(append([]byte{}, keepalive...), self...), hitRec(50001, grooty, 17_050_240, 100)...)
	p.Segment(time.UnixMilli(0), "game", 1, b)
	b2 := hitRec(50001, grooty, 17_050_240, 100)
	p.Segment(time.UnixMilli(8000), "game", 1+uint32(len(b)), b2)
	p.Flush(time.UnixMilli(9000))
	p.Flush(time.UnixMilli(9500))
	if n != 1 {
		t.Fatalf("%d handed over", n)
	}
}
