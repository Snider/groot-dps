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
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"

	"groot/dps/capture"
	"groot/dps/live"
)

//go:embed frontend
var frontend embed.FS

func main() {
	replay := flag.String("replay", "", "play a .pcapng/.pcap instead of capturing live")
	fast := flag.Bool("fast", false, "with -replay: as fast as it reads, not at the capture's pace")
	dotsFile := flag.String("dots", "", "JSON list of DoT skill ids whose damage ticks count")
	x := flag.Int("x", 40, "window position, from the screen's left")
	y := flag.Int("y", 120, "window position, from the screen's top")
	flag.Parse()

	pipe := live.New(loadDots(*dotsFile))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		var err error
		if *replay != "" {
			err = capture.Replay(ctx, *replay, !*fast, pipe.Segment)
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
		Assets:      application.AssetOptions{Handler: api(pipe, frontend)},
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
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

// api serves the window's files plus /api/meter (a snapshot) and /api/reset.
func api(pipe *live.Pipeline, files embed.FS) http.Handler {
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
