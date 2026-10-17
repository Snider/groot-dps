package wire

import "testing"

// Fuzz tests throw random bytes at the decoders. The only rule: never crash.
// `go test` runs the seeds; `go test -fuzz=FuzzUnpack ./wire` hunts for new crashes.

func FuzzSplitter(f *testing.F) {
	f.Add(cat(keepalive, pkt(OpDamage, 1, 2, 3)))
	f.Add([]byte{0x0E, 0x00, 0x36, 0xFF, 0xFF, 0xFF, 0xFF, 0x0F})
	f.Fuzz(func(t *testing.T, data []byte) {
		var s Splitter
		half := len(data) / 2
		for _, p := range append(s.Feed(data[:half]), s.Feed(data[half:])...) {
			if len(p) == 0 || len(p) > maxPacket {
				t.Fatalf("emitted a %d-byte packet", len(p))
			}
		}
	})
}

func FuzzUnpack(f *testing.F) {
	f.Add(pkt(OpDamage, 1, 2, 3))
	f.Add(container(pkt(OpDamage, filler(40)...), pkt(OpDeath, filler(40)...)))
	f.Add(frame([]byte{0xF1, 0xFF, 0xFF, 0x10, 0, 0, 0, 1, 2, 3}))
	f.Fuzz(func(t *testing.T, data []byte) {
		Unpack(data)
	})
}

func FuzzParseDamage(f *testing.F) {
	f.Add(pkt(OpDamage, damageBody(500, 0x05, 42, 11020100, 3, make([]byte, 11), 1234)...))
	f.Add(pkt(OpDamage, damageBody(7, 0x04, 8, 1000, 1, make([]byte, 8), 999)...))
	f.Fuzz(func(t *testing.T, data []byte) {
		ParseDamage(data)
	})
}
