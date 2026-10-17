package tcp

import (
	"bytes"
	"testing"
)

func push(t *testing.T, s *Stream, seq uint32, data string) string {
	t.Helper()
	out, _ := s.Push(seq, []byte(data))
	return string(out)
}

func TestInOrder(t *testing.T) {
	var s Stream
	got := push(t, &s, 100, "abc") + push(t, &s, 103, "def")
	if got != "abcdef" {
		t.Fatalf("got %q", got)
	}
}

func TestOutOfOrderIsHeldThenReleased(t *testing.T) {
	var s Stream
	got := push(t, &s, 100, "abc")
	if out := push(t, &s, 106, "ghi"); out != "" {
		t.Fatalf("released early: %q", out)
	}
	got += push(t, &s, 103, "def")
	if got != "abcdefghi" {
		t.Fatalf("got %q", got)
	}
}

func TestDuplicateAndOverlapAreTrimmed(t *testing.T) {
	var s Stream
	got := push(t, &s, 100, "abcd")
	got += push(t, &s, 100, "abcd") // retransmit
	got += push(t, &s, 102, "cdef") // overlap
	if got != "abcdef" {
		t.Fatalf("got %q", got)
	}
}

func TestSequenceWraps(t *testing.T) {
	var s Stream
	got := push(t, &s, 0xFFFFFFFE, "ab") + push(t, &s, 0, "cd")
	if got != "abcd" {
		t.Fatalf("got %q", got)
	}
}

func TestLostSegmentGivesUpAndReportsGap(t *testing.T) {
	var s Stream
	s.Push(100, []byte("a"))
	var out []byte
	var gap bool
	// Segment 101 never arrives; keep sending later data until we give up.
	for i := range maxPending + 1 {
		o, g := s.Push(uint32(102+i), []byte{byte('A' + i%26)})
		out = append(out, o...)
		gap = gap || g
	}
	if !gap {
		t.Fatal("expected a gap report")
	}
	if len(out) != maxPending+1 || out[0] != 'A' {
		t.Fatalf("after a gap we should continue from the oldest held segment, got %q", out)
	}
	if !bytes.HasSuffix(out, []byte{byte('A' + maxPending%26)}) {
		t.Fatalf("lost the newest segment: %q", out)
	}
}

func FuzzStream(f *testing.F) {
	f.Add(uint32(100), []byte("abc"), uint32(106), []byte("ghi"), uint32(103), []byte("def"))
	f.Fuzz(func(t *testing.T, s1 uint32, p1 []byte, s2 uint32, p2 []byte, s3 uint32, p3 []byte) {
		var s Stream
		total := 0
		for _, seg := range []struct {
			seq uint32
			p   []byte
		}{{s1, p1}, {s2, p2}, {s3, p3}} {
			out, _ := s.Push(seg.seq, seg.p)
			total += len(out)
		}
		if total > len(p1)+len(p2)+len(p3) {
			t.Fatalf("emitted %d bytes from %d in", total, len(p1)+len(p2)+len(p3))
		}
	})
}
