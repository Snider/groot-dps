// SPDX-License-Identifier: EUPL-1.2

package history

import (
	"fmt"
	"log"
	"time"

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
				log.Printf("history: queue full, a fight wasn't saved")
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
		daily := time.NewTicker(24 * time.Hour)
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
		log.Printf("history: %v", err)
	}
}

func (s *Service) cleanup() {
	if n, err := s.Store.Cleanup(time.Now(), s.Options().KeepDays); err != nil {
		log.Printf("history cleanup: %v", err)
	} else if n > 0 {
		log.Printf("history: cleared %d old days", n)
	}
}

// Label names a fight's file.
//
//	history.Label(v) // "boss 2401 killed"
func Label(v live.FightView) string {
	label := fmt.Sprintf("fight %d", v.NPC)
	if v.Boss {
		label = fmt.Sprintf("boss %d", v.NPC)
	}
	if v.Killed {
		label += " killed"
	}
	return label
}
