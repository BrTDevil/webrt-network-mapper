//go:build linux

package wifi

import (
	"context"
	"errors"
	"os/exec"
	"strconv"
	"strings"
)

func scanOS(ctx context.Context) ([]Network, error) {
	if _, err := exec.LookPath("nmcli"); err != nil {
		return nil, errors.New("nmcli nu este instalat (NetworkManager necesar pentru scanarea Wi-Fi)")
	}
	cmd := exec.CommandContext(ctx, "nmcli", "-t", "-f", "SSID,BSSID,CHAN,FREQ,SIGNAL,SECURITY", "dev", "wifi", "list", "--rescan", "yes")
	out, err := cmd.Output()
	if err != nil {
		// --rescan poate fi refuzat (rate-limit); reîncearcă fără.
		out, err = exec.CommandContext(ctx, "nmcli", "-t", "-f", "SSID,BSSID,CHAN,FREQ,SIGNAL,SECURITY", "dev", "wifi", "list").Output()
		if err != nil {
			return nil, err
		}
	}
	return parseNmcli(string(out)), nil
}

// parseNmcli parsează ieșirea în mod terse, unde „:” din valori e escapat ca „\:”.
func parseNmcli(s string) []Network {
	var out []Network
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		f := splitEscaped(line)
		if len(f) < 6 {
			continue
		}
		ch, _ := strconv.Atoi(f[2])
		freq, _ := strconv.Atoi(strings.Fields(f[3] + " 0")[0])
		sig, _ := strconv.Atoi(f[4])
		sec := f[5]
		if sec == "" || sec == "--" {
			sec = "Deschisă"
		}
		out = append(out, Network{SSID: f[0], BSSID: f[1], Channel: ch, Frequency: freq, Signal: PercentToDBM(sig), Security: sec})
	}
	return out
}

func splitEscaped(line string) []string {
	var fields []string
	var cur strings.Builder
	for i := 0; i < len(line); i++ {
		c := line[i]
		if c == '\\' && i+1 < len(line) {
			cur.WriteByte(line[i+1])
			i++
			continue
		}
		if c == ':' {
			fields = append(fields, cur.String())
			cur.Reset()
			continue
		}
		cur.WriteByte(c)
	}
	return append(fields, cur.String())
}
