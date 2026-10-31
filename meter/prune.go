// SPDX-License-Identifier: EUPL-1.2

package meter

// KeepMS: fights that ended longer ago than this are dropped by Prune.
const KeepMS = 10 * 60_000

// Prune forgets fights that ended over KeepMS before now (except the one on
// screen), and the heals and deaths older than the oldest fight kept. Call
// it now and then on a live meter so hours of play don't pile up.
func (m *Meter) Prune(now int64) {
	current := m.Current()
	oldest := now
	for id, f := range m.fights {
		if f != current && f.End < now-KeepMS {
			delete(m.fights, id)
			continue
		}
		oldest = min(oldest, f.Start)
	}
	m.heals = keepFrom(m.heals, func(h heal) bool { return h.at >= oldest })
	m.deaths = keepFrom(m.deaths, func(d death) bool { return d.at >= oldest })
}

// keepFrom drops the leading items (oldest first) that keep says no to.
func keepFrom[T any](s []T, keep func(T) bool) []T {
	i := 0
	for i < len(s) && !keep(s[i]) {
		i++
	}
	return append(s[:0], s[i:]...)
}
