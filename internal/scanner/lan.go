package scanner

import (
	"context"
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"sync"
)

// Options configurează scanul LAN.
type Options struct {
	Interface string // gol = interfața rutei implicite
	Subnet    string // gol = subnetul interfeței
	Ports     []int  // gol = CommonPorts
	MaxHosts  int    // protecție: refuză subnet-uri mai mari (implicit /22 = 1024)
	Progress  func(pct int, msg string)
}

// Host e un dispozitiv găsit.
type Host struct {
	IP        netip.Addr
	MAC       string
	Vendor    string
	Hostname  string
	Ports     []int
	LatencyMS float64
	Type      string
	OS        string
}

// Result e rezultatul scanului LAN.
type Result struct {
	Net    *NetInfo
	Method string
	Hosts  []Host
}

// ErrSubnetTooLarge semnalează că subnetul depășește limita și cere confirmare.
type ErrSubnetTooLarge struct {
	Subnet string
	Hosts  int
}

func (e *ErrSubnetTooLarge) Error() string {
	return fmt.Sprintf("subnetul %s are %d adrese (mai mare de /22); confirmă explicit scanarea", e.Subnet, e.Hosts)
}

// ScanLAN descoperă host-urile active, apoi le completează cu vendor, hostname, porturi și tip.
func ScanLAN(ctx context.Context, opt Options) (*Result, error) {
	report := func(p int, m string) {
		if opt.Progress != nil {
			opt.Progress(p, m)
		}
	}
	if len(opt.Ports) == 0 {
		opt.Ports = CommonPorts
	}
	if opt.MaxHosts == 0 {
		opt.MaxHosts = 1022
	}

	n, err := DetectNetwork(opt.Interface)
	if err != nil {
		return nil, err
	}
	if opt.Subnet != "" {
		p, err := netip.ParsePrefix(opt.Subnet)
		if err != nil {
			return nil, fmt.Errorf("subnet invalid %q: %w", opt.Subnet, err)
		}
		n.Prefix = p.Masked()
	}
	hosts := n.Hosts()
	if len(hosts) > opt.MaxHosts {
		return nil, &ErrSubnetTooLarge{Subnet: n.Prefix.String(), Hosts: len(hosts)}
	}
	report(2, fmt.Sprintf("Interfață %s, subnet %s", n.Interface, n.Prefix))

	// 1. Descoperire: ARP raw dacă avem privilegii, altfel ping + tabela ARP.
	macs := map[netip.Addr]string{}
	lat := map[netip.Addr]float64{}
	method := "arp"
	arpRes, arpErr := arpSweep(ctx, n, hosts, func(d, t int) { report(2+d*28/t, "ARP sweep") })
	if arpErr == nil {
		macs = arpRes
		report(30, fmt.Sprintf("ARP: %d dispozitive; verific latența", len(macs)))
	} else {
		method = "ping + tabela ARP"
		report(3, "ARP raw indisponibil ("+shortErr(arpErr)+"), folosesc ping sweep")
	}
	// Ping-ul rulează oricum, pe tot subnetul: dă latența și, mai ales, a doua șansă
	// de descoperire. Broadcast-urile ARP se pierd des pe Wi-Fi și prin repetoare, iar
	// kernelul reîncearcă rezolvarea ARP pentru fiecare ping; chiar dacă host-ul are
	// firewall și nu răspunde la ping, MAC-ul lui ajunge în tabela ARP a sistemului.
	alive := pingSweep(ctx, hosts, func(d, t int) {
		if method == "arp" {
			report(30+d*10/t, fmt.Sprintf("Ping (a doua trecere) %d/%d", d, t))
		} else {
			report(3+d*37/t, fmt.Sprintf("Ping %d/%d", d, t))
		}
	})
	for ip, l := range alive {
		lat[ip] = l
	}
	for ip, mac := range systemARPTable() {
		if !n.Prefix.Contains(ip) {
			continue
		}
		if _, ok := macs[ip]; !ok {
			macs[ip] = mac
		}
	}
	// Propriul calculator nu apare în tabela ARP.
	macs[n.IP] = n.MAC
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	report(40, fmt.Sprintf("%d dispozitive active; identific servicii", len(macs)))

	// 2. Detalii per host, în paralel.
	ips := keys(macs)
	sort.Slice(ips, func(i, j int) bool { return ips[i].Less(ips[j]) })
	res := make([]Host, len(ips))
	lim := newPortLimiter(64)
	sem := make(chan struct{}, 32)
	var wg sync.WaitGroup
	var mu sync.Mutex
	done := 0
	for i, ip := range ips {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, ip netip.Addr) {
			defer wg.Done()
			defer func() { <-sem }()
			mac := normalizeMAC(macs[ip])
			h := Host{IP: ip, MAC: mac, Vendor: LookupVendor(mac), LatencyMS: lat[ip]}
			h.Hostname = resolveHostname(ctx, ip)
			h.Ports = scanPorts(ctx, lim, ip, opt.Ports)
			h.Type, h.OS = Classify(ClassifyInput{
				IP: ip.String(), MAC: mac, Vendor: h.Vendor, Hostname: h.Hostname, Ports: h.Ports,
				IsGateway: ip.String() == n.Gateway, IsSelf: ip == n.IP,
			})
			res[i] = h
			mu.Lock()
			done++
			report(40+done*58/len(ips), fmt.Sprintf("Analizat %d/%d: %s", done, len(ips), ip))
			mu.Unlock()
		}(i, ip)
	}
	wg.Wait()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return &Result{Net: n, Method: method, Hosts: res}, nil
}

func keys[V any](m map[netip.Addr]V) []netip.Addr {
	out := make([]netip.Addr, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func shortErr(err error) string {
	s := err.Error()
	if strings.Contains(s, "operation not permitted") || strings.Contains(s, "permission denied") {
		return "lipsesc privilegiile CAP_NET_RAW"
	}
	if len(s) > 80 {
		s = s[:80]
	}
	return s
}
