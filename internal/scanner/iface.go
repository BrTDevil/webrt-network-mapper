// Package scanner descoperă dispozitivele din rețeaua locală.
package scanner

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
)

// NetInfo descrie interfața activă și rețeaua ei.
type NetInfo struct {
	Interface string       `json:"interface"`
	MAC       string       `json:"mac"`
	IP        netip.Addr   `json:"ip"`
	Prefix    netip.Prefix `json:"subnet"`
	Gateway   string       `json:"gateway"`
	DNS       []string     `json:"dns"`
	ifi       *net.Interface
}

// DetectNetwork găsește interfața folosită pentru ruta implicită.
// Dacă ifaceName nu e gol, folosește acea interfață.
func DetectNetwork(ifaceName string) (*NetInfo, error) {
	var ifi *net.Interface
	var localIP net.IP
	if ifaceName != "" {
		i, err := net.InterfaceByName(ifaceName)
		if err != nil {
			return nil, fmt.Errorf("interfața %q: %w", ifaceName, err)
		}
		ifi = i
	} else {
		// UDP „connect” nu trimite pachete, doar alege ruta și IP-ul sursă.
		conn, err := net.Dial("udp4", "192.0.2.1:9")
		if err == nil {
			localIP = conn.LocalAddr().(*net.UDPAddr).IP
			conn.Close()
		}
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	for i := range ifaces {
		it := &ifaces[i]
		if it.Flags&net.FlagUp == 0 || it.Flags&net.FlagLoopback != 0 {
			continue
		}
		if ifi != nil && it.Name != ifi.Name {
			continue
		}
		addrs, _ := it.Addrs()
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok || ipn.IP.To4() == nil {
				continue
			}
			if localIP != nil && !ipn.IP.Equal(localIP) {
				continue
			}
			ip, _ := netip.AddrFromSlice(ipn.IP.To4())
			ones, _ := ipn.Mask.Size()
			info := &NetInfo{
				Interface: it.Name,
				MAC:       strings.ToUpper(it.HardwareAddr.String()),
				IP:        ip,
				Prefix:    netip.PrefixFrom(ip, ones).Masked(),
				ifi:       it,
			}
			info.Gateway = defaultGateway(it.Name)
			info.DNS = dnsServers()
			return info, nil
		}
	}
	return nil, errors.New("nu am găsit o interfață IPv4 activă")
}

// Hosts întoarce toate adresele de host din subnet (fără rețea și broadcast).
func (n *NetInfo) Hosts() []netip.Addr {
	var out []netip.Addr
	p := n.Prefix.Masked()
	bits := p.Bits()
	a := p.Addr()
	if bits >= 31 {
		return []netip.Addr{a}
	}
	count := 1 << (32 - bits)
	a = a.Next() // sare peste adresa de rețea
	for i := 1; i < count-1; i++ {
		out = append(out, a)
		a = a.Next()
	}
	return out
}

func defaultGateway(iface string) string {
	switch runtime.GOOS {
	case "linux":
		f, err := os.Open("/proc/net/route")
		if err != nil {
			return ""
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			fs := strings.Fields(sc.Text())
			if len(fs) < 3 || fs[1] != "00000000" || (iface != "" && fs[0] != iface) {
				continue
			}
			var g uint32
			fmt.Sscanf(fs[2], "%X", &g)
			return fmt.Sprintf("%d.%d.%d.%d", byte(g), byte(g>>8), byte(g>>16), byte(g>>24))
		}
	case "darwin":
		out, err := exec.Command("route", "-n", "get", "default").Output()
		if err != nil {
			return ""
		}
		if m := regexp.MustCompile(`gateway:\s*(\S+)`).FindSubmatch(out); m != nil {
			return string(m[1])
		}
	case "windows":
		out, err := exec.Command("route", "print", "-4", "0.0.0.0").Output()
		if err != nil {
			return ""
		}
		re := regexp.MustCompile(`(?m)^\s*0\.0\.0\.0\s+0\.0\.0\.0\s+(\d+\.\d+\.\d+\.\d+)`)
		if m := re.FindSubmatch(out); m != nil {
			return string(m[1])
		}
	}
	return ""
}

func dnsServers() []string {
	var out []string
	switch runtime.GOOS {
	case "windows":
		b, err := exec.Command("ipconfig", "/all").Output()
		if err != nil {
			return nil
		}
		// Linia „DNS Servers” e urmată de adrese pe rânduri indentate.
		inDNS := false
		for _, line := range strings.Split(string(b), "\n") {
			l := strings.TrimSpace(line)
			if strings.Contains(l, "DNS Servers") || strings.Contains(l, "Servere DNS") {
				inDNS = true
				if i := strings.LastIndex(l, ":"); i >= 0 {
					l = strings.TrimSpace(l[i+1:])
				}
			} else if inDNS && strings.Contains(l, ":") {
				inDNS = false
			}
			if inDNS {
				if a, err := netip.ParseAddr(l); err == nil && a.Is4() {
					out = appendUniq(out, l)
				}
			}
		}
	default:
		out = parseResolv("/etc/resolv.conf")
		// systemd-resolved: serverele reale se văd în resolvectl.
		if len(out) == 1 && strings.HasPrefix(out[0], "127.") {
			if b, err := exec.Command("resolvectl", "dns").Output(); err == nil {
				real := []string{}
				for _, m := range regexp.MustCompile(`\b\d+\.\d+\.\d+\.\d+\b`).FindAll(b, -1) {
					real = appendUniq(real, string(m))
				}
				if len(real) > 0 {
					out = real
				}
			}
		}
	}
	return out
}

func parseResolv(path string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []string
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		fs := strings.Fields(sc.Text())
		if len(fs) >= 2 && fs[0] == "nameserver" {
			out = appendUniq(out, fs[1])
		}
	}
	return out
}

func appendUniq(s []string, v string) []string {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}
