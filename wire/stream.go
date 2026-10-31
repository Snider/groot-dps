// SPDX-License-Identifier: EUPL-1.2

package wire

import (
	"bytes"
	"encoding/binary"
)

const (
	maxPacket       = 65535            // anything bigger is a bad length, not a packet
	maxFragmentWait = 16384            // a longer length still arriving is garbage, unless it's a bundle
	maxBundleClaim  = 1_000_000        // a bundle header claiming more than this isn't one
	maxBuffer       = 10 * 1024 * 1024 // runaway guard: drop everything and re-sync

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

		if rest[0] == 0x00 { // padding
			off++
			continue
		}
		if t, ok := tlsRecord(rest); ok {
			if len(rest) < t {
				break // the rest of it is still to come
			}
			off += t
			continue
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
			if size > maxFragmentWait && !bundleHeader(rest[n:]) {
				s.synced = false
				off++
				continue
			}
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

// tlsRecord: the game server also talks TLS from its port, on a second
// connection. Content type 20-23, version 3.1-3.4, big-endian length ≤ 2^14+256.
// Returns the record's size, header included.
func tlsRecord(b []byte) (int, bool) {
	if len(b) < 5 || b[0] < 0x14 || b[0] > 0x17 || b[1] != 0x03 || b[2] < 0x01 || b[2] > 0x04 {
		return 0, false
	}
	n := int(binary.BigEndian.Uint16(b[3:5]))
	return 5 + n, n > 0 && n <= 16384+256
}

// bundleHeader: FF FF and a believable decompressed size.
func bundleHeader(b []byte) bool {
	if len(b) < 6 || b[0] != 0xFF || b[1] != 0xFF {
		return false
	}
	size := binary.LittleEndian.Uint32(b[2:6])
	return size > 0 && size <= maxBundleClaim
}
