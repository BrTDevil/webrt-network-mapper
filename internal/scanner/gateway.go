package scanner

import (
	"context"
	"net/netip"
)

// GatewayMAC întoarce IP-ul și MAC-ul routerului rețelei curente ("" dacă nu se pot afla).
// Același IP (ex. 192.168.1.1) apare în multe locații; MAC-ul routerului e cel care
// deosebește rețelele. Un ping forțează rezolvarea ARP, apoi MAC-ul se citește din
// tabela ARP a sistemului — nu cere privilegii.
func GatewayMAC(ctx context.Context, ifaceName string) (gw, mac string) {
	n, err := DetectNetwork(ifaceName)
	if err != nil || n.Gateway == "" {
		return "", ""
	}
	ip, err := netip.ParseAddr(n.Gateway)
	if err != nil {
		return n.Gateway, ""
	}
	if m := systemARPTable()[ip]; m != "" {
		return n.Gateway, normalizeMAC(m)
	}
	pingOne(ctx, ip)
	return n.Gateway, normalizeMAC(systemARPTable()[ip])
}
