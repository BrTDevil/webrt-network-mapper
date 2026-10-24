//go:build linux

package scanner

import (
	"bufio"
	"context"
	"net"
	"net/netip"
	"os"
	"strings"
	"time"

	"github.com/mdlayher/arp"
)

// arpSweep trimite cereri ARP către toate host-urile și colectează răspunsurile.
// Necesită root sau CAP_NET_RAW; altfel întoarce eroare și se folosește fallback-ul.
func arpSweep(ctx context.Context, n *NetInfo, hosts []netip.Addr, progress func(done, total int)) (map[netip.Addr]string, error) {
	c, err := arp.Dial(n.ifi)
	if err != nil {
		return nil, err
	}
	defer c.Close()

	found := map[netip.Addr]string{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			c.SetReadDeadline(time.Now().Add(1500 * time.Millisecond))
			p, _, err := c.Read()
			if err != nil {
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					return
				}
				continue
			}
			if p.Operation != arp.OperationReply {
				continue
			}
			if n.Prefix.Contains(p.SenderIP) {
				found[p.SenderIP] = strings.ToUpper(p.SenderHardwareAddr.String())
			}
		}
	}()

	// Două runde de cereri: unele dispozitive (telefoane în sleep) ratează prima.
	for round := 0; round < 2; round++ {
		for i, h := range hosts {
			if ctx.Err() != nil {
				break
			}
			if h == n.IP {
				continue
			}
			c.Request(h)
			if i%16 == 0 {
				time.Sleep(2 * time.Millisecond)
			}
			if progress != nil && round == 1 {
				progress(i+1, len(hosts))
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	<-done
	return found, nil
}

// systemARPTable citește tabela ARP a kernelului.
func systemARPTable() map[netip.Addr]string {
	out := map[netip.Addr]string{}
	f, err := os.Open("/proc/net/arp")
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Scan() // antet
	for sc.Scan() {
		fs := strings.Fields(sc.Text())
		// IP address, HW type, Flags, HW address, Mask, Device
		if len(fs) < 4 || fs[2] == "0x0" || fs[3] == "00:00:00:00:00:00" {
			continue
		}
		if ip, err := netip.ParseAddr(fs[0]); err == nil {
			out[ip] = strings.ToUpper(fs[3])
		}
	}
	return out
}
