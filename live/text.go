// SPDX-License-Identifier: EUPL-1.2

package live

import (
	core "github.com/dappcore/go"
)

// Text is a fight for a terminal: a heading, then one line per player, best first.
//
//	boss 2401 killed · 75.0s · 1,234,567 damage
//	  Grooty      Cleric    16,460/s  100.0%  crit 31%
func (v FightView) Text() string {
	b := core.NewBuilder()
	what := "fight"
	if v.Boss {
		what = "boss"
	}
	b.WriteString(core.Sprintf("%s %d", what, v.NPC))
	if v.Killed {
		b.WriteString(" killed")
	}
	b.WriteString(core.Sprintf(" · %.1fs · %s damage\n", v.Seconds, thousands(v.Total)))
	for _, r := range v.Rows {
		b.WriteString(core.Sprintf("  %-12s %-9s %9s/s  %5.1f%%  crit %2.0f%%", r.Name, r.Class, thousands(uint64(r.PerSec)), r.Share, r.CritRate))
		if r.Deaths > 0 {
			b.WriteString(core.Sprintf("  died %d", r.Deaths))
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// thousands: 1234567 → "1,234,567".
func thousands(n uint64) string {
	s := core.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
