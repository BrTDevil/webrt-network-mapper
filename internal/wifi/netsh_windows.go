//go:build windows

package wifi

import (
	"context"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

func scanOS(ctx context.Context) ([]Network, error) {
	cmd := exec.CommandContext(ctx, "netsh", "wlan", "show", "networks", "mode=bssid")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return parseNetsh(string(out)), nil
}

var (
	reSSID   = regexp.MustCompile(`^SSID \d+\s*:\s*(.*)$`)
	reBSSID  = regexp.MustCompile(`^BSSID \d+\s*:\s*([0-9a-fA-F:]{17})`)
	reKV     = regexp.MustCompile(`^([^:]+?)\s*:\s*(.*)$`)
	reNumber = regexp.MustCompile(`\d+`)
)

// parseNetsh parsează ieșirea text; cheile sunt recunoscute și în alte limbi
// după poziție (Authentication/Signal/Channel apar mereu în această ordine).
func parseNetsh(s string) []Network {
	var out []Network
	var ssid, auth string
	var cur *Network
	flush := func() {
		if cur != nil {
			out = append(out, *cur)
			cur = nil
		}
	}
	for _, raw := range strings.Split(strings.ReplaceAll(s, "\r", ""), "\n") {
		line := strings.TrimSpace(raw)
		if m := reSSID.FindStringSubmatch(line); m != nil {
			flush()
			ssid, auth = m[1], ""
			continue
		}
		if m := reBSSID.FindStringSubmatch(line); m != nil {
			flush()
			cur = &Network{SSID: ssid, BSSID: m[1], Security: auth}
			continue
		}
		m := reKV.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		key := strings.ToLower(m[1])
		val := strings.TrimSpace(m[2])
		switch {
		case strings.Contains(key, "authentication") || strings.Contains(key, "autentificare"):
			auth = val
		case cur == nil:
		case strings.Contains(key, "signal") || strings.Contains(key, "semnal"):
			if n := reNumber.FindString(val); n != "" {
				p, _ := strconv.Atoi(n)
				cur.Signal = PercentToDBM(p)
			}
		case strings.Contains(key, "channel") && !strings.Contains(key, "utilization") || strings.Contains(key, "canal"):
			cur.Channel, _ = strconv.Atoi(reNumber.FindString(val))
		case strings.Contains(key, "band") || strings.Contains(key, "bandă"):
			switch {
			case strings.HasPrefix(val, "2"):
				cur.Band = "2.4"
			case strings.HasPrefix(val, "5"):
				cur.Band = "5"
			case strings.HasPrefix(val, "6"):
				cur.Band = "6"
			}
		}
	}
	flush()
	return out
}
