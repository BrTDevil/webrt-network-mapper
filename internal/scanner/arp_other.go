//go:build !linux

package scanner

import (
	"context"
	"errors"
	"net/netip"
	"os/exec"
	"regexp"
	"strings"
)

func arpSweep(ctx context.Context, n *NetInfo, hosts []netip.Addr, progress func(done, total int)) (map[netip.Addr]string, error) {
	return nil, errors.New("ARP raw disponibil doar pe Linux")
}

// systemARPTable parsează `arp -a` (Windows: „192.168.1.1  aa-bb-cc-dd-ee-ff  dynamic”,
// macOS: „? (192.168.1.1) at aa:bb:cc:dd:ee:ff on en0”).
func systemARPTable() map[netip.Addr]string {
	out := map[netip.Addr]string{}
	b, err := exec.Command("arp", "-a").Output()
	if err != nil {
		return out
	}
	re := regexp.MustCompile(`(\d+\.\d+\.\d+\.\d+)\)?\s+(?:at\s+)?([0-9a-fA-F]{1,2}[:-][0-9a-fA-F]{1,2}[:-][0-9a-fA-F]{1,2}[:-][0-9a-fA-F]{1,2}[:-][0-9a-fA-F]{1,2}[:-][0-9a-fA-F]{1,2})`)
	for _, m := range re.FindAllStringSubmatch(string(b), -1) {
		ip, err := netip.ParseAddr(m[1])
		if err != nil {
			continue
		}
		mac := normalizeMAC(strings.ReplaceAll(m[2], "-", ":"))
		if mac == "FF:FF:FF:FF:FF:FF" || strings.HasPrefix(mac, "01:00:5E") {
			continue
		}
		out[ip] = mac
	}
	return out
}
