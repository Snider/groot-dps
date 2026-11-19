// SPDX-License-Identifier: EUPL-1.2

package live

import "testing"

func TestFightView_Text_Good(t *testing.T) {
	v := FightView{NPC: 2401, Boss: true, Killed: true, Seconds: 75, Total: 1234567,
		Rows: []RowView{{Name: "Grooty", Class: "Cleric", PerSec: 16460.4, Share: 100, CritRate: 31, Deaths: 1}}}
	want := "boss 2401 killed · 75.0s · 1,234,567 damage\n  Grooty       Cleric       16,460/s  100.0%  crit 31%  died 1\n"
	if got := v.Text(); got != want {
		t.Fatalf("got\n%q\nwant\n%q", got, want)
	}
}

func TestFightView_Text_Ugly_Empty(t *testing.T) {
	if got := (FightView{}).Text(); got != "fight 0 · 0.0s · 0 damage\n" {
		t.Fatalf("%q", got)
	}
}

func TestThousands(t *testing.T) {
	for n, want := range map[uint64]string{0: "0", 999: "999", 1000: "1,000", 1234567: "1,234,567"} {
		if got := thousands(n); got != want {
			t.Errorf("%d: %q", n, got)
		}
	}
}
