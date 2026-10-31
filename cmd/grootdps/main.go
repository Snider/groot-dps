// SPDX-License-Identifier: EUPL-1.2

// grootdps is the meter window: a small always-on-top overlay.
//
//	grootdps                                # live, from Npcap (Windows, where the game runs)
//	grootdps -replay testdata/fight.pcapng  # a capture, at its own pace (develop anywhere)
//
// The DoT allowlist (a JSON list of skill ids) is read from -dots, or from
// dot_skill_ids.json next to the program. Without it DoT ticks don't count.
//
// The window talks to the meter over the app's own asset server (/api/...),
// inside the process: no port is opened and nothing goes over the network.
package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"groot/dps/capture"
	"groot/dps/history"
	"groot/dps/live"
)

//go:embed frontend
var frontend embed.FS

func main() {
	replay := flag.String("replay", "", "play a .pcapng/.pcap instead of capturing live")
	fast := flag.Bool("fast", false, "with -replay: as fast as it reads, not at the capture's pace")
	dotsFile := flag.String("dots", "", "JSON list of DoT skill ids whose damage ticks count")
	keepDays := flag.Int("keep-days", 30, "keep finished fights on disk this many days (0: don't save them)")
	historyDir := flag.String("history", defaultHistory(), "where finished fights are saved")
	x := flag.Int("x", 40, "window position, from the screen's left")
	y := flag.Int("y", 120, "window position, from the screen's top")
	flag.Parse()

	pipe := live.New(loadDots(*dotsFile))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store := history.Store{}
	if *keepDays > 0 {
		store.Dir = *historyDir
		queue := make(chan record, 64)
		pipe.OnFinished = func(at time.Time, v live.FightView) { // before capture starts: no race
			select {
			case queue <- record{at, v}:
			default:
				log.Printf("history: queue full, a fight wasn't saved")
			}
		}
		go keepHistory(ctx, queue, store, *keepDays)
	}
	go func() {
		var err error
		if *replay != "" {
			err = capture.Replay(ctx, *replay, !*fast, pipe.Segment)
			pipe.Flush(time.Now()) // the file's over: so are its fights
			log.Printf("replay finished: %d packets", pipe.Snapshot().Packets)
		} else {
			err = capture.Live(ctx, pipe.Segment, log.Printf)
		}
		if err != nil {
			log.Printf("capture: %v", err)
		}
	}()

	app := application.New(application.Options{
		Name:        "Groot DPS",
		Description: "A DPS meter for Aion 2 that only listens",
		Assets:      application.AssetOptions{Handler: api(pipe, store, frontend)},
		Mac:         application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
		Windows:     application.WindowsOptions{},
	})
	app.Window.NewWithOptions(application.WebviewWindowOptions{
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
	err := app.Run()
	pipe.Flush(time.Now())             // a fight still going when the window closed
	time.Sleep(200 * time.Millisecond) // let the writer finish it
	if err != nil {
		log.Fatal(err)
	}
}

// api serves the window's files plus /api/meter (a snapshot), /api/reset and
// /api/history (opens the fights folder).
func api(pipe *live.Pipeline, store history.Store, files embed.FS) http.Handler {
	mux := http.NewServeMux()
	var connected sync.Once
	mux.HandleFunc("GET /api/meter", func(w http.ResponseWriter, r *http.Request) {
		connected.Do(func() { log.Printf("window connected") })
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(pipe.Snapshot())
	})
	mux.HandleFunc("POST /api/reset", func(w http.ResponseWriter, r *http.Request) {
		pipe.Reset()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/history", func(w http.ResponseWriter, r *http.Request) {
		if store.Dir == "" {
			http.Error(w, "history is off", http.StatusNotFound)
			return
		}
		os.MkdirAll(store.Dir, 0o755)
		if err := application.Get().Browser.OpenFile(store.Dir); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.Handle("/", application.BundledAssetFileServer(mustSub(files, "frontend")))
	return mux
}

func loadDots(path string) map[uint32]bool {
	if path == "" {
		exe, err := os.Executable()
		if err != nil {
			return nil
		}
		path = filepath.Join(filepath.Dir(exe), "dot_skill_ids.json")
		if _, err := os.Stat(path); err != nil {
			log.Printf("no dot_skill_ids.json next to the program: DoT ticks won't count")
			return nil
		}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		log.Printf("dots: %v", err)
		return nil
	}
	var ids []uint32
	if err := json.Unmarshal(b, &ids); err != nil {
		log.Printf("dots %s: %v", path, err)
		return nil
	}
	dots := make(map[uint32]bool, len(ids))
	for _, id := range ids {
		dots[id] = true
	}
	return dots
}

// defaultHistory: %AppData%\GrootDPS\fights on Windows (the OS's per-user config folder elsewhere).
func defaultHistory() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "GrootDPS", "fights")
}

// record is what a saved fight file holds.
type record struct {
	At    time.Time      `json:"at"`
	Fight live.FightView `json:"fight"`
}

func (r record) label() string {
	label := fmt.Sprintf("fight %d", r.Fight.NPC)
	if r.Fight.Boss {
		label = fmt.Sprintf("boss %d", r.Fight.NPC)
	}
	if r.Fight.Killed {
		label += " killed"
	}
	return label
}

// keepHistory saves finished fights (off the capture path, through a queue)
// and clears out old days at start and once a day.
func keepHistory(ctx context.Context, queue <-chan record, store history.Store, keepDays int) {
	save := func(r record) {
		if _, err := store.Save(r.At, r.label(), r); err != nil {
			log.Printf("history: %v", err)
		}
	}
	clean := func() {
		if n, err := store.Cleanup(time.Now(), keepDays); err != nil {
			log.Printf("history cleanup: %v", err)
		} else if n > 0 {
			log.Printf("history: cleared %d old days", n)
		}
	}
	clean()
	daily := time.NewTicker(24 * time.Hour)
	defer daily.Stop()
	for {
		select {
		case r := <-queue:
			save(r)
		case <-daily.C:
			clean()
		case <-ctx.Done():
			for { // drain what's queued, then stop
				select {
				case r := <-queue:
					save(r)
				default:
					return
				}
			}
		}
	}
}
