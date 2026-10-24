package scanner

import (
	"context"
	"net"
	"net/netip"
	"sort"
	"strconv"
	"sync"
	"time"
)

// CommonPorts e lista scurtă de porturi verificate (doar TCP connect, fără autentificare).
var CommonPorts = []int{21, 22, 23, 53, 80, 139, 443, 445, 515, 554, 631, 1883, 3389, 5900, 8080, 8443, 9100}

// ServiceNames asociază porturile cu serviciul uzual.
var ServiceNames = map[int]string{
	21: "ftp", 22: "ssh", 23: "telnet", 53: "dns", 80: "http", 139: "netbios", 443: "https",
	445: "smb", 515: "lpd", 554: "rtsp", 631: "ipp", 1883: "mqtt", 3389: "rdp", 5900: "vnc",
	8080: "http-alt", 8443: "https-alt", 9100: "jetdirect",
}

// portLimiter limitează conexiunile TCP simultane pe tot scanul.
type portLimiter chan struct{}

func newPortLimiter(n int) portLimiter { return make(portLimiter, n) }

// scanPorts încearcă TCP connect cu timeout 300 ms pe fiecare port.
func scanPorts(ctx context.Context, lim portLimiter, ip netip.Addr, ports []int) []int {
	var mu sync.Mutex
	var open []int
	var wg sync.WaitGroup
	d := net.Dialer{Timeout: 300 * time.Millisecond}
	for _, p := range ports {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			select {
			case lim <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-lim }()
			conn, err := d.DialContext(ctx, "tcp4", net.JoinHostPort(ip.String(), strconv.Itoa(p)))
			if err != nil {
				return
			}
			conn.Close()
			mu.Lock()
			open = append(open, p)
			mu.Unlock()
		}(p)
	}
	wg.Wait()
	sort.Ints(open)
	return open
}
