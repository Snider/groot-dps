// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"io"
	"os"
	"strconv"

	core "github.com/dappcore/go"

	"groot/dps/history"
	"groot/dps/live"
	"groot/dps/source"
)

// commands is grootdps without the window, through Core's command tree:
//
//	grootdps replay fight.pcapng            # every fight in it, printed as it finishes
//	grootdps replay fight.pcapng --pace     # at the capture's own pace
//	grootdps replay fight.pcapng --save     # and save them to history
//	grootdps fights                         # the last 10 saved fights
//	grootdps fights 3 --history=D:\fights   # the last 3, from another folder
//
// Every command also takes --dots=<file> (replay) and --history=<dir>.
func commands(out io.Writer) *core.Core {
	c := core.New(core.WithOption("name", "grootdps"), core.WithCli())
	c.Command("replay", core.Command{Description: "play a capture without the window", Action: func(o core.Options) core.Result {
		return replay(out, o)
	}})
	c.Command("fights", core.Command{Description: "list saved fights, newest first", Action: func(o core.Options) core.Result {
		return fights(out, o)
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
		core.WithService(source.Register(source.Options{Dots: loadDots(o.String("dots")), Replay: file, Fast: !o.Bool("pace")})),
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
	return defaultHistory()
}
