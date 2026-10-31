package wire

import (
	"bytes"
	"testing"
)

func TestKeepaliveIsElevenBytes(t *testing.T) {
	// 0x0E = 14, one varint byte: 14 + 1 - 4 = 11.
	if got := len(frame(keepalive[1:])); got != 11 {
		t.Fatalf("frame(keepalive body) = %d bytes, want 11", got)
	}
}

func TestSplitterSyncsOnKeepaliveAndDropsIt(t *testing.T) {
	a := pkt(0x3804, 1, 2, 3)
	b := pkt(0x3603, 9)
	var s Splitter
	got := s.Feed(cat([]byte{0xDE, 0xAD, 0xBE}, keepalive, a, b))
	if len(got) != 2 || !bytes.Equal(got[0], a) || !bytes.Equal(got[1], b) {
		t.Fatalf("got %x", got)
	}
}

func TestSplitterIgnoresEverythingBeforeFirstSync(t *testing.T) {
	var s Splitter
	if got := s.Feed(pkt(0x3804, 1, 2, 3)); len(got) != 0 {
		t.Fatalf("emitted before sync: %x", got)
	}
}

func TestSplitterHandlesPacketsSplitAcrossChunks(t *testing.T) {
	a := pkt(0x3804, 1, 2, 3, 4, 5, 6)
	stream := cat(keepalive, a, keepalive, a)
	var s Splitter
	var got [][]byte
	for _, b := range stream { // worst case: one byte at a time
		got = append(got, s.Feed([]byte{b})...)
	}
	if len(got) != 2 || !bytes.Equal(got[0], a) || !bytes.Equal(got[1], a) {
		t.Fatalf("got %x", got)
	}
}

func TestSplitterSyncPatternSplitAcrossChunks(t *testing.T) {
	a := pkt(0x3804, 7)
	stream := cat([]byte{0x01, 0x02}, keepalive, a)
	var s Splitter
	got := s.Feed(stream[:3]) // ends with 0x0E
	got = append(got, s.Feed(stream[3:])...)
	if len(got) != 1 || !bytes.Equal(got[0], a) {
		t.Fatalf("got %x", got)
	}
}

func TestSplitterResyncsAfterBadLength(t *testing.T) {
	a := pkt(0x3804, 1)
	// 0x02 → size 2+1-4 = -1: nonsense, must drop and hunt for the next keepalive.
	var s Splitter
	got := s.Feed(cat(keepalive, []byte{0x02, 0x55, 0x66}, keepalive, a))
	if len(got) != 1 || !bytes.Equal(got[0], a) {
		t.Fatalf("got %x", got)
	}
}

func TestSplitterCopiesPackets(t *testing.T) {
	var s Splitter
	got := s.Feed(cat(keepalive, pkt(0x3804, 1)))
	s.Feed(cat(pkt(0x3804, 2), pkt(0x3804, 3), pkt(0x3804, 4)))
	if got[0][len(got[0])-1] != 1 {
		t.Fatal("emitted packet aliases the internal buffer")
	}
}

// Framing cases from A2Tools' framing.rs tests (c55788c).

func TestSplitterSkipsZeroPadding(t *testing.T) {
	a, b := pkt(0x3804, 1), pkt(0x3603, 2)
	var s Splitter
	got := s.Feed(cat(keepalive, []byte{0, 0}, a, []byte{0}, b))
	if len(got) != 2 || !bytes.Equal(got[0], a) || !bytes.Equal(got[1], b) {
		t.Fatalf("got %x", got)
	}
}

// The game server talks TLS from the same port on a second connection. Read
// as a frame, 17 is a length of 23 and the walk lands in ciphertext: kills
// came out 20-35% short of the boss's HP until A2Tools skipped these.
func TestSplitterSkipsTLSRecordsWhole(t *testing.T) {
	tls := []byte{0x17, 0x03, 0x03, 0x05, 0x7a, 0x88, 0x71} // ciphertext that would claim a 14 KB packet
	for i := range 0x057a - 2 {
		tls = append(tls, byte(i%251)|1)
	}
	a, b := pkt(0x3623, 1), pkt(0x3804, filler(200)...)
	stream := cat(keepalive, a, tls, b)
	for _, cut := range []int{len(stream), len(keepalive) + len(a) + 600} {
		var s Splitter
		got := append(s.Feed(stream[:cut]), s.Feed(stream[cut:])...)
		if len(got) != 2 || !bytes.Equal(got[0], a) || !bytes.Equal(got[1], b) {
			t.Fatalf("cut %d: got %d packets", cut, len(got))
		}
	}
}

func TestSplitterTakesPacketsUpTo64KB(t *testing.T) {
	for _, n := range []int{122, 123, 200, 16_380, 20_000, 50_000} {
		a := pkt(0x3804, filler(n)...)
		var s Splitter
		got := s.Feed(cat(keepalive, a, keepalive))
		if len(got) != 1 || !bytes.Equal(got[0], a) {
			t.Errorf("%d-byte body: got %d packets", n, len(got))
		}
	}
}

// A length past 16 KB that runs past what's arrived is garbage, not a fragment
// worth holding the stream for. Unless it's a bundle: zone-load bundles run past 16 KB.
func TestSplitterOnlyWaitsLongForBundles(t *testing.T) {
	a := pkt(0x3623, 7)
	garbage := cat(uv(40_000), []byte{0x12, 0x34, 0, 0, 0, 0})
	var s Splitter
	if got := s.Feed(cat(keepalive, garbage, keepalive, a)); len(got) != 1 || !bytes.Equal(got[0], a) {
		t.Fatalf("garbage length held the stream: got %x", got)
	}

	big := frame(cat([]byte{0xFF, 0xFF}, []byte{0x00, 0x00, 0x01, 0x00}, filler(40_000)))
	var w Splitter
	got := w.Feed(cat(keepalive, big[:9_000]))
	got = append(got, w.Feed(cat(big[9_000:], a))...)
	if len(got) != 2 || !bytes.Equal(got[0], big) || !bytes.Equal(got[1], a) {
		t.Fatalf("bundle not waited for: got %d packets", len(got))
	}
}
