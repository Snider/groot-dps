// SPDX-License-Identifier: EUPL-1.2

// grootdps is the meter window: a small always-on-top overlay.
//
//	grootdps                                # live, from Npcap (Windows, where the game runs)
//	grootdps -replay testdata/fight.pcapng  # a capture, at its own pace (develop anywhere)
//	grootdps replay testdata/fight.pcapng   # the same, no window: fights print to the terminal
//	grootdps fights 5                       # the last 5 saved fights
//
// The DoT allowlist (a JSON list of skill ids) is read from -dots, or from
// dot_skill_ids.json next to the program. Without it DoT ticks don't count.
//
// The window talks to the meter over the app's own asset server (/api/...),
// inside the process: no port is opened and nothing goes over the network.
package main

import (
	"embed"
	"flag"

	core "github.com/dappcore/go"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"groot/dps/history"
	"groot/dps/live"
	"groot/dps/source"
)

//go:embed frontend
var frontend embed.FS

func main() {
	if len(core.Args()) > 1 && !core.IsFlag(core.Args()[1]) {
		core.AttachParentConsole() // before anything prints: the command's output needs somewhere to go
	}
	if c := commands(core.Stdout()); isCommand(c, core.Args()[1:]) {
		if r := c.Cli().Run(core.Args()[1:]...); !r.OK {
			fatal(r.Error())
		}
		return
	}
	prefs := openSettings()
	saved := prefs.Values() // flags default to the settings file; a flag given wins for this run
	replay := flag.String("replay", "", "play a .pcapng/.pcap instead of capturing live")
	fast := flag.Bool("fast", false, "with -replay: as fast as it reads, not at the capture's pace")
	dotsFile := flag.String("dots", saved.Dots, "JSON list of DoT skill ids whose damage ticks count")
	keepDays := flag.Int("keep-days", saved.KeepDays, "keep finished fights on disk this many days (0: don't save them)")
	historyDir := flag.String("history", saved.HistoryDir, "where finished fights are saved")
	x := flag.Int("x", saved.WindowX, "window position, from the screen's left")
	y := flag.Int("y", saved.WindowY, "window position, from the screen's top")
	flag.Parse()

	c := core.New(
		core.WithOption("name", "grootdps"),
		core.WithService(source.Register(source.Options{Dots: loadDots(*dotsFile), Replay: *replay, Fast: *fast})),
		core.WithService(history.Register(history.Options{Dir: *historyDir, KeepDays: *keepDays})), // after source: stops after its last flush
		core.WithServiceLock(),
	)
	if r := c.ServiceStartup(c.Context(), nil); !r.OK {
		fatal(r.Error())
	}
	src := core.MustServiceFor[*source.Service](c, "source")
	hist := core.MustServiceFor[*history.Service](c, "history")

	app := application.New(application.Options{
		Name:        "Groot DPS",
		Description: "A DPS meter for Aion 2 that only listens",
		Assets:      application.AssetOptions{Handler: api(src.Pipe, hist.Store, frontend)},
		Mac:         application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
		Windows:     application.WindowsOptions{},
	})
	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:           "Groot DPS",
		Width:           380,
		Height:          300,
		MinWidth:        260,
		MinHeight:       120,
		Frameless:       true,
		AlwaysOnTop:     true,
		BackgroundType:  application.BackgroundTypeTransparent,
		Mac:             application.MacWindow{Backdrop: application.MacBackdropTransparent},
		InitialPosition: application.WindowXY,
		X:               *x,
		Y:               *y,
		URL:             "/",
	})
	win.RegisterHook(events.Common.WindowClosing, func(*application.WindowEvent) {
		if x, y := win.Position(); x != saved.WindowX || y != saved.WindowY {
			if r := prefs.SaveWindow(x, y); !r.OK { // opens where you left it next time
				core.Warn("settings not saved", "err", r.Error())
			}
		}
	})
	err := app.Run()
	c.ServiceShutdown(core.Background()) // saves a fight still going when the window closed
	if err != nil {
		fatal(err)
	}
}

// api serves the window's files plus /api/meter (a snapshot), /api/reset and
// /api/history (opens the fights folder).
func api(pipe *live.Pipeline, store history.Store, files embed.FS) core.Handler {
	mux := &core.ServeMux{}
	var connected core.Once
	mux.HandleFunc("GET /api/meter", func(w core.ResponseWriter, r *core.Request) {
		connected.Do(func() { core.Info("window connected") })
		w.Header().Set("Content-Type", "application/json")
		core.JSONNewEncoder(w).Encode(pipe.Snapshot())
	})
	mux.HandleFunc("POST /api/reset", func(w core.ResponseWriter, r *core.Request) {
		pipe.Reset()
		w.WriteHeader(core.StatusNoContent)
	})
	mux.HandleFunc("POST /api/history", func(w core.ResponseWriter, r *core.Request) {
		if store.Dir == "" {
			core.HTTPError(w, "history is off", core.StatusNotFound)
			return
		}
		core.MkdirAll(store.Dir, 0o755)
		if err := application.Get().Browser.OpenFile(store.Dir); err != nil {
			core.HTTPError(w, err.Error(), core.StatusInternalServerError)
			return
		}
		w.WriteHeader(core.StatusNoContent)
	})
	mux.Handle("/", application.BundledAssetFileServer(mustSub(files, "frontend")))
	return mux
}

func loadDots(path string) map[uint32]bool {
	if path == "" {
		exe := core.Executable()
		if !exe.OK {
			return nil
		}
		path = core.PathJoin(core.PathDir(exe.Value.(string)), "dot_skill_ids.json")
		if !core.Stat(path).OK {
			core.Warn("no dot_skill_ids.json next to the program: DoT ticks won't count")
			return nil
		}
	}
	read := core.ReadFile(path)
	if !read.OK {
		core.Warn("dots unreadable", "path", path, "err", read.Err())
		return nil
	}
	var ids []uint32
	if r := core.JSONUnmarshal(read.Bytes(), &ids); !r.OK {
		core.Warn("dots unreadable", "path", path, "err", r.Err())
		return nil
	}
	dots := make(map[uint32]bool, len(ids))
	for _, id := range ids {
		dots[id] = true
	}
	return dots
}

// fatal reports what stopped GrootDPS from starting, and exits.
func fatal(v any) {
	core.Error(core.Sprintf("%v", v))
	core.Exit(1)
}
