// SPDX-License-Identifier: EUPL-1.2

package wire

import (
	"bytes"
	"testing"
)

func TestOpcode(t *testing.T) {
	op, ok := Opcode(pkt(0x3804, 1, 2))
	if !ok || op != 0x3804 {
		t.Fatalf("Opcode = %#x, %v", op, ok)
	}
	if _, ok := Opcode([]byte{0x05}); ok {
		t.Fatal("Opcode on 1 byte should fail")
	}
}

func TestUnpackPlainPacketIsReturnedAsIs(t *testing.T) {
	p := pkt(0x3804, 1, 2, 3)
	got := Unpack(p)
	if len(got) != 1 || !bytes.Equal(got[0], p) {
		t.Fatalf("got %x", got)
	}
}

func filler(n int) []byte { return bytes.Repeat([]byte{0xAB}, n) }

func TestUnpackContainer(t *testing.T) {
	a := pkt(0x3804, filler(40)...)
	b := pkt(0x3603, filler(40)...)
	got := Unpack(container(a, b))
	if len(got) != 2 || !bytes.Equal(got[0], a) || !bytes.Equal(got[1], b) {
		t.Fatalf("got %d packets: %x", len(got), got)
	}
}

func TestUnpackNestedContainerKeepsOrder(t *testing.T) {
	a := pkt(0x3804, filler(40)...)
	b := pkt(0x3805, filler(40)...)
	c := pkt(0x3603, filler(40)...)
	got := Unpack(container(a, container(b, b), c))
	want := [][]byte{a, b, b, c}
	if len(got) != len(want) {
		t.Fatalf("got %d packets, want %d", len(got), len(want))
	}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Fatalf("packet %d = %x, want %x", i, got[i], want[i])
		}
	}
}

func TestUnpackSkipsZeroPaddingBetweenFrames(t *testing.T) {
	a := pkt(0x3804, filler(40)...)
	c := container(a, []byte{0, 0, 0}, a)
	got := Unpack(c)
	if len(got) != 2 {
		t.Fatalf("got %d packets, want 2", len(got))
	}
}

func TestUnpackContainerWithFlagByte(t *testing.T) {
	a := pkt(0x3804, filler(40)...)
	c := container(a)
	// Re-frame with a 0xF? flag byte in front of FF FF.
	_, n := uvarint(c)
	flagged := frame(append([]byte{0xF1}, c[n:]...))
	got := Unpack(flagged)
	if len(got) != 1 || !bytes.Equal(got[0], a) {
		t.Fatalf("got %x", got)
	}
}

func TestUnpackGarbageContainerDoesNotPanic(t *testing.T) {
	Unpack(frame([]byte{0xFF, 0xFF, 0x10, 0, 0, 0, 1, 2, 3}))
	Unpack(frame([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x7F, 1}))
	Unpack(frame([]byte{0xFF, 0xFF}))
}

func TestDecompressRespectsBudget(t *testing.T) {
	c := container(pkt(0x3804, filler(40)...))
	small := 10
	if _, ok := decompress(c, &small); ok {
		t.Fatal("decompressed past the budget")
	}
	if small != 10 {
		t.Fatalf("budget spent on a refused container: %d", small)
	}
	big := 1000
	if _, ok := decompress(c, &big); !ok || big >= 1000 {
		t.Fatalf("ok=%v budget=%d", ok, big)
	}
}
