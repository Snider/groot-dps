// SPDX-License-Identifier: EUPL-1.2

// Package tcp puts one direction of a TCP connection back in order.
package tcp

import "bytes"

// maxPending is how many out-of-order segments we hold before deciding the
// missing one is gone for good (lost at capture, not retransmitted to us).
const maxPending = 64

// Stream reassembles one direction of a TCP flow by sequence number.
type Stream struct {
	next    uint32
	started bool
	pending map[uint32][]byte
}

// Push takes a segment and returns any bytes that are now in order.
// gap is true when we skipped missing data; the caller should re-sync
// whatever is parsing the bytes.
func (s *Stream) Push(seq uint32, payload []byte) (out []byte, gap bool) {
	if !s.started {
		s.next, s.started = seq, true
	}
	if len(payload) == 0 {
		return nil, false
	}
	if s.pending == nil {
		s.pending = map[uint32][]byte{}
	}
	if int32(seq-s.next) > 0 {
		s.pending[seq] = bytes.Clone(payload)
		if len(s.pending) <= maxPending {
			return nil, false
		}
		// Give up on the hole: jump to the oldest segment we hold.
		s.next, gap = s.oldest(), true
	} else {
		out = s.take(nil, seq, payload)
	}
	return s.drain(out), gap
}

// take appends the part of payload at or after next.
func (s *Stream) take(out []byte, seq uint32, payload []byte) []byte {
	behind := int(int32(s.next - seq))
	if behind >= len(payload) {
		return out // pure retransmit
	}
	out = append(out, payload[behind:]...)
	s.next += uint32(len(payload) - behind)
	return out
}

// drain releases held segments that are now contiguous.
func (s *Stream) drain(out []byte) []byte {
	for {
		progressed := false
		for seq, p := range s.pending {
			if int32(seq-s.next) <= 0 {
				out = s.take(out, seq, p)
				delete(s.pending, seq)
				progressed = true
			}
		}
		if !progressed {
			return out
		}
	}
}

func (s *Stream) oldest() uint32 {
	first := true
	var min uint32
	for seq := range s.pending {
		if first || int32(seq-min) < 0 {
			min, first = seq, false
		}
	}
	return min
}
