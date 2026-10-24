//go:build linux

package wifi

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"
)

// CurrentConnection întoarce SSID-ul la care e conectat laptopul (sau nil).
func CurrentConnection(ctx context.Context) *Current {
	out, err := exec.CommandContext(ctx, "nmcli", "-t", "-f", "ACTIVE,SSID,BSSID,DEVICE", "dev", "wifi", "list", "--rescan", "no").Output()
	if err != nil {
		return nil
	}
	for _, line := range strings.Split(string(out), "\n") {
		f := splitEscaped(line)
		if len(f) >= 4 && f[0] == "yes" {
			return &Current{SSID: f[1], BSSID: normalizeBSSID(f[2]), Interface: f[3]}
		}
	}
	return nil
}

// Connect conectează laptopul la SSID folosind parola dată de utilizator.
// Parola e trimisă lui nmcli prin stdin (nu apare în lista de procese) și nu e
// păstrată: profilul temporar e șters de Restore. Fără parolă, se folosește un
// profil deja salvat în sistem pentru acel SSID.
func Connect(ctx context.Context, ssid, password, iface string) (*Session, error) {
	if _, err := exec.LookPath("nmcli"); err != nil {
		return nil, errors.New("nmcli nu este instalat (NetworkManager)")
	}
	s := &Session{Interface: iface}
	if cur := CurrentConnection(ctx); cur != nil {
		if s.Interface == "" {
			s.Interface = cur.Interface
		}
		if cur.SSID == ssid {
			return s, nil // deja conectat, nimic de schimbat
		}
	}
	if s.Interface == "" {
		s.Interface = wifiInterface(ctx)
	}
	if s.Interface == "" {
		return nil, errors.New("nu am găsit o placă Wi-Fi")
	}
	s.prevUUID = activeUUID(ctx, s.Interface)

	var cmd *exec.Cmd
	if password == "" {
		// profil existent (rețea la care laptopul s-a mai conectat)
		cmd = exec.CommandContext(ctx, "nmcli", "--wait", "40", "connection", "up", "id", ssid, "ifname", s.Interface)
	} else {
		s.tempName = fmt.Sprintf("netmap-temp-%d", time.Now().Unix())
		cmd = exec.CommandContext(ctx, "nmcli", "--ask", "--wait", "40", "device", "wifi", "connect", ssid, "name", s.tempName, "ifname", s.Interface)
		cmd.Stdin = strings.NewReader(password + "\n")
	}
	s.switched = true
	out, err := cmd.CombinedOutput()
	if err != nil {
		s.Restore()
		msg := strings.TrimSpace(string(out))
		if strings.Contains(strings.ToLower(msg), "secrets were required") || strings.Contains(msg, "802-11-wireless-security") {
			return nil, errors.New("parola Wi-Fi este greșită")
		}
		if password == "" && strings.Contains(msg, "unknown connection") {
			return nil, fmt.Errorf("laptopul nu are salvată rețeaua %q — introdu parola", ssid)
		}
		return nil, fmt.Errorf("conectarea la %q a eșuat: %s", ssid, lastLine(msg))
	}
	// Așteaptă adresa IP de la DHCP.
	deadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(deadline) {
		if hasIPv4(s.Interface) {
			return s, nil
		}
		select {
		case <-ctx.Done():
			s.Restore()
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	s.Restore()
	return nil, fmt.Errorf("conectat la %q, dar nu am primit adresă IP (DHCP)", ssid)
}

// Restore șterge profilul temporar și reactivează conexiunea de dinainte.
func (s *Session) Restore() {
	if s == nil || !s.switched {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if s.tempName != "" {
		exec.CommandContext(ctx, "nmcli", "connection", "delete", "id", s.tempName).Run()
	}
	if s.prevUUID != "" {
		exec.CommandContext(ctx, "nmcli", "--wait", "40", "connection", "up", "uuid", s.prevUUID).Run()
	}
	s.switched = false
}

func activeUUID(ctx context.Context, iface string) string {
	out, err := exec.CommandContext(ctx, "nmcli", "-t", "-f", "UUID,DEVICE", "connection", "show", "--active").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		f := splitEscaped(line)
		if len(f) >= 2 && f[1] == iface {
			return f[0]
		}
	}
	return ""
}

func wifiInterface(ctx context.Context) string {
	out, err := exec.CommandContext(ctx, "nmcli", "-t", "-f", "DEVICE,TYPE", "device").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		f := splitEscaped(line)
		if len(f) >= 2 && f[1] == "wifi" {
			return f[0]
		}
	}
	return ""
}

func hasIPv4(iface string) bool {
	it, err := net.InterfaceByName(iface)
	if err != nil {
		return false
	}
	addrs, _ := it.Addrs()
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil && !ipn.IP.IsLinkLocalUnicast() {
			return true
		}
	}
	return false
}

func lastLine(s string) string {
	l := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimPrefix(l[len(l)-1], "Error: ")
}
