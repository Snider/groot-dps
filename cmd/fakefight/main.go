// SPDX-License-Identifier: EUPL-1.2

// fakefight writes a capture of a made-up boss fight, for working on the
// window without the game: go run ./cmd/fakefight testdata/fake.pcap
// Every name in it is made up; the spawn and self record are golden records.
package main

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/rand/v2"
	"net"
	"os"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcapgo"
)

const (
	selfRec = "3336ed745e91c128370647726f6f747918051e000000011c0000007f0100007f0100001c000000d002040000000000" // Grooty, Cleric, entity 14957
	bossRec = "4136fbaa030c220047412300000200d89f47001ca2c6004c38460000874300c001c0cf24c0cf24640000006400000000000000000000000000000000000000000000000000000001000000000000000000000000000000000000000604110181969800ffffffffffffffff8075d52abb030000fbaa03013000d89f47001ca2c6004c3846110284969800ffffffffffffffff8075d52abb030000fbaa030100d89f47001ca2c6004c38461103bc060000ffffffffffffffff8075d52abb030000fbaa030100d89f47001ca2c6004c38461106d6010000ffffffffffffffff8075d52abb030000fbaa030100d89f47001ca2c6004c384601002d00000003012800000028000000437253c900"
	// The golden roster (names scrubbed): Rootsy, Barky, Twiggy, Mossie, Acornboi.
	rosterRec = "029771931a001d416c6c20626f7373657320616c742072756e204c46206368616e74657205cc2709000003f0b3030000000009ff0202051e01f0b303000000000906526f6f747379200000002d000000db0500000009cd1004be1201000000000000300000000000000001051e02e9b7020000000209054261726b790c0000002d000000f30600000209cd1004f85201000000000000790000000000000001051e03e212040000000009065477696767791c0000002d000000cc0500000009cd1004210001000000000000250000000000000001051e04efd1020000000409064d6f737369651c0000002d0000001d08000007fe08cd1004e85201000000000000750000000000000001051e05b2860200000024050841636f726e626f69210000002d000000990600002405cd1003234301000000000000b20000000000000001050009"
	bossID    = 54651
	selfID    = 14957
	fightSec  = 75
)

var party = []struct {
	id    uint64
	name  string
	skill uint32 // class band: 12 Templar, 11 Gladiator, 14 Ranger, 18 Chanter, 17 Cleric
	hit   int
}{
	{selfID, "", 17_050_240, 900},
	{2001, "Barky", 12_020_100, 1700},
	{2002, "Twiggy", 11_030_000, 2400},
	{2003, "Mossie", 14_090_000, 2100},
	{2004, "Acornboi", 18_010_000, 1300},
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: fakefight out.pcap")
		os.Exit(2)
	}
	fh, err := os.Create(os.Args[1])
	if err != nil {
		panic(err)
	}
	defer fh.Close()
	w := pcapgo.NewWriter(fh)
	w.WriteFileHeader(65535, layers.LinkTypeEthernet)

	seq := uint32(1000)
	at := time.Date(2026, 10, 31, 20, 0, 0, 0, time.UTC)
	send := func(payload []byte) {
		f := tcpFrame(seq, payload)
		w.WritePacket(gopacket.CaptureInfo{Timestamp: at, CaptureLength: len(f), Length: len(f)}, f)
		seq += uint32(len(payload))
	}

	send(append([]byte{0x0E, 0x00, 0x36, 0, 0, 0, 0, 0, 0, 0, 0}, frame(mustHex(selfRec))...))
	for _, p := range party[1:] {
		send(frame(identity(p.id, p.name)))
	}
	send(frame(mustHex(rosterRec)))
	send(frame(mustHex(bossRec)))

	r := rand.New(rand.NewPCG(1, 2))
	for tick := 0; tick < fightSec*4; tick++ { // four volleys a second
		at = at.Add(250 * time.Millisecond)
		var batch []byte
		for _, p := range party {
			if r.IntN(3) == 0 {
				continue
			}
			amt := p.hit/2 + r.IntN(p.hit)
			if r.IntN(5) == 0 {
				amt *= 2 // a crit-ish hit
			}
			batch = append(batch, frame(damage(bossID, p.id, p.skill, uint64(amt)))...)
		}
		if tick%8 == 0 { // the cleric heals someone
			batch = append(batch, frame(heal(2001, selfID, 1_900_001_31, uint64(3000+r.IntN(2000))))...)
		}
		if tick%40 == 0 {
			batch = append(batch, 0x0E, 0x00, 0x36, 0, 0, 0, 0, 0, 0, 0, 0)
		}
		send(batch)
	}
	at = at.Add(250 * time.Millisecond)
	send(frame([]byte{0x42, 0x36, 0xfb, 0xaa, 0x03, 0x00, 0x03})) // the boss dies
}

func frame(body []byte) []byte {
	return append(binary.AppendUvarint(nil, uint64(len(body)+4)), body...)
}

func identity(id uint64, name string) []byte {
	b := binary.AppendUvarint([]byte{0x45, 0x36}, id)
	b = append(b, 0, 0, 0, 0, 0x07, byte(len(name)))
	return append(b, name...)
}

func damage(target, actor uint64, skill uint32, amount uint64) []byte {
	b := binary.AppendUvarint([]byte{0x04, 0x38}, target)
	b = append(b, 0x04, 0x00)
	b = binary.AppendUvarint(b, actor)
	b = binary.LittleEndian.AppendUint32(b, skill)
	b = append(b, 0x5d, 0x02)
	b = append(b, make([]byte, 8)...)
	b = binary.AppendUvarint(b, 16000)
	return binary.AppendUvarint(b, amount)
}

func heal(target, actor uint64, effect uint32, amount uint64) []byte {
	b := binary.AppendUvarint([]byte{0x05, 0x38}, target)
	b = append(b, 0x01)
	b = binary.AppendUvarint(b, actor)
	b = append(b, 0x02)
	b = binary.LittleEndian.AppendUint32(b, effect)
	return binary.AppendUvarint(b, amount)
}

func tcpFrame(seq uint32, payload []byte) []byte {
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4}
	ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolTCP, SrcIP: net.IP{203, 0, 113, 5}, DstIP: net.IP{192, 168, 1, 20}}
	tcp := &layers.TCP{SrcPort: 7777, DstPort: 50123, Seq: seq, ACK: true, PSH: true, Window: 512}
	tcp.SetNetworkLayerForChecksum(ip)
	buf := gopacket.NewSerializeBuffer()
	if err := gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, tcp, gopacket.Payload(payload)); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func mustHex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}
