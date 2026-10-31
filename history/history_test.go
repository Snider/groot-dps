// SPDX-License-Identifier: EUPL-1.2

package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type fight struct {
	Boss  bool   `json:"boss"`
	Total uint64 `json:"total"`
}

func TestSaveWritesOneFilePerFightUnderItsDay(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	at := time.Date(2026, 10, 31, 20, 3, 15, 0, time.Local)
	path, err := s.Save(at, "boss 2310471", fight{Boss: true, Total: 2046574})
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(s.Dir, "2026-10-31", "20-03-15 boss 2310471.json"); path != want {
		t.Fatalf("path %s, want %s", path, want)
	}
	b, _ := os.ReadFile(path)
	var got fight
	if err := json.Unmarshal(b, &got); err != nil || got.Total != 2046574 {
		t.Fatalf("got %s (%v)", b, err)
	}
	// Two fights in the same second don't overwrite each other.
	p2, err := s.Save(at, "boss 2310471", fight{Total: 1})
	if err != nil || p2 == path {
		t.Fatalf("second save %s, %v", p2, err)
	}
}

// Names come from game data: nothing in one can climb out of the folder.
func TestSaveKeepsLabelsTame(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	path, err := s.Save(time.Now(), `../../evil\name:*?`, fight{})
	if err != nil {
		t.Fatal(err)
	}
	if rel, _ := filepath.Rel(s.Dir, path); filepath.Dir(filepath.Dir(rel)) != "." {
		t.Fatalf("escaped: %s", path)
	}
}

func TestCleanupDropsOldDays(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.Local)
	for _, d := range []int{0, 5, 29, 30, 31, 90} {
		if _, err := s.Save(now.AddDate(0, 0, -d), "fight", fight{}); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(s.Dir, "notes.txt"), []byte("mine"), 0o644) // not ours: left alone
	removed, err := s.Cleanup(now, 30)
	if err != nil || removed != 2 {
		t.Fatalf("removed %d, %v", removed, err)
	}
	days, _ := os.ReadDir(s.Dir)
	var names []string
	for _, d := range days {
		names = append(names, d.Name())
	}
	if len(names) != 5 { // 0, 5, 29, 30 days old + notes.txt
		t.Fatalf("left %v", names)
	}
}

func TestOffMeansNothingIsWritten(t *testing.T) {
	s := Store{} // no dir: history off
	if path, err := s.Save(time.Now(), "fight", fight{}); path != "" || err != nil {
		t.Fatalf("%s, %v", path, err)
	}
	if n, err := s.Cleanup(time.Now(), 30); n != 0 || err != nil {
		t.Fatalf("%d, %v", n, err)
	}
}
