// SPDX-License-Identifier: EUPL-1.2

package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func open(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "settings.yaml")
	r := Open(path)
	if !r.OK {
		t.Fatal(r.Error())
	}
	return r.Value.(*Store), path
}

// No file yet: the defaults, with fights beside the settings file.
func TestSettings_Values_Good_Defaults(t *testing.T) {
	s, path := open(t)
	v := s.Values()
	if v.KeepDays != 30 || v.WindowX != 40 || v.WindowY != 120 || v.Dots != "" {
		t.Fatalf("%+v", v)
	}
	if v.HistoryDir != filepath.Join(filepath.Dir(path), "fights") {
		t.Fatalf("history dir %q", v.HistoryDir)
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("reading settings wrote a file")
	}
}

// Set writes the file; a fresh Open reads it back. The window remembers too.
func TestSettings_Set_Good_RoundTrip(t *testing.T) {
	s, path := open(t)
	if r := s.Set("history.keep_days", "7"); !r.OK {
		t.Fatal(r.Error())
	}
	if r := s.SaveWindow(-1200, 80); !r.OK {
		t.Fatal(r.Error())
	}
	v := Open(path).Value.(*Store).Values()
	if v.KeepDays != 7 || v.WindowX != -1200 || v.WindowY != 80 {
		t.Fatalf("%+v", v)
	}
}

func TestSettings_Set_Bad(t *testing.T) {
	s, path := open(t)
	for _, kv := range [][2]string{{"history.keep_days", "lots"}, {"history.keep_days", "-1"}, {"window.x", "left"}, {"colour", "green"}} {
		if r := s.Set(kv[0], kv[1]); r.OK {
			t.Fatalf("%s=%s accepted", kv[0], kv[1])
		}
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("a refused setting still wrote the file")
	}
}

// The environment beats the file, and is never written into it.
func TestSettings_Values_Ugly_EnvOverridesNotSaved(t *testing.T) {
	s, path := open(t)
	s.Set("history.keep_days", "7")
	t.Setenv("GROOTDPS_HISTORY_KEEP_DAYS", "2")
	s2 := Open(path).Value.(*Store)
	if got := s2.Values().KeepDays; got != 2 {
		t.Fatalf("env didn't win: %d", got)
	}
	s2.SaveWindow(1, 2)
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "keep_days: 7") {
		t.Fatalf("env leaked into the file:\n%s", b)
	}
}
