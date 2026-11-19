// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"strings"
	"testing"
)

func run(t *testing.T, args ...string) (string, bool) {
	t.Helper()
	var out strings.Builder
	r := commands(&out).Cli().Run(args...)
	return out.String(), r.OK
}

// Replay with --save, then fights: the same boss comes back out of history.
func TestCommands_ReplayThenFights_Good(t *testing.T) {
	dir := "--history=" + t.TempDir()
	out, ok := run(t, "replay", "../../testdata/fake.pcap", "--save", dir)
	if !ok || !strings.Contains(out, "killed") || !strings.Contains(out, "Grooty") {
		t.Fatalf("replay: ok=%v\n%s", ok, out)
	}
	out, ok = run(t, "fights", "5", dir)
	if !ok || strings.Count(out, "killed") != 1 {
		t.Fatalf("fights: ok=%v\n%s", ok, out)
	}
}

func TestCommands_Replay_Bad(t *testing.T) {
	if _, ok := run(t, "replay"); ok {
		t.Fatal("replay with no file succeeded")
	}
	if _, ok := run(t, "replay", "no-such.pcap"); ok {
		t.Fatal("replay of a missing file succeeded")
	}
	if _, ok := run(t, "fights", "lots"); ok {
		t.Fatal("fights lots succeeded")
	}
}

// Without --save a replay leaves history alone; window flags aren't commands.
func TestCommands_Ugly(t *testing.T) {
	dir := "--history=" + t.TempDir()
	run(t, "replay", "../../testdata/fake.pcap", dir)
	if out, _ := run(t, "fights", dir); !strings.Contains(out, "no saved fights") {
		t.Fatalf("replay without --save wrote history:\n%s", out)
	}
	c := commands(&strings.Builder{})
	for _, args := range [][]string{{"-replay", "x.pcap"}, {}, {"nope"}} {
		if isCommand(c, args) {
			t.Fatalf("%q taken as a command: the window wouldn't open", args)
		}
	}
	if !isCommand(c, []string{"fights"}) {
		t.Fatal("fights not a command")
	}
}

// set writes the settings file, settings reads it back; bad input changes nothing.
func TestCommands_SetThenSettings_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("APPDATA", home) // Windows
	t.Setenv("XDG_CONFIG_HOME", home)
	if out, ok := run(t, "set", "history.keep_days", "7"); !ok || !strings.Contains(out, "history.keep_days  7") {
		t.Fatalf("set: ok=%v\n%s", ok, out)
	}
	if _, ok := run(t, "set", "history.keep_days", "lots"); ok {
		t.Fatal("set accepted lots")
	}
	if _, ok := run(t, "set", "history.keep_days"); ok {
		t.Fatal("set with no value succeeded")
	}
	if out, _ := run(t, "settings"); !strings.Contains(out, "history.keep_days  7") || !strings.Contains(out, home) {
		t.Fatalf("settings:\n%s", out)
	}
}
