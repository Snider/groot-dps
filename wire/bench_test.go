// SPDX-License-Identifier: EUPL-1.2

package wire

import (
	"encoding/hex"
	"testing"
)

// The decoder sits on the live packet path, so it gets a number: one real
// damage record (from the golden set) parsed per op.
func BenchmarkParseDamage(b *testing.B) {
	raw, _ := hex.DecodeString("e9de020600c80b9147ff003f02020002aff4b76301000000e4506a0100")
	p := pkt(OpDamage, raw...)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := ParseDamage(p); err != nil {
			b.Fatal(err)
		}
	}
}
