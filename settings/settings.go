// SPDX-License-Identifier: EUPL-1.2

// Package settings is GrootDPS's own settings file, through CoreGO config:
// %AppData%\GrootDPS\settings.yaml on Windows (~/Library/Application
// Support/GrootDPS on macOS). Nothing in it leaves the PC.
//
// What wins, highest first: a flag on the command line, a GROOTDPS_*
// environment variable (GROOTDPS_HISTORY_KEEP_DAYS=7), the file, the default.
// Only what you set is written back, never the environment.
//
//	s := settings.Open(settings.DefaultPath()).Value.(*settings.Store)
//	v := s.Values()               // v.KeepDays == 30 until someone changes it
//	s.Set("history.keep_days", "7") // and it's in the file
package settings

import (
	"strconv"

	config "github.com/dappcore/config/go"
	core "github.com/dappcore/go"
)

// Values is every setting, resolved.
type Values struct {
	KeepDays   int    // history.keep_days: days of fights kept (0: don't save them)
	HistoryDir string // history.dir: where fights are saved
	Dots       string // meter.dots: JSON list of DoT skill ids ("" = next to the program)
	WindowX    int    // window.x: where the meter window opens
	WindowY    int    // window.y
}

// Keys are the settings `grootdps set` accepts, in display order.
var Keys = []string{"history.keep_days", "history.dir", "meter.dots", "window.x", "window.y"}

// Store is the open settings file.
type Store struct {
	cfg *config.Config
}

// Dir is GrootDPS's own folder: settings.yaml and fights/ live in it.
//
//	settings.Dir() // C:\Users\me\AppData\Roaming\GrootDPS
func Dir() string { return config.XDGWithPrefix("GrootDPS").Config() }

// DefaultPath is where the settings file lives.
func DefaultPath() string { return core.Path(Dir(), "settings.yaml") }

// Open reads the file at path (a missing file is fine: defaults apply).
//
//	r := settings.Open(settings.DefaultPath())
//	if !r.OK { core.Println(r.Error()) }
func Open(path string) core.Result {
	r := config.New(
		config.WithPath(path),
		config.WithEnvPrefix("GROOTDPS"),
		config.WithDefaults(map[string]any{
			"history.keep_days": 30,
			"history.dir":       core.Path(core.PathDir(path), "fights"),
			"meter.dots":        "",
			"window.x":          40,
			"window.y":          120,
		}),
	)
	if !r.OK {
		return r
	}
	return core.Ok(&Store{cfg: r.Value.(*config.Config)})
}

// Path is the file this Store reads and writes.
func (s *Store) Path() string { return s.cfg.Path() }

// Values resolves every setting.
func (s *Store) Values() Values {
	var v Values
	s.cfg.Get("history.keep_days", &v.KeepDays)
	s.cfg.Get("history.dir", &v.HistoryDir)
	s.cfg.Get("meter.dots", &v.Dots)
	s.cfg.Get("window.x", &v.WindowX)
	s.cfg.Get("window.y", &v.WindowY)
	return v
}

// Set checks and writes one setting, as typed on a command line.
//
//	s.Set("history.keep_days", "7")
//	s.Set("window.x", "-1200") // a monitor to the left
func (s *Store) Set(key, value string) core.Result {
	var v any = value
	switch key {
	case "history.keep_days":
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 {
			return core.Fail(core.E("settings.Set", "history.keep_days is a number of days, 0 or more", nil))
		}
		v = n
	case "window.x", "window.y":
		n, err := strconv.Atoi(value)
		if err != nil {
			return core.Fail(core.E("settings.Set", key+" is a whole number of pixels", nil))
		}
		v = n
	case "history.dir", "meter.dots":
	default:
		return core.Fail(core.E("settings.Set", "no setting called "+key+" (try: "+core.Join(", ", Keys...)+")", nil))
	}
	if r := s.cfg.Set(key, v); !r.OK {
		return r
	}
	return s.cfg.Commit()
}

// SaveWindow remembers where the window was, for next time.
//
//	x, y := win.Position(); s.SaveWindow(x, y)
func (s *Store) SaveWindow(x, y int) core.Result {
	s.cfg.Set("window.x", x)
	s.cfg.Set("window.y", y)
	return s.cfg.Commit()
}
