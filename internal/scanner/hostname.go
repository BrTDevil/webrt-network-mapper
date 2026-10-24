package scanner

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"time"
)

// resolveHostname încearcă pe rând: reverse DNS, mDNS (PTR unicast), NetBIOS.
func resolveHostname(ctx context.Context, ip netip.Addr) string {
	if h := reverseDNS(ctx, ip); h != "" {
		return h
	}
	if h := mdnsPTR(ip); h != "" {
		return h
	}
	return netbiosName(ip)
}

func reverseDNS(ctx context.Context, ip netip.Addr) string {
	ctx, cancel := context.WithTimeout(ctx, 1200*time.Millisecond)
	defer cancel()
	names, err := net.DefaultResolver.LookupAddr(ctx, ip.String())
	if err != nil || len(names) == 0 {
		return ""
	}
	h := strings.TrimSuffix(names[0], ".")
	// systemd-resolved răspunde cu nume sintetice pentru gateway / host local.
	if h == "_gateway" || h == "_outbound" || strings.HasSuffix(h, ".in-addr.arpa") {
		return ""
	}
	return h
}

func reverseName(ip netip.Addr) string {
	b := ip.As4()
	return fmt.Sprintf("%d.%d.%d.%d.in-addr.arpa", b[3], b[2], b[1], b[0])
}

// mdnsPTR trimite o interogare PTR direct către host pe portul 5353.
// Multe dispozitive Apple, imprimante și Linux cu Avahi răspund unicast.
func mdnsPTR(ip netip.Addr) string {
	conn, err := net.DialTimeout("udp4", net.JoinHostPort(ip.String(), "5353"), 500*time.Millisecond)
	if err != nil {
		return ""
	}
	defer conn.Close()
	q := buildDNSQuery(reverseName(ip), 12) // PTR
	if _, err := conn.Write(q); err != nil {
		return ""
	}
	conn.SetReadDeadline(time.Now().Add(600 * time.Millisecond))
	buf := make([]byte, 1500)
	n, err := conn.Read(buf)
	if err != nil {
		return ""
	}
	name := parsePTRAnswer(buf[:n])
	return strings.TrimSuffix(strings.TrimSuffix(name, "."), ".local")
}

func buildDNSQuery(name string, qtype uint16) []byte {
	b := make([]byte, 12, 64)
	binary.BigEndian.PutUint16(b[0:], 0x4e4d) // id
	binary.BigEndian.PutUint16(b[4:], 1)      // qdcount
	for _, label := range strings.Split(name, ".") {
		b = append(b, byte(len(label)))
		b = append(b, label...)
	}
	b = append(b, 0)
	b = binary.BigEndian.AppendUint16(b, qtype)
	b = binary.BigEndian.AppendUint16(b, 1) // IN
	return b
}

// readName decodează un nume DNS (cu pointeri de compresie).
func readName(msg []byte, off int) (string, int, bool) {
	var labels []string
	jumped := false
	end := off
	for hops := 0; hops < 32; hops++ {
		if off >= len(msg) {
			return "", 0, false
		}
		l := int(msg[off])
		switch {
		case l == 0:
			if !jumped {
				end = off + 1
			}
			return strings.Join(labels, "."), end, true
		case l&0xC0 == 0xC0:
			if off+1 >= len(msg) {
				return "", 0, false
			}
			if !jumped {
				end = off + 2
			}
			off = int(binary.BigEndian.Uint16(msg[off:]) & 0x3FFF)
			jumped = true
		default:
			if off+1+l > len(msg) {
				return "", 0, false
			}
			labels = append(labels, string(msg[off+1:off+1+l]))
			off += 1 + l
		}
	}
	return "", 0, false
}

func parsePTRAnswer(msg []byte) string {
	if len(msg) < 12 {
		return ""
	}
	qd := int(binary.BigEndian.Uint16(msg[4:]))
	an := int(binary.BigEndian.Uint16(msg[6:]))
	off := 12
	for i := 0; i < qd; i++ {
		_, next, ok := readName(msg, off)
		if !ok {
			return ""
		}
		off = next + 4
	}
	for i := 0; i < an; i++ {
		_, next, ok := readName(msg, off)
		if !ok || next+10 > len(msg) {
			return ""
		}
		typ := binary.BigEndian.Uint16(msg[next:])
		rdlen := int(binary.BigEndian.Uint16(msg[next+8:]))
		rdata := next + 10
		if typ == 12 {
			name, _, ok := readName(msg, rdata)
			if ok {
				return name
			}
		}
		off = rdata + rdlen
	}
	return ""
}

// netbiosName face o interogare NBSTAT (UDP 137) și întoarce numele stației.
func netbiosName(ip netip.Addr) string {
	conn, err := net.DialTimeout("udp4", net.JoinHostPort(ip.String(), "137"), 500*time.Millisecond)
	if err != nil {
		return ""
	}
	defer conn.Close()
	q := []byte{
		0x13, 0x37, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x20, 'C', 'K',
	}
	for i := 0; i < 30; i++ {
		q = append(q, 'A')
	}
	q = append(q, 0x00, 0x00, 0x21, 0x00, 0x01)
	if _, err := conn.Write(q); err != nil {
		return ""
	}
	conn.SetReadDeadline(time.Now().Add(600 * time.Millisecond))
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil || n < 57 {
		return ""
	}
	// Antet (12) + nume (34) + tip/clasă/TTL/rdlen (10) = 56, apoi nr. de nume.
	count := int(buf[56])
	off := 57
	for i := 0; i < count && off+18 <= n; i++ {
		name := strings.TrimSpace(string(buf[off : off+15]))
		suffix := buf[off+15]
		flags := binary.BigEndian.Uint16(buf[off+16:])
		off += 18
		if suffix == 0x00 && flags&0x8000 == 0 { // nume unic de stație, nu de grup
			return name
		}
	}
	return ""
}
