// SPDX-License-Identifier: EUPL-1.2

package live

import core "github.com/dappcore/go"

// FightFinished goes out on the Core bus once for each fight of yours that's
// over (killed, quiet for 30s, or still going when the meter closes).
//
//	core.On(c, func(f live.FightFinished) core.Result { save(f.At, f.Fight); return core.Ok(nil) })
type FightFinished struct {
	At    core.Time `json:"at"`
	Fight FightView `json:"fight"`
}
