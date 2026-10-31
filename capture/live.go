// SPDX-License-Identifier: EUPL-1.2

package capture

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/gopacket/pcap"
)

// Live listens on every adapter (the game's traffic is on one of them; the
// splitter only syncs on game frames, so the rest add nothing). Needs Npcap
// on Windows. It only reads: the handles never send.
func Live(ctx context.Context, h Handler, logf func(string, ...any)) error {
	devs, err := pcap.FindAllDevs()
	if err != nil {
		return fmt.Errorf("no capture driver (install Npcap): %w", err)
	}
	var dir Direction // inbound = to one of this PC's addresses
	for _, d := range devs {
		for _, a := range d.Addresses {
			dir.Local = append(dir.Local, a.IP)
		}
	}
	var mu sync.Mutex // h isn't asked to be safe for concurrent use
	var wg sync.WaitGroup
	opened := 0
	for _, d := range devs {
		hd, err := pcap.OpenLive(d.Name, 65535, false, 250*time.Millisecond)
		if err != nil {
			logf("skip %s: %v", d.Description, err)
			continue
		}
		if err := hd.SetBPFFilter("tcp"); err != nil {
			logf("skip %s: %v", d.Description, err)
			hd.Close()
			continue
		}
		opened++
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer hd.Close()
			lt := hd.LinkType()
			for ctx.Err() == nil {
				data, ci, err := hd.ReadPacketData()
				if errors.Is(err, pcap.NextErrorTimeoutExpired) {
					continue
				}
				if err != nil {
					logf("%s: %v", d.Description, err)
					return
				}
				mu.Lock()
				Frame(data, lt, ci.Timestamp, dir, h)
				mu.Unlock()
			}
		}()
	}
	if opened == 0 {
		return errors.New("couldn't open any adapter (Npcap installed? run as admin if it was installed admin-only)")
	}
	logf("listening on %d adapters", opened)
	wg.Wait()
	return nil
}
