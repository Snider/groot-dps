// SPDX-License-Identifier: EUPL-1.2

package meter

// What the meter shows: rows per fight, the fights worth showing.

import (
	"cmp"
	"slices"

	"groot/dps/wire"
)

type Row struct {
	Actor       uint32 // the owner, for summons
	Name        string // "" until an identity record names them
	Class       wire.Class
	CombatPower uint64 // from the party roster
	Self        bool
	InParty     bool
	Amount      uint64
	DPS, Share  float64 // Share in %
	Hits, Crits uint64
	Deaths      int        // during this fight
	Skills      []SkillRow // biggest first
}

type SkillRow struct {
	Skill               uint32
	Amount, Hits, Crits uint64
}

type HealRow struct {
	Actor     uint32
	Amount    uint64
	PerSecond float64
}

// Rows: damage on target per player (summons folded into owners), biggest first.
func (m *Meter) Rows(target uint32) []Row {
	f := m.fights[target]
	if f == nil {
		return nil
	}
	secs := float64(max(f.Duration(), minDivideMS)) / 1000
	byOwner := map[uint32]*Row{}
	skills := map[uint32]map[uint32]*SkillRow{}
	for id, a := range f.Actors {
		o := m.Owner(id)
		r := byOwner[o]
		if r == nil {
			r = &Row{Actor: o}
			m.describe(r)
			byOwner[o], skills[o] = r, map[uint32]*SkillRow{}
		}
		r.Amount, r.Hits, r.Crits = r.Amount+a.amount, r.Hits+a.hits, r.Crits+a.crits
		for k, s := range a.skills {
			t := skills[o][k]
			if t == nil {
				t = &SkillRow{Skill: k}
				skills[o][k] = t
			}
			t.Amount, t.Hits, t.Crits = t.Amount+s.Amount, t.Hits+s.Hits, t.Crits+s.Crits
		}
	}
	rows := make([]Row, 0, len(byOwner))
	for o, r := range byOwner {
		r.DPS = float64(r.Amount) / secs
		for _, d := range m.deaths {
			if d.entity == o && d.at >= f.Start && d.at <= max(f.End, f.KilledAt) {
				r.Deaths++
			}
		}
		if f.Total > 0 {
			r.Share = float64(r.Amount) / float64(f.Total) * 100
		}
		for _, s := range skills[o] {
			r.Skills = append(r.Skills, *s)
		}
		slices.SortFunc(r.Skills, func(a, b SkillRow) int { return cmp.Or(cmp.Compare(b.Amount, a.Amount), cmp.Compare(a.Skill, b.Skill)) })
		rows = append(rows, *r)
	}
	slices.SortFunc(rows, func(a, b Row) int { return cmp.Or(cmp.Compare(b.Amount, a.Amount), cmp.Compare(a.Actor, b.Actor)) })
	return rows
}

// HealRows: healing done during the fight on target, per healer, biggest first.
func (m *Meter) HealRows(target uint32) []HealRow {
	f := m.fights[target]
	if f == nil {
		return nil
	}
	secs := float64(max(f.Duration(), minDivideMS)) / 1000
	by := map[uint32]uint64{}
	for _, h := range m.heals {
		if h.at >= f.Start && h.at <= f.End {
			by[h.healer] += h.amount
		}
	}
	rows := make([]HealRow, 0, len(by))
	for id, amt := range by {
		rows = append(rows, HealRow{Actor: id, Amount: amt, PerSecond: float64(amt) / secs})
	}
	slices.SortFunc(rows, func(a, b HealRow) int { return cmp.Or(cmp.Compare(b.Amount, a.Amount), cmp.Compare(a.Actor, b.Actor)) })
	return rows
}

// Fights: every current fight long enough to count, bosses first, then biggest.
func (m *Meter) Fights() []*Fight {
	var out []*Fight
	for _, f := range m.fights {
		if f.Counts() {
			out = append(out, f)
		}
	}
	slices.SortFunc(out, func(a, b *Fight) int {
		return cmp.Or(boolCmp(b.Boss, a.Boss), cmp.Compare(b.Total, a.Total), cmp.Compare(a.Target, b.Target))
	})
	return out
}

func boolCmp(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	}
	return -1
}

// Current is the fight to show: your latest (anyone's until we know you), but
// a boss over the trash around it while the boss fight is live.
func (m *Meter) Current() *Fight {
	var cands []*Fight
	for _, f := range m.fights {
		if m.Ours(f) {
			cands = append(cands, f)
		}
	}
	if len(cands) == 0 {
		for _, f := range m.fights {
			cands = append(cands, f)
		}
	}
	var latest, boss *Fight
	for _, f := range cands {
		if latest == nil || f.End > latest.End || f.End == latest.End && f.Target < latest.Target {
			latest = f
		}
		if f.Boss && (boss == nil || f.End > boss.End) {
			boss = f
		}
	}
	if boss != nil && boss.End >= latest.End-IdleResetMS {
		return boss
	}
	return latest
}
