// SPDX-License-Identifier: EUPL-1.2

package source_test

import (
	"os"
	"path/filepath"
	"testing"

	core "github.com/dappcore/go"

	"groot/dps/history"
	"groot/dps/live"
	"groot/dps/source"
)

// The whole meter, no window: a replay goes through the source, its fights
// cross the Core bus, and history saves them on shutdown at the latest.
func TestSource_Replay_Good_FightsReachHistory(t *testing.T) {
	dir := t.TempDir()
	var seen []live.FightFinished
	c := core.New(
		core.WithService(source.Register(source.Options{Replay: "../testdata/fake.pcap", Fast: true})),
		core.WithService(history.Register(history.Options{Dir: dir, KeepDays: 30})),
		core.WithServiceLock(),
	)
	core.On(c, func(f live.FightFinished) core.Result { seen = append(seen, f); return core.Ok(nil) })
	if r := c.ServiceStartup(core.Background(), nil); !r.OK {
		t.Fatal(r.Error())
	}
	<-core.MustServiceFor[*source.Service](c, "source").Done()
	if r := c.ServiceShutdown(core.Background()); !r.OK {
		t.Fatal(r.Error())
	}
	if len(seen) == 0 {
		t.Fatal("no FightFinished on the bus")
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*", "*.json"))
	if len(files) != len(seen) {
		t.Fatalf("saved %d files for %d fights", len(files), len(seen))
	}
	if src := core.MustServiceFor[*source.Service](c, "source"); src.Pipe.Snapshot().Packets == 0 {
		t.Fatal("no packets replayed")
	}
}

// A capture that can't be opened is logged, not fatal: the window still opens.
func TestSource_Replay_Bad_MissingFile(t *testing.T) {
	c := core.New(core.WithService(source.Register(source.Options{Replay: "no-such.pcap", Fast: true})))
	if r := c.ServiceStartup(core.Background(), nil); !r.OK {
		t.Fatal(r.Error())
	}
	if r := c.ServiceShutdown(core.Background()); !r.OK {
		t.Fatal(r.Error())
	}
}

// History off (KeepDays 0): fights still cross the bus, nothing hits the disk.
func TestSource_Replay_Ugly_HistoryOff(t *testing.T) {
	dir := t.TempDir()
	c := core.New(
		core.WithService(source.Register(source.Options{Replay: "../testdata/fake.pcap", Fast: true})),
		core.WithService(history.Register(history.Options{Dir: dir, KeepDays: 0})),
	)
	c.ServiceStartup(core.Background(), nil)
	<-core.MustServiceFor[*source.Service](c, "source").Done()
	c.ServiceShutdown(core.Background())
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("history off but %d entries written", len(entries))
	}
}
