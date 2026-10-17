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
