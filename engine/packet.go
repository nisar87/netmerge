package engine

import (
	"encoding/binary"
	"net/netip"
)

// Small helpers to read and rewrite IPv4/IPv6 packets with TCP, UDP or ICMP inside.

const (
	protoICMP   = 1
	protoTCP    = 6
	protoUDP    = 17
	protoICMPv6 = 58
)

// ipPacket is a parsed view of one IP packet (no copies).
type ipPacket struct {
	b       []byte
	v6      bool
	proto   uint8
	hdrLen  int // IP header length (IPv6: fixed 40; extension headers are not followed)
	src     netip.Addr
	dst     netip.Addr
	payload []byte // the TCP/UDP/ICMP part
	frag    bool   // a fragment (not the whole packet)
}

func parseIP(b []byte) (ipPacket, bool) {
	var p ipPacket
	if len(b) < 20 {
		return p, false
	}
	p.b = b
	switch b[0] >> 4 {
	case 4:
		ihl := int(b[0]&0x0f) * 4
		total := int(binary.BigEndian.Uint16(b[2:4]))
		if ihl < 20 || total < ihl || total > len(b) {
			return p, false
		}
		p.b = b[:total]
		p.hdrLen = ihl
		p.proto = b[9]
		p.src = netip.AddrFrom4([4]byte(b[12:16]))
		p.dst = netip.AddrFrom4([4]byte(b[16:20]))
		fo := binary.BigEndian.Uint16(b[6:8])
		p.frag = fo&0x2000 != 0 || fo&0x1fff != 0
		p.payload = p.b[ihl:]
	case 6:
		if len(b) < 40 {
			return p, false
		}
		pl := int(binary.BigEndian.Uint16(b[4:6]))
		if 40+pl > len(b) {
			return p, false
		}
		p.v6 = true
		p.b = b[:40+pl]
		p.hdrLen = 40
		p.proto = b[6]
		p.src = netip.AddrFrom16([16]byte(b[8:24]))
		p.dst = netip.AddrFrom16([16]byte(b[24:40]))
		p.frag = p.proto == 44
		p.payload = p.b[40:]
	default:
		return p, false
	}
	return p, true
}

// ---- checksums ----

func csumAdd(sum uint32, b []byte) uint32 {
	for len(b) >= 2 {
		sum += uint32(b[0])<<8 | uint32(b[1])
		b = b[2:]
	}
	if len(b) == 1 {
		sum += uint32(b[0]) << 8
	}
	return sum
}

func csumFold(sum uint32) uint16 {
	for sum>>16 != 0 {
		sum = sum&0xffff + sum>>16
	}
	return ^uint16(sum)
}

// pseudoSum is the checksum of the pseudo header for TCP/UDP.
func pseudoSum(src, dst netip.Addr, proto uint8, length int) uint32 {
	var sum uint32
	sum = csumAdd(sum, src.AsSlice())
	sum = csumAdd(sum, dst.AsSlice())
	sum += uint32(proto)
	sum += uint32(length)
	return sum
}

// fixIPv4Header recomputes the IPv4 header checksum.
func fixIPv4Header(b []byte) {
	ihl := int(b[0]&0x0f) * 4
	b[10], b[11] = 0, 0
	binary.BigEndian.PutUint16(b[10:12], csumFold(csumAdd(0, b[:ihl])))
}

// fixL4 recomputes the TCP or UDP checksum of a packet whose addresses or ports changed.
func fixL4(p ipPacket) {
	seg := p.payload
	var off int
	switch p.proto {
	case protoTCP:
		if len(seg) < 20 {
			return
		}
		off = 16
	case protoUDP:
		if len(seg) < 8 {
			return
		}
		off = 6
	default:
		return
	}
	seg[off], seg[off+1] = 0, 0
	sum := pseudoSum(p.src, p.dst, p.proto, len(seg))
	sum = csumAdd(sum, seg)
	c := csumFold(sum)
	if c == 0 && p.proto == protoUDP {
		c = 0xffff
	}
	binary.BigEndian.PutUint16(seg[off:off+2], c)
}

// setAddrs rewrites source/destination addresses and ports in place and fixes the checksums.
func setAddrs(p *ipPacket, src netip.AddrPort, dst netip.AddrPort) {
	b := p.b
	if p.v6 {
		s16, d16 := src.Addr().As16(), dst.Addr().As16()
		copy(b[8:24], s16[:])
		copy(b[24:40], d16[:])
	} else {
		s4, d4 := src.Addr().As4(), dst.Addr().As4()
		copy(b[12:16], s4[:])
		copy(b[16:20], d4[:])
		fixIPv4Header(b)
	}
	p.src, p.dst = src.Addr(), dst.Addr()
	binary.BigEndian.PutUint16(p.payload[0:2], src.Port())
	binary.BigEndian.PutUint16(p.payload[2:4], dst.Port())
	fixL4(*p)
}

func l4Ports(p ipPacket) (uint16, uint16, bool) {
	if len(p.payload) < 4 {
		return 0, 0, false
	}
	return binary.BigEndian.Uint16(p.payload[0:2]), binary.BigEndian.Uint16(p.payload[2:4]), true
}

// buildUDP makes a whole IP+UDP packet (for replies to apps).
func buildUDP(src, dst netip.AddrPort, data []byte) []byte {
	v6 := src.Addr().Is6() && !src.Addr().Is4In6()
	ulen := 8 + len(data)
	var b []byte
	if v6 {
		b = make([]byte, 40+ulen)
		b[0] = 0x60
		binary.BigEndian.PutUint16(b[4:6], uint16(ulen))
		b[6] = protoUDP
		b[7] = 64
		s, d := src.Addr().As16(), dst.Addr().As16()
		copy(b[8:24], s[:])
		copy(b[24:40], d[:])
	} else {
		b = make([]byte, 20+ulen)
		b[0] = 0x45
		binary.BigEndian.PutUint16(b[2:4], uint16(20+ulen))
		b[8] = 64
		b[9] = protoUDP
		s, d := src.Addr().Unmap().As4(), dst.Addr().Unmap().As4()
		copy(b[12:16], s[:])
		copy(b[16:20], d[:])
		fixIPv4Header(b)
	}
	p, _ := parseIP(b)
	u := p.payload
	binary.BigEndian.PutUint16(u[0:2], src.Port())
	binary.BigEndian.PutUint16(u[2:4], dst.Port())
	binary.BigEndian.PutUint16(u[4:6], uint16(ulen))
	copy(u[8:], data)
	fixL4(p)
	return b
}

// echoReply turns an ICMP / ICMPv6 echo request into the reply, in place. Returns false for anything else.
func echoReply(p ipPacket) bool {
	if len(p.payload) < 8 {
		return false
	}
	if !p.v6 && p.proto == protoICMP && p.payload[0] == 8 {
		p.payload[0] = 0
		p.payload[2], p.payload[3] = 0, 0
		binary.BigEndian.PutUint16(p.payload[2:4], csumFold(csumAdd(0, p.payload)))
		copy(p.b[12:16], p.dst.AsSlice())
		copy(p.b[16:20], p.src.AsSlice())
		fixIPv4Header(p.b)
		return true
	}
	if p.v6 && p.proto == protoICMPv6 && p.payload[0] == 128 {
		p.payload[0] = 129
		p.payload[2], p.payload[3] = 0, 0
		src, dst := p.dst, p.src
		copy(p.b[8:24], src.AsSlice())
		copy(p.b[24:40], dst.AsSlice())
		sum := pseudoSum(src, dst, protoICMPv6, len(p.payload))
		binary.BigEndian.PutUint16(p.payload[2:4], csumFold(csumAdd(sum, p.payload)))
		return true
	}
	return false
}
