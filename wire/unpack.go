package wire

import (
	"encoding/binary"

	"github.com/pierrec/lz4/v4"
)

const (
	maxNesting      = 8
	maxDecompressed = 10_000_000
)

// Opcode reads the 2-byte opcode after the length varint.
func Opcode(p []byte) (uint16, bool) {
	_, n := uvarint(p)
	if n <= 0 || len(p) < n+2 {
		return 0, false
	}
	return binary.LittleEndian.Uint16(p[n:]), true
}

// Unpack flattens a packet: a plain packet comes back as-is, an LZ4
// container comes back as the packets inside it (recursively, in order).
func Unpack(p []byte) [][]byte {
	if _, ok := containerHeader(p); !ok {
		return [][]byte{p}
	}
	var out [][]byte
	walk(p, &out, 0)
	return out
}

// walk scans buf as a run of frames, opening containers as it goes.
func walk(buf []byte, out *[][]byte, depth int) {
	for pos := 0; pos < len(buf); {
		if buf[pos] == 0x00 { // padding between frames
			pos++
			continue
		}
		v, n := uvarint(buf[pos:])
		if n <= 0 || v > maxDecompressed {
			return
		}
		size := int(v) + n - 4
		if size <= 0 {
			pos++
			continue
		}
		end := pos + size
		if end > len(buf) {
			return
		}
		f := buf[pos:end]
		if inner, ok := decompress(f); ok && depth < maxNesting {
			walk(inner, out, depth+1)
		} else if size > n {
			*out = append(*out, f)
		}
		pos = end
	}
}

// containerHeader returns the offset of FF FF in a container frame,
// skipping an optional 0xF? flag byte after the length.
func containerHeader(f []byte) (int, bool) {
	_, n := uvarint(f)
	if n <= 0 {
		return 0, false
	}
	h := n
	if h < len(f) && f[h]&0xF0 == 0xF0 && f[h] != 0xFF {
		h++
	}
	if len(f) < h+2 || f[h] != 0xFF || f[h+1] != 0xFF {
		return 0, false
	}
	return h, true
}

// decompress opens a container frame: FF FF, u32 LE size, LZ4 block.
func decompress(f []byte) ([]byte, bool) {
	h, ok := containerHeader(f)
	if !ok || len(f) < h+6 {
		return nil, false
	}
	size := binary.LittleEndian.Uint32(f[h+2:])
	if size == 0 || size > maxDecompressed {
		return nil, false
	}
	out := make([]byte, size)
	k, err := lz4.UncompressBlock(f[h+6:], out)
	if err != nil || k <= 0 {
		return nil, false
	}
	return out[:k], true
}
