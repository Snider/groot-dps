// SPDX-License-Identifier: EUPL-1.2

package history

import (
	core "github.com/dappcore/go"

	"groot/dps/live"
)

// Options: where fights go and how long they stay. KeepDays 0 turns history off.
//
//	history.Options{Dir: `C:\Users\me\AppData\Roaming\GrootDPS\fights`, KeepDays: 30}
type Options struct {
	Dir      string
	KeepDays int
}

// Service saves every live.FightFinished on the bus, off the capture path
// (through a queue), and clears out old days at start and once a day.
//
//	core.WithService(history.Register(history.Options{Dir: dir, KeepDays: 30}))
type Service struct {
	*core.ServiceRuntime[Options]
	Store Store
	queue chan live.FightFinished
}

// Register builds the service and subscribes it to finished fights.
func Register(opts Options) func(*core.Core) core.Result {
	return func(c *core.Core) core.Result {
		s := &Service{ServiceRuntime: core.NewServiceRuntime(c, opts), queue: make(chan live.FightFinished, 64)}
		if opts.KeepDays > 0 {
			s.Store.Dir = opts.Dir
		}
		core.On(c, func(f live.FightFinished) core.Result {
			if s.Store.Dir == "" {
				return core.Ok(nil)
			}
			select {
			case s.queue <- f:
			default:
				core.Warn("history: queue full, a fight wasn't saved")
			}
			return core.Ok(nil)
		})
		return core.Ok(s)
	}
}

// OnStartup clears old days and starts the writer.
func (s *Service) OnStartup(ctx core.Context) core.Result {
	if s.Store.Dir == "" {
		return core.Ok(nil)
	}
	s.cleanup()
	s.Core().Go(func() {
		daily := core.NewTicker(24 * core.Hour)
		defer daily.Stop()
		for {
			select {
			case f := <-s.queue:
				s.save(f)
			case <-daily.C:
				s.cleanup()
			case <-ctx.Done():
				return
			}
		}
	})
	return core.Ok(nil)
}

// OnShutdown saves what's still queued, including the fight the source
// flushed as the meter closed (source stops first: it registers first).
func (s *Service) OnShutdown(core.Context) core.Result {
	for {
		select {
		case f := <-s.queue:
			s.save(f)
		default:
			return core.Ok(nil)
		}
	}
}

func (s *Service) save(f live.FightFinished) {
	if _, err := s.Store.Save(f.At, Label(f.Fight), f); err != nil {
		core.Warn("history: couldn't save a fight", "err", err)
	}
}

func (s *Service) cleanup() {
	if n, err := s.Store.Cleanup(core.Now(), s.Options().KeepDays); err != nil {
		core.Warn("history cleanup failed", "err", err)
	} else if n > 0 {
		core.Info("history: cleared old days", "days", n)
	}
}

// Label names a fight's file.
//
//	history.Label(v) // "boss 2401 killed"
func Label(v live.FightView) string {
	label := core.Sprintf("fight %d", v.NPC)
	if v.Boss {
		label = core.Sprintf("boss %d", v.NPC)
	}
	if v.Killed {
		label += " killed"
	}
	return label
}

// Saved is one fight read back from disk.
type Saved struct {
	Path string
	live.FightFinished
}

// Recent reads back the newest n saved fights, newest first. Files that
// aren't fights (or don't parse) are skipped, not fatal.
//
//	fights, err := history.Store{Dir: dir}.Recent(10)
func (s Store) Recent(n int) ([]Saved, error) {
	if s.Dir == "" || n <= 0 {
		return nil, nil
	}
	if !core.Stat(s.Dir).OK {
		return nil, nil
	}
	listed := core.ReadDir(core.DirFS(s.Dir), ".")
	if !listed.OK {
		return nil, listed.Err()
	}
	days := listed.Value.([]core.FsDirEntry)
	var out []Saved
	for i := len(days) - 1; i >= 0 && len(out) < n; i-- { // ReadDir sorts by name: day folders sort by date
		if !days[i].IsDir() || !core.TimeParse(dayLayout, days[i].Name()).OK {
			continue
		}
		day := core.PathJoin(s.Dir, days[i].Name())
		inDay := core.ReadDir(core.DirFS(day), ".")
		if !inDay.OK {
			return out, inDay.Err()
		}
		files := inDay.Value.([]core.FsDirEntry)
		for j := len(files) - 1; j >= 0 && len(out) < n; j-- {
			path := core.PathJoin(day, files[j].Name())
			if files[j].IsDir() || core.PathExt(path) != ".json" {
				continue
			}
			read := core.ReadFile(path)
			if !read.OK {
				continue
			}
			var f live.FightFinished
			if !core.JSONUnmarshal(read.Bytes(), &f).OK || f.At.IsZero() {
				continue
			}
			out = append(out, Saved{Path: path, FightFinished: f})
		}
	}
	return out, nil
}
