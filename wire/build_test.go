package wire

import (
	"encoding/binary"

	"github.com/pierrec/lz4/v4"
)

// pkt builds a framed packet: varint length (size + 4 - lenBytes), opcode LE, body.
func pkt(op uint16, body ...byte) []byte {
	inner := binary.LittleEndian.AppendUint16(nil, op)
	inner = append(inner, body...)
	return frame(inner)
}

// frame prefixes payload with the game's length varint.
func frame(payload []byte) []byte {
	// size = v + n - 4, size = n + len(payload)  =>  v = len(payload) + 4
	return append(binary.AppendUvarint(nil, uint64(len(payload)+4)), payload...)
}

// container wraps inner frames as an FF FF LZ4 block.
func container(frames ...[]byte) []byte {
	var raw []byte
	for _, f := range frames {
		raw = append(raw, f...)
	}
	comp := make([]byte, lz4.CompressBlockBound(len(raw)))
	n, err := lz4.CompressBlock(raw, comp, nil)
	if err != nil || n == 0 {
		panic("lz4 compress failed (input too small to compress?)")
	}
	body := []byte{0xFF, 0xFF}
	body = binary.LittleEndian.AppendUint32(body, uint32(len(raw)))
	body = append(body, comp[:n]...)
	return frame(body)
}

func uv(v uint64) []byte { return binary.AppendUvarint(nil, v) }

func cat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

var keepalive = []byte{0x0E, 0x00, 0x36, 0, 0, 0, 0, 0, 0, 0, 0}
