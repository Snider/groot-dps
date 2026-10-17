package wire

import (
	"bytes"
	"encoding/binary"
)

const (
	maxPacket = 40 * 1024        // anything bigger is a bad length, not a packet
	maxBuffer = 10 * 1024 * 1024 // runaway guard: drop everything and re-sync

	keepaliveLen = 11 // 0x0E → 14 + 1 - 4
)

// syncPattern starts the 11-byte keepalive. It's how we find a packet
// boundary when we join the stream mid-flow.
var syncPattern = []byte{0x0E, 0x00, 0x36}

func uvarint(b []byte) (uint64, int) { return binary.Uvarint(b) }

// Splitter turns reassembled TCP bytes into whole packets.
// Feed it in order; it keeps partial packets between calls.
type Splitter struct {
	buf    []byte
	synced bool
}

// Feed appends data and returns every complete packet now available.
// Keepalives are swallowed. Returned slices are copies.
func (s *Splitter) Feed(data []byte) [][]byte {
	if len(s.buf)+len(data) > maxBuffer {
		s.buf, s.synced = s.buf[:0], false
	}
	s.buf = append(s.buf, data...)

	var out [][]byte
	off := 0
	for off < len(s.buf) {
		rest := s.buf[off:]
		if !s.synced {
			i := bytes.Index(rest, syncPattern)
			if i < 0 {
				// Keep a tail in case the pattern straddles the next chunk.
				off = max(off, len(s.buf)-(len(syncPattern)-1))
				break
			}
			off += i
			rest = s.buf[off:]
			s.synced = true
		}

		v, n := uvarint(rest)
		if n == 0 {
			break // varint not complete yet
		}
		size := int(v) + n - 4
		if n < 0 || v > maxPacket || size <= 0 || size > maxPacket {
			s.synced = false
			off++
			continue
		}
		if len(rest) < size {
			break
		}
		p := rest[:size]
		if !(size == keepaliveLen && bytes.HasPrefix(p, syncPattern)) {
			out = append(out, bytes.Clone(p))
		}
		off += size
	}

	n := copy(s.buf, s.buf[off:])
	s.buf = s.buf[:n]
	return out
}
