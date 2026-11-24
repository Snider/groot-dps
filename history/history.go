// SPDX-License-Identifier: EUPL-1.2

// Package history keeps finished fights on your own disk, one JSON file per
// fight in a folder per day, and clears out old days. Nothing leaves the PC.
package history

import (
	core "github.com/dappcore/go"
)

const dayLayout = "2006-01-02"

// Store writes under Dir. An empty Dir means history is off.
type Store struct {
	Dir string
}

// Save writes v as <Dir>/<day>/<hh-mm-ss> <label>.json and returns the path.
func (s Store) Save(at core.Time, label string, v any) (string, error) {
	if s.Dir == "" {
		return "", nil
	}
	day := core.PathJoin(s.Dir, at.Format(dayLayout))
	if r := core.MkdirAll(day, 0o755); !r.OK {
		return "", r.Err()
	}
	encoded := core.JSONMarshalIndent(v, "", "  ")
	if !encoded.OK {
		return "", encoded.Err()
	}
	b := encoded.Bytes()
	base := at.Format("15-04-05") + " " + tame(label)
	for n := 1; ; n++ {
		name := base + ".json"
		if n > 1 {
			name = core.Sprintf("%s (%d).json", base, n)
		}
		path := core.PathJoin(day, name)
		opened := core.OpenFile(path, core.O_WRONLY|core.O_CREATE|core.O_EXCL, 0o644)
		if !opened.OK && core.IsExist(opened.Err()) {
			continue
		}
		if !opened.OK {
			return "", opened.Err()
		}
		f := opened.Value.(*core.OSFile)
		_, werr := f.Write(b)
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		return path, werr
	}
}

// Cleanup removes day folders more than keepDays old. Anything else in Dir is left alone.
func (s Store) Cleanup(now core.Time, keepDays int) (int, error) {
	if s.Dir == "" {
		return 0, nil
	}
	if !core.Stat(s.Dir).OK {
		return 0, nil
	}
	listed := core.ReadDir(core.DirFS(s.Dir), ".")
	if !listed.OK {
		return 0, listed.Err()
	}
	entries := listed.Value.([]core.FsDirEntry)
	// Deletes go through a core Fs rooted at the history folder: it can't
	// reach outside it, and refuses a home or drive root on every OS.
	fsys := (&core.Fs{}).New(s.Dir)
	y, m, d := now.Date()
	cutoff := core.Date(y, m, d, 0, 0, 0, 0, now.Location()).AddDate(0, 0, -keepDays)
	removed := 0
	for _, e := range entries {
		day := core.TimeParseInLocation(dayLayout, e.Name(), now.Location())
		if !e.IsDir() || !day.OK || !day.Value.(core.Time).Before(cutoff) {
			continue
		}
		if r := fsys.DeleteAll(e.Name()); !r.OK {
			return removed, r.Err()
		}
		removed++
	}
	return removed, nil
}

// tame keeps a label to letters, digits, spaces and dashes, so a name from
// the game can't make a path.
func tame(s string) string {
	s = core.MapRunes(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == ' ', r == '-':
			return r
		}
		return -1
	}, s)
	if s = core.Trim(s); s == "" {
		s = "fight"
	}
	if len(s) > 40 {
		s = s[:40]
	}
	return s
}
