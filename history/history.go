// SPDX-License-Identifier: EUPL-1.2

// Package history keeps finished fights on your own disk, one JSON file per
// fight in a folder per day, and clears out old days. Nothing leaves the PC.
package history

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const dayLayout = "2006-01-02"

// Store writes under Dir. An empty Dir means history is off.
type Store struct {
	Dir string
}

// Save writes v as <Dir>/<day>/<hh-mm-ss> <label>.json and returns the path.
func (s Store) Save(at time.Time, label string, v any) (string, error) {
	if s.Dir == "" {
		return "", nil
	}
	day := filepath.Join(s.Dir, at.Format(dayLayout))
	if err := os.MkdirAll(day, 0o755); err != nil {
		return "", err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	base := at.Format("15-04-05") + " " + tame(label)
	for n := 1; ; n++ {
		name := base + ".json"
		if n > 1 {
			name = fmt.Sprintf("%s (%d).json", base, n)
		}
		path := filepath.Join(day, name)
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		_, werr := f.Write(b)
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		return path, werr
	}
}

// Cleanup removes day folders more than keepDays old. Anything else in Dir is left alone.
func (s Store) Cleanup(now time.Time, keepDays int) (int, error) {
	if s.Dir == "" {
		return 0, nil
	}
	entries, err := os.ReadDir(s.Dir)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	y, m, d := now.Date()
	cutoff := time.Date(y, m, d, 0, 0, 0, 0, now.Location()).AddDate(0, 0, -keepDays)
	removed := 0
	for _, e := range entries {
		day, err := time.ParseInLocation(dayLayout, e.Name(), now.Location())
		if !e.IsDir() || err != nil || !day.Before(cutoff) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(s.Dir, e.Name())); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

// tame keeps a label to letters, digits, spaces and dashes, so a name from
// the game can't make a path.
func tame(s string) string {
	s = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == ' ', r == '-':
			return r
		}
		return -1
	}, s)
	if s = strings.TrimSpace(s); s == "" {
		s = "fight"
	}
	if len(s) > 40 {
		s = s[:40]
	}
	return s
}
