// SPDX-License-Identifier: EUPL-1.2

// Package source is the Core service that feeds the meter: it runs the
// capture (live from Npcap, or a replay file) into a live.Pipeline and puts
// every finished fight on the bus as a live.FightFinished.
//
// Passive only: it reads a copy of the traffic and never touches the client.
//
//	c := core.New(core.WithService(source.Register(source.Options{Replay: "testdata/fight.pcapng"})))
//	src, _ := core.ServiceFor[*source.Service](c, "source")
//	snap := src.Pipe.Snapshot()
package source

import (
	core "github.com/dappcore/go"

	"groot/dps/capture"
	"groot/dps/live"
)

// Options picks where segments come from.
//
//	source.Options{}                                        // live, from Npcap
//	source.Options{Replay: "fight.pcapng", Fast: true}      // a capture, as fast as it reads
type Options struct {
	Dots   map[uint32]bool // DoT skill ids whose ticks count
	Replay string          // a .pcapng/.pcap to play instead of capturing live
	Fast   bool            // with Replay: don't wait for the capture's own pace
}

// Service owns the pipeline. Pipe is safe to read from any goroutine.
type Service struct {
	*core.ServiceRuntime[Options]
	Pipe *live.Pipeline
	done chan struct{} // closed when the capture goroutine has stopped; nil until OnStartup
}

// Register builds the service; capture starts on OnStartup.
//
//	core.WithService(source.Register(source.Options{Dots: dots}))
func Register(opts Options) func(*core.Core) core.Result {
	return func(c *core.Core) core.Result {
		s := &Service{ServiceRuntime: core.NewServiceRuntime(c, opts), Pipe: live.New(opts.Dots)}
		s.Pipe.OnFinished = func(at core.Time, v live.FightView) { // before capture starts: no race
			c.ACTION(live.FightFinished{At: at, Fight: v})
		}
		return core.Ok(s)
	}
}

// OnStartup starts the capture; it stops when Core shuts down.
func (s *Service) OnStartup(ctx core.Context) core.Result {
	opts := s.Options()
	s.done = make(chan struct{})
	s.Core().Go(func() {
		defer close(s.done)
		var err error
		if opts.Replay != "" {
			err = capture.Replay(ctx, opts.Replay, !opts.Fast, s.Pipe.Segment)
			s.Pipe.Flush(core.Now()) // the file's over: so are its fights
			core.Info("replay finished", "packets", s.Pipe.Snapshot().Packets)
		} else {
			err = capture.Live(ctx, s.Pipe.Segment, func(format string, args ...any) { core.Info(core.Sprintf(format, args...)) })
		}
		if err != nil && ctx.Err() == nil && !core.Is(err, core.ContextCanceled) {
			core.Error("capture stopped", "err", err)
		}
	})
	return core.Ok(nil)
}

// Done is closed once the capture has stopped: a replay reached its end, or
// Core shut down. Nil before OnStartup.
//
//	<-src.Done() // the replay's fights are all on the bus
func (s *Service) Done() <-chan struct{} { return s.done }

// OnShutdown hands over a fight still going when the meter closed.
func (s *Service) OnShutdown(core.Context) core.Result {
	if s.done != nil {
		<-s.done
	}
	s.Pipe.Flush(core.Now())
	return core.Ok(nil)
}
