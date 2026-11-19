// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"log"
	"os"
	"syscall"
)

// attachConsole: the release exe is built -H windowsgui (no console window
// for the meter), so a command run from cmd or PowerShell borrows the console
// it was started from; otherwise its output would go nowhere. The prompt comes
// back straight away, so output lands after it: a GUI-exe quirk, harmless.
func attachConsole() {
	const attachParentProcess = ^uintptr(0) // (DWORD)-1
	if r, _, _ := syscall.NewLazyDLL("kernel32.dll").NewProc("AttachConsole").Call(attachParentProcess); r == 0 {
		return // no parent console (double-clicked) or already has one (go run)
	}
	if f, err := os.OpenFile("CONOUT$", os.O_RDWR, 0); err == nil {
		os.Stdout, os.Stderr = f, f
		log.SetOutput(f)
	}
}
