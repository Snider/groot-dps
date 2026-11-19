// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"io"
	"log"
	"os"
	"strconv"

	core "github.com/dappcore/go"

	"groot/dps/history"
	"groot/dps/live"
	"groot/dps/settings"
	"groot/dps/source"
)

// commands is grootdps without the window, through Core's command tree:
//
//	grootdps replay fight.pcapng            # every fight in it, printed as it finishes
//	grootdps replay fight.pcapng --pace     # at the capture's own pace
//	grootdps replay fight.pcapng --save     # and save them to history
//	grootdps fights                         # the last 10 saved fights
//	grootdps fights 3 --history=D:\fights   # the last 3, from another folder
//	grootdps settings                       # what's set, and where the file is
//	grootdps set history.keep_days 7        # change one, for good
//
// --dots and --history override the settings file for one run.
func commands(out io.Writer) *core.Core {
	c := core.New(core.WithOption("name", "grootdps"), core.WithCli())
	c.Command("replay", core.Command{Description: "play a capture without the window", Action: func(o core.Options) core.Result {
		return replay(out, o)
	}})
	c.Command("fights", core.Command{Description: "list saved fights, newest first", Action: func(o core.Options) core.Result {
		return fights(out, o)
	}})
	c.Command("settings", core.Command{Description: "show your settings and where they're kept", Action: func(core.Options) core.Result {
		return showSettings(out, openSettings())
	}})
	c.Command("set", core.Command{Description: "change a setting: grootdps set history.keep_days 7", Action: func(o core.Options) core.Result {
		args := o.Args()
		if len(args) != 2 {
			return core.Fail(core.E("grootdps.set", "set <key> <value>, keys: "+core.Join(", ", settings.Keys...), nil))
		}
		s := openSettings()
		if r := s.Set(args[0], args[1]); !r.OK {
			return r
		}
		return showSettings(out, s)
	}})
	return c
}

// isCommand: does args start with a command (rather than window flags)?
//
//	isCommand(c, []string{"replay", "x.pcap"}) // true
//	isCommand(c, []string{"-replay", "x.pcap"}) // false: the window
func isCommand(c *core.Core, args []string) bool {
	return len(args) > 0 && !core.IsFlag(args[0]) && c.Command(args[0]).OK
}

func replay(out io.Writer, o core.Options) core.Result {
	file := o.String("_arg")
	if file == "" {
		return core.Fail(core.E("grootdps.replay", "which capture? grootdps replay <file.pcapng>", nil))
	}
	if _, err := os.Stat(file); err != nil {
		return core.Fail(core.E("grootdps.replay", "can't read "+file, err))
	}
	keep := 0
	if o.Bool("save") {
		keep = 30
	}
	n := 0
	c := core.New(
		core.WithService(source.Register(source.Options{Dots: loadDots(dotsFile(o)), Replay: file, Fast: !o.Bool("pace")})),
		core.WithService(history.Register(history.Options{Dir: historyDir(o), KeepDays: keep})),
		core.WithServiceLock(),
	)
	core.On(c, func(f live.FightFinished) core.Result {
		n++
		io.WriteString(out, f.Fight.Text()+"\n")
		return core.Ok(nil)
	})
	if r := c.ServiceStartup(core.Background(), nil); !r.OK {
		return r
	}
	<-core.MustServiceFor[*source.Service](c, "source").Done()
	if r := c.ServiceShutdown(core.Background()); !r.OK {
		return r
	}
	if n == 0 {
		io.WriteString(out, "no fights of yours in "+file+"\n")
	}
	return core.Ok(n)
}

func fights(out io.Writer, o core.Options) core.Result {
	n := 10
	if s := o.String("_arg"); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil || v <= 0 {
			return core.Fail(core.E("grootdps.fights", "how many? grootdps fights 10", nil))
		}
		n = v
	}
	dir := historyDir(o)
	saved, err := history.Store{Dir: dir}.Recent(n)
	if err != nil {
		return core.Fail(core.E("grootdps.fights", "reading "+dir, err))
	}
	if len(saved) == 0 {
		io.WriteString(out, "no saved fights in "+dir+"\n")
	}
	for _, s := range saved {
		io.WriteString(out, s.At.Local().Format("Mon 2 Jan 15:04")+" · "+s.Fight.Text()+"\n")
	}
	return core.Ok(len(saved))
}

func historyDir(o core.Options) string {
	if d := o.String("history"); d != "" {
		return d
	}
	return openSettings().Values().HistoryDir
}

func dotsFile(o core.Options) string {
	if d := o.String("dots"); d != "" {
		return d
	}
	return openSettings().Values().Dots
}

// openSettings opens the settings file. A broken one is reported and
// replaced by the defaults for this run, so the meter still opens.
func openSettings() *settings.Store {
	r := settings.Open(settings.DefaultPath())
	if !r.OK {
		log.Printf("settings: %s (using defaults)", r.Error())
		r = settings.Open(core.Path(core.TempDir(), "grootdps-defaults", "settings.yaml"))
	}
	return r.Value.(*settings.Store)
}

func showSettings(out io.Writer, s *settings.Store) core.Result {
	v := s.Values()
	io.WriteString(out, core.Sprintf(`%s
  history.keep_days  %d
  history.dir        %s
  meter.dots         %s
  window.x           %d
  window.y           %d
`, s.Path(), v.KeepDays, v.HistoryDir, orNext(v.Dots), v.WindowX, v.WindowY))
	return core.Ok(nil)
}

func orNext(dots string) string {
	if dots == "" {
		return "(dot_skill_ids.json next to the program)"
	}
	return dots
}
