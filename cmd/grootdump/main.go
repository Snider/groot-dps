// grootdump reads a Wireshark capture and shows what Aion 2 sent us.
//
//	grootdump testdata/dummy.pcapng                 # opcode histogram per flow
//	grootdump -list testdata/dummy.pcapng           # every packet, in order (loot hunting)
//	grootdump -op 0x3804 -hex testdata/dummy.pcapng # just damage, with hex
//	grootdump -damage testdata/dummy.pcapng         # decoded hits
package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcapgo"

	"groot/dps/tcp"
	"groot/dps/wire"
)

type packetReader interface {
	ReadPacketData() ([]byte, gopacket.CaptureInfo, error)
	LinkType() layers.LinkType
}

type flow struct {
	name     string
	stream   tcp.Stream
	split    wire.Splitter
	counts   map[uint16]int
	total    int
	gaps     int
	first    time.Time
	lastSeen time.Time
}

func main() {
	list := flag.Bool("list", false, "print every game packet in order")
	opFlag := flag.String("op", "", "only show this opcode, e.g. 0x3804")
	showHex := flag.Bool("hex", false, "include full hex for shown packets")
	damage := flag.Bool("damage", false, "decode and print damage packets")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: grootdump [-list] [-op 0x3804] [-hex] [-damage] capture.pcapng")
		os.Exit(2)
	}

	var only = -1
	if *opFlag != "" {
		v, err := strconv.ParseUint(*opFlag, 0, 16)
		if err != nil {
			fail(fmt.Errorf("bad -op %q: %w", *opFlag, err))
		}
		only = int(v)
	}

	r, closeFn, err := open(flag.Arg(0))
	if err != nil {
		fail(err)
	}
	defer closeFn()

	flows := map[string]*flow{}
	for {
		data, ci, err := r.ReadPacketData()
		if err == io.EOF {
			break
		}
		if err != nil {
			fail(err)
		}
		pk := gopacket.NewPacket(data, r.LinkType(), gopacket.DecodeOptions{Lazy: true, NoCopy: true})
		t, _ := pk.Layer(layers.LayerTypeTCP).(*layers.TCP)
		if t == nil || pk.NetworkLayer() == nil {
			continue
		}
		src, dst := pk.NetworkLayer().NetworkFlow().Endpoints()
		key := fmt.Sprintf("%v:%d > %v:%d", src, t.SrcPort, dst, t.DstPort)
		f := flows[key]
		if f == nil {
			f = &flow{name: key, counts: map[uint16]int{}, first: ci.Timestamp}
			flows[key] = f
		}
		f.lastSeen = ci.Timestamp

		bytes, gap := f.stream.Push(t.Seq, t.Payload)
		if gap {
			f.gaps++
			f.split = wire.Splitter{} // re-sync on the next keepalive
		}
		for _, raw := range f.split.Feed(bytes) {
			for _, p := range wire.Unpack(raw) {
				op, ok := wire.Opcode(p)
				if !ok {
					continue
				}
				f.counts[op]++
				f.total++
				if only >= 0 && int(op) != only {
					continue
				}
				if *list || only >= 0 {
					printPacket(ci.Timestamp, f, op, p, *showHex)
				}
				if *damage && op == wire.OpDamage {
					printDamage(ci.Timestamp, p)
				}
			}
		}
	}

	if *list || only >= 0 || *damage {
		return
	}
	summary(flows)
}

func open(path string) (packetReader, func(), error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	closeFn := func() { fh.Close() }
	if ng, err := pcapgo.NewNgReader(fh, pcapgo.DefaultNgReaderOptions); err == nil {
		return ng, closeFn, nil
	}
	if _, err := fh.Seek(0, io.SeekStart); err != nil {
		closeFn()
		return nil, nil, err
	}
	pc, err := pcapgo.NewReader(fh)
	if err != nil {
		closeFn()
		return nil, nil, fmt.Errorf("%s: not a pcap or pcapng file: %w", path, err)
	}
	return pc, closeFn, nil
}

func printPacket(ts time.Time, f *flow, op uint16, p []byte, full bool) {
	name := wire.OpName(op)
	if name == "" {
		name = "?"
	}
	shown := p
	if !full && len(shown) > 32 {
		shown = shown[:32]
	}
	fmt.Printf("%s +%7.3fs  0x%04x %-12s %5dB  %s\n",
		ts.Format("15:04:05.000"), ts.Sub(f.first).Seconds(), op, name, len(p), hex.EncodeToString(shown))
}

func printDamage(ts time.Time, p []byte) {
	d, err := wire.ParseDamage(p)
	if err != nil {
		fmt.Printf("%s  damage ?? %v  %s\n", ts.Format("15:04:05.000"), err, hex.EncodeToString(p))
		return
	}
	tags := ""
	for _, t := range []struct {
		on   bool
		name string
	}{{d.Crit(), "crit"}, {d.Miss(), "miss"}, {d.Resist(), "resist"}, {d.Back(), "back"}, {d.Front(), "front"}, {d.Parry(), "parry"}, {d.Perfect(), "perfect"}, {d.Double(), "double"}, {len(d.Absorbed) > 0, "shield"}} {
		if t.on {
			tags += " " + t.name
		}
	}
	if len(d.Extra) > 0 {
		tags += fmt.Sprintf(" +%v", d.Extra)
	}
	fmt.Printf("%s  %8d -> %-8d skill %-9d %10d%s\n", ts.Format("15:04:05.000"), d.Actor, d.Target, d.Skill, d.Amount, tags)
}

func summary(flows map[string]*flow) {
	var game []*flow
	for _, f := range flows {
		if f.total > 0 {
			game = append(game, f)
		}
	}
	if len(game) == 0 {
		fmt.Println("no Aion 2 packets found (no keepalive 0E 00 36 in any TCP flow)")
		return
	}
	sort.Slice(game, func(i, j int) bool { return game[i].total > game[j].total })
	for _, f := range game {
		fmt.Printf("%s  %d packets over %.1fs, %d gaps\n", f.name, f.total, f.lastSeen.Sub(f.first).Seconds(), f.gaps)
		ops := make([]uint16, 0, len(f.counts))
		for op := range f.counts {
			ops = append(ops, op)
		}
		sort.Slice(ops, func(i, j int) bool { return f.counts[ops[i]] > f.counts[ops[j]] })
		for _, op := range ops {
			name := wire.OpName(op)
			if name == "" {
				name = "?  (unknown — loot candidate?)"
			}
			fmt.Printf("  0x%04x  %6d  %s\n", op, f.counts[op], name)
		}
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "grootdump:", err)
	os.Exit(1)
}
