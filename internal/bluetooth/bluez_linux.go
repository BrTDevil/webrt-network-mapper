//go:build linux

package bluetooth

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	reEvent = regexp.MustCompile(`Device ((?:[0-9A-F]{2}:){5}[0-9A-F]{2})`)
	reRSSI  = regexp.MustCompile(`RSSI: (?:0x[0-9a-f]+ \()?(-?\d+)\)?`)
	reInfo  = regexp.MustCompile(`^\s*([A-Za-z.]+):\s*(.*)$`)
)

// scanOS folosește bluetoothctl (BlueZ): pornește descoperirea, ascultă, apoi
// citește ce a aflat BlueZ despre fiecare dispozitiv văzut.
func scanOS(ctx context.Context, dur time.Duration) ([]Device, error) {
	if _, err := exec.LookPath("bluetoothctl"); err != nil {
		return nil, errors.New("bluetoothctl nu este instalat (pachetul bluez)")
	}
	show, err := exec.CommandContext(ctx, "bluetoothctl", "show").Output()
	if err != nil || !bytes.Contains(show, []byte("Controller")) {
		return nil, errors.New("nu există adaptor Bluetooth")
	}
	if bytes.Contains(show, []byte("Powered: no")) {
		return nil, errors.New("Bluetooth-ul este oprit — pornește-l din setările sistemului")
	}

	secs := int(dur.Seconds())
	if secs < 4 {
		secs = 4
	}
	cmd := exec.CommandContext(ctx, "bluetoothctl", "--timeout", strconv.Itoa(secs), "scan", "on")
	out, _ := cmd.Output() // iese cu cod ≠ 0 la timeout; ne interesează doar ieșirea
	seen := map[string]*int{}
	sc := bufio.NewScanner(bytes.NewReader(stripANSI(out)))
	for sc.Scan() {
		line := sc.Text()
		m := reEvent.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		mac := m[1]
		if _, ok := seen[mac]; !ok {
			seen[mac] = nil
		}
		if r := reRSSI.FindStringSubmatch(line); r != nil {
			v, _ := strconv.Atoi(r[1])
			seen[mac] = &v
		}
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	var devs []Device
	for mac, rssi := range seen {
		d := Device{MAC: mac, RSSI: rssi}
		info, err := exec.CommandContext(ctx, "bluetoothctl", "info", mac).Output()
		if err == nil {
			parseInfo(stripANSI(info), &d)
		}
		devs = append(devs, d)
	}
	return devs, nil
}

func parseInfo(b []byte, d *Device) {
	lines := strings.Split(string(b), "\n")
	if len(lines) > 0 {
		if strings.Contains(lines[0], "(random)") {
			d.AddressType = "random"
		} else if strings.Contains(lines[0], "(public)") {
			d.AddressType = "public"
		}
	}
	for _, l := range lines {
		m := reInfo.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		k, v := m[1], strings.TrimSpace(m[2])
		switch k {
		case "Name":
			d.Name = v
		case "Alias":
			if d.Name == "" {
				d.Name = v
			}
		case "Icon":
			d.Kind = v
		case "Paired":
			d.Paired = v == "yes"
		case "RSSI":
			if r := regexp.MustCompile(`-?\d+`).FindAllString(v, -1); len(r) > 0 {
				n, _ := strconv.Atoi(r[len(r)-1])
				if n < 0 {
					d.RSSI = &n
				}
			}
		case "ManufacturerData.Key":
			// „0x004c (76)”
			if f := strings.Fields(v); len(f) > 0 {
				if id, err := strconv.ParseInt(strings.TrimPrefix(f[0], "0x"), 16, 32); err == nil {
					d.Manufacturer = CompanyName(int(id))
				}
			}
		}
	}
}

var reANSI = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]|\x01|\x02`)

func stripANSI(b []byte) []byte { return reANSI.ReplaceAll(b, nil) }
