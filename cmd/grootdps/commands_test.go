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
